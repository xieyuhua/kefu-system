package service

// ---------------- 账号域（Account） ----------------
// 后台账号（users）、渠道分配关系（agent_channels）、登录鉴权与个人资料。
//
// 形态约定：业务编排都是 AccountService 的方法（指针接收者），构造时建好自己的仓储依赖；
// 只有无状态的纯工具函数（authErr / hashPwd / orDefault）保留为包级函数。
import (
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/auth"
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/setting"
)

// AccountService 账号内核的业务编排层。
type AccountService struct {
	repo *repository.UserRepository
}

// NewAccountService 创建业务层实例（自带数据访问层依赖）。
func NewAccountService() *AccountService {
	return &AccountService{repo: repository.NewUserRepository()}
}

// init 把「按租户 + 账号 ID 载入账号」的能力注入 platform/authctx。
// platform 不反向依赖业务内核，authctx.CurrentUser 只能靠这个回调拿到账号实体，
// 因此注册点落在账号内核自己的 init 里（route 会 import 本包，保证注册被执行）。
func init() {
	authctx.SetUserLoader(NewAccountService().LoadUser)
}

// ---------------- 鉴权错误 ----------------

// AuthError 带 HTTP 语义的鉴权错误，供 handler 直接映射为响应（param/unauth/forbid/server）。
type AuthError struct {
	Status string
	Msg    string
}

func (e *AuthError) Error() string { return e.Msg }

func authErr(status, msg string) *AuthError { return &AuthError{Status: status, Msg: msg} }

// hashPwd 口令哈希（bcrypt），失败由调用方按「系统错误」处理。
func hashPwd(p string) (string, error) { return auth.Default().HashPassword(p) }

// ---------------- 登录 ----------------

// Login 校验账号密码并签发 token。
// 返回的 *AuthError 由 handler 按 Status 映射响应（账号或密码错误 → 401，停用 → 403）。
func (s *AccountService) Login(req dto.LoginRequest) (*dto.LoginResponse, *AuthError) {
	if req.Account == "" {
		return nil, authErr("param", "请输入账号和密码")
	}
	u, err := s.repo.FindUserByAccount(req.Account)
	if err != nil {
		return nil, authErr("unauth", "账号或密码错误")
	}
	if !auth.Default().CheckPassword(u.PasswordHash, req.Password) {
		return nil, authErr("unauth", "账号或密码错误")
	}
	if !u.Enabled {
		return nil, authErr("forbid", "账号已被停用，请联系管理员")
	}

	token, err := auth.Default().GenerateToken(u.ID, u.TenantID, u.Role, u.Account)
	if err != nil {
		return nil, authErr("server", "签发 token 失败")
	}
	// 登录成功即标记在线（离线状态由 WS 断开或退出登录时翻转）
	_ = s.SetOnline(u.TenantID, u.ID, true)

	cur, err := s.BuildCurrentUser(u)
	if err != nil {
		return nil, authErr("server", err.Error())
	}
	cur.Online = true
	return &dto.LoginResponse{Token: token, User: *cur}, nil
}

// LoadUser 按租户 + 主键取账号（鉴权中间件、个人资料读取用）。
func (s *AccountService) LoadUser(tenantID, userID uint) (*model.User, error) {
	return s.repo.FindUserByID(tenantID, userID)
}

// ---------------- 退出登录 ----------------

// LogoutUser 客服/管理员主动退出登录的三件事，顺序不可调换：
//  1. 置离线（复用 SetOnline）；
//  2. 取消该账号待执行的宽限释放 —— 必须先于第 3 步，
//     否则宽限定时器到点会再释放一次，把退出后被别人重新领取的会话错误踢回待接待；
//  3. 立即释放其接待中的会话（不等 90 秒宽限期）。
//
// 注意：浏览器刷新页面不会调用退出登录，因此不会走到这里，仍按断连的宽限期处理。
// 管理员与客服共用同一套逻辑：释放按会话归属（agent_id）处理，只动自己接待的会话。
func (s *AccountService) LogoutUser(tenantID, userID uint) {
	_ = s.SetOnline(tenantID, userID, false)
	realtime.CancelAgentOffline(userID)
	NewConversationService().ReleaseAgentConversationsNow(tenantID, userID)
}

