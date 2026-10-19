// Package account 账号内核：后台账号（users）、渠道分配关系（agent_channels）、
// 登录鉴权与个人资料。模块内按 handler / service / repository 分层。
//
// 依赖方向：core/account → platform → pkg（不依赖任何业务端模块）。
// 端侧（modules/client、modules/agent、modules/admin）都可以依赖本包，
// 本包不得反向 import 端侧模块。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），Service 在构造时建好自己的
// Repository 依赖；只有无状态的纯工具函数（authErr / hashPwd / orDefault）保留为包级函数。
package account

import (
	"strings"

	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/setting"
)

// Service 账号内核的业务编排层。
type Service struct {
	repo *Repository
}

// NewService 创建业务层实例（自带数据访问层依赖）。
func NewService() *Service { return &Service{repo: NewRepository()} }

// init 把「按租户 + 账号 ID 载入账号」的能力注入 platform/authctx。
// platform 不反向依赖业务内核，authctx.CurrentUser 只能靠这个回调拿到账号实体，
// 因此注册点落在账号内核自己的 init 里（route 会 import 本包，保证注册被执行）。
func init() {
	authctx.SetUserLoader(NewService().LoadUser)
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
func hashPwd(p string) (string, error) { return auth.HashPassword(p) }

// ---------------- 登录 ----------------

// Login 校验账号密码并签发 token。
// 返回的 *AuthError 由 handler 按 Status 映射响应（账号或密码错误 → 401，停用 → 403）。
func (s *Service) Login(req dto.LoginRequest) (*dto.LoginResponse, *AuthError) {
	if req.Account == "" {
		return nil, authErr("param", "请输入账号和密码")
	}
	u, err := s.repo.FindUserByAccount(req.Account)
	if err != nil {
		return nil, authErr("unauth", "账号或密码错误")
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		return nil, authErr("unauth", "账号或密码错误")
	}
	if !u.Enabled {
		return nil, authErr("forbid", "账号已被停用，请联系管理员")
	}

	token, err := auth.GenerateToken(u.ID, u.TenantID, u.Role, u.Account)
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
func (s *Service) LoadUser(tenantID, userID uint) (*model.User, error) {
	return s.repo.FindUserByID(tenantID, userID)
}

// UserChannels 返回账号可见的渠道 ID：
// 管理员 = 租户全部渠道；普通客服 = 自己被分配的渠道。
func (s *Service) UserChannels(u *model.User) []uint {
	var ids []uint
	if u.Role == "admin" {
		chs, err := s.repo.listChannelsByTenant(u.TenantID, u.ID, true)
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
func (s *Service) BuildCurrentUser(u *model.User) (*dto.CurrentUser, error) {
	tenant := &model.Tenant{}
	if t, err := s.repo.FindTenantByID(u.TenantID); err == nil {
		tenant = t
	}
	return &dto.CurrentUser{
		ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
		Account: u.Account, Avatar: u.Avatar, Online: u.Online,
		MaxConcurrent: u.MaxConcurrent, Channels: s.UserChannels(u),
		Tenant: dto.TenantBrief{
			ID: tenant.ID, Name: tenant.Name, Logo: tenant.Logo,
			DefaultImage: setting.DefaultImageURL(tenant.ID),
		},
	}, nil
}

// ---------------- 个人资料 / 密码 ----------------

// UpdateProfile 修改当前登录账号的昵称与头像，并向该账号的所有端广播 profile.updated。
// 返回更新后的账号（供 handler 直接回包）。
func (s *Service) UpdateProfile(tenantID, userID uint, name, avatar string) (*model.User, *AuthError) {
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
func (s *Service) ChangePassword(tenantID, userID uint, oldPassword, newPassword string) *AuthError {
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
	if !auth.CheckPassword(fresh.PasswordHash, oldPassword) {
		return authErr("param", "原密码不正确")
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return authErr("server", "密码加密失败")
	}
	if err := s.repo.UpdateUserPassword(userID, hash); err != nil {
		return authErr("server", "密码修改失败")
	}
	return nil
}