// UserChannels 返回账号可见的渠道 ID：
// 管理员 = 租户全部渠道；普通客服 = 自己被分配的渠道。
func (s *AccountService) UserChannels(u *model.User) []uint {
	var ids []uint
	if u.Role == "admin" {
		chs, err := s.repo.UserListChannelsByTenant(u.TenantID, u.ID, true)
		if err == nil {
			ids = make([]uint, 0, len(chs))
			for _, ch := range chs {
				ids = append(ids, ch.ID)
			}
		}
	} else {
		ids, _ = s.repo.ListAgentChannelIDsByUser(u.ID)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

// BuildCurrentUser 组装当前登录用户信息（含租户名/Logo 与可见渠道）。
func (s *AccountService) BuildCurrentUser(u *model.User) (*dto.CurrentUser, error) {
	tenant := &model.Tenant{}
	if t, err := s.repo.FindTenantByID(u.TenantID); err == nil {
		tenant = t
	}
	return &dto.CurrentUser{
		ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
		Account: u.Account, Avatar: u.Avatar, Online: u.Online,
		MaxConcurrent: u.MaxConcurrent, Channels: s.UserChannels(u),
		Tenant: dto.TenantBrief{
			// 系统名称优先取后台设置（设置 → 系统名称），未设置时回退到租户名称，
			// 这样运营可以自行改品牌名而不用改库里的 tenants.name。
			ID: tenant.ID, Name: systemNameOr(tenant), Logo: tenant.Logo,
			DefaultImage: setting.DefaultImageURL(tenant.ID),
		},
	}, nil
}

// ---------------- 个人资料 / 密码 ----------------

// UpdateProfile 修改当前登录账号的昵称与头像，并向该账号的所有端广播 profile.updated。
// 返回更新后的账号（供 handler 直接回包）。
func (s *AccountService) UpdateProfile(tenantID, userID uint, name, avatar string) (*model.User, *AuthError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, authErr("param", "昵称不能为空")
	}
	if len([]rune(name)) > 20 {
		return nil, authErr("param", "昵称不能超过 20 个字")
	}
	avatar = strings.TrimSpace(avatar)

	if err := s.repo.UpdateUserProfile(userID, name, avatar); err != nil {
		return nil, authErr("server", "保存失败")
	}

	fresh, err := s.repo.FindUserByID(tenantID, userID)
	if err != nil {
		return nil, authErr("server", "保存失败")
	}

	// 广播资料变更，工作台/后台的头像与昵称即时刷新
	tenant := &model.Tenant{}
	if t, terr := s.repo.FindTenantByID(fresh.TenantID); terr == nil {
		tenant = t
	}
	realtime.HubInstance().Publish(tenantID, []uint{fresh.ID}, realtime.Event{
		Event: "profile.updated",
		Data: map[string]interface{}{
			"id":      fresh.ID,
			"name":    fresh.Name,
			"avatar":  fresh.Avatar,
			"account": fresh.Account,
			"role":    fresh.Role,
			"tenant":  map[string]interface{}{"id": tenant.ID, "name": tenant.Name},
		},
	})
	return fresh, nil
}

// ChangePassword 修改当前登录账号的密码：校验原密码 → 写新哈希。
func (s *AccountService) ChangePassword(tenantID, userID uint, oldPassword, newPassword string) *AuthError {
	if oldPassword == "" || newPassword == "" {
		return authErr("param", "请填写原密码与新密码")
	}
	if len(newPassword) < 6 {
		return authErr("param", "新密码至少 6 位")
	}
	if oldPassword == newPassword {
		return authErr("param", "新密码不能与原密码相同")
	}

	fresh, err := s.repo.FindUserByID(tenantID, userID)
	if err != nil {
		return authErr("unauth", "账号不存在")
	}
	if !auth.Default().CheckPassword(fresh.PasswordHash, oldPassword) {
		return authErr("param", "原密码不正确")
	}
	hash, err := auth.Default().HashPassword(newPassword)
	if err != nil {
		return authErr("server", "密码加密失败")
	}
	if err := s.repo.UpdateUserPassword(userID, hash); err != nil {
		return authErr("server", "密码修改失败")
	}
	return nil
}

// systemNameOr 返回展示用的系统名称：优先后台设置 system.name，未设置时用租户名称。
func systemNameOr(tenant *model.Tenant) string {
	if name := strings.TrimSpace(setting.GetSetting(tenant.ID, setting.SettingSystemName)); name != "" {
		return name
	}
	return tenant.Name
}
