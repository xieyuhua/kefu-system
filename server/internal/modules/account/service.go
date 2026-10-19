// Package account 账号模块：后台账号（users）、渠道分配关系（agent_channels）、
// 登录鉴权与个人资料。模块内按 handler / service / repository 分层。
//
// 依赖方向：account 只依赖 platform（不依赖其它业务模块）。
package account

import (
	"errors"
	"strings"

	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/setting"
)

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

// orDefault 空串取默认值（渠道名/昵称兜底）
func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// ---------------- 登录 ----------------

// Login 校验账号密码并签发 token。
// 返回的 *AuthError 由 handler 按 Status 映射响应（账号或密码错误 → 401，停用 → 403）。
func Login(req dto.LoginRequest) (*dto.LoginResponse, *AuthError) {
	if req.Account == "" {
		return nil, authErr("param", "请输入账号和密码")
	}
	u, err := FindUserByAccount(req.Account)
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
	_ = SetOnline(u.TenantID, u.ID, true)

	cur, err := BuildCurrentUser(u)
	if err != nil {
		return nil, authErr("server", err.Error())
	}
	cur.Online = true
	return &dto.LoginResponse{Token: token, User: *cur}, nil
}

// LoadUser 按租户 + 主键取账号（鉴权中间件、个人资料读取用）。
func LoadUser(tenantID, userID uint) (*model.User, error) {
	return FindUserByID(tenantID, userID)
}

// UserChannels 返回账号可见的渠道 ID：
// 管理员 = 租户全部渠道；普通客服 = 自己被分配的渠道。
func UserChannels(u *model.User) []uint {
	var ids []uint
	if u.Role == "admin" {
		chs, err := listChannelsByTenant(u.TenantID, u.ID, true)
		if err == nil {
			ids = make([]uint, 0, len(chs))
			for _, ch := range chs {
				ids = append(ids, ch.ID)
			}
		}
	} else {
		ids, _ = ListAgentChannelIDsByUser(u.ID)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

// BuildCurrentUser 组装当前登录用户信息（含租户名/Logo 与可见渠道）。
func BuildCurrentUser(u *model.User) (*dto.CurrentUser, error) {
	var tenant model.Tenant
	if t, err := FindTenantByID(u.TenantID); err == nil {
		tenant = *t
	}
	return &dto.CurrentUser{
		ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
		Account: u.Account, Avatar: u.Avatar, Online: u.Online,
		MaxConcurrent: u.MaxConcurrent, Channels: UserChannels(u),
		Tenant: dto.TenantBrief{
			ID: tenant.ID, Name: tenant.Name, Logo: tenant.Logo,
			DefaultImage: setting.DefaultImageURL(tenant.ID),
		},
	}, nil
}

// ---------------- 子账号（Agent） ----------------

func ListAgents(tenantID uint) ([]dto.AgentVO, error) {
	users, err := ListUsersByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AgentVO, 0, len(users))
	for _, u := range users {
		vo := dto.AgentVO{
			ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
			Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
			Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
		}
		vo.Status = "offline"
		if u.Online {
			vo.Status = "online"
		}
		// 该账号可接待的渠道；查询失败不阻断列表展示，保持空渠道列表（与原逻辑一致）
		if chs, err := listChannelsByTenant(tenantID, u.ID, false); err == nil {
			for _, ch := range chs {
				vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
			}
		}
		// 进行中会话数；统计失败按 0 处理（与原逻辑一致）
		if active, err := CountOpenConversations(tenantID, u.ID); err == nil {
			vo.ActiveCount = active
		}
		out = append(out, vo)
	}
	return out, nil
}

func CreateAgent(tenantID uint, req dto.AgentRequest) (*dto.AgentVO, error) {
	if req.Account == "" || req.Password == "" {
		return nil, errors.New("账号与密码不能为空")
	}
	// 账号全局唯一；统计失败按「未被占用」处理（与原逻辑一致）
	if cnt, _ := CountUsersByAccount(req.Account, 0); cnt > 0 {
		return nil, errors.New("账号已存在")
	}
	hash, err := hashPwd(req.Password)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role != "admin" {
		role = "agent"
	}
	maxC := req.MaxConcurrent
	if maxC <= 0 {
		maxC = 5
	}
	u := model.User{
		TenantID:      tenantID,
		Account:       req.Account,
		PasswordHash:  hash,
		Name:          orDefault(req.Name, req.Account),
		Role:          role,
		Avatar:        "/demo/avatar-2.svg",
		MaxConcurrent: maxC,
		Enabled:       true,
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := CreateUser(&u); err != nil {
		return nil, err
	}
	setAgentChannels(tenantID, u.ID, req.ChannelIDs)
	return getAgentVO(tenantID, u.ID)
}

func UpdateAgent(tenantID, id uint, req dto.AgentRequest) error {
	u, err := FindUserByID(tenantID, id)
	if err != nil {
		return errors.New("账号不存在")
	}
	if req.Name != "" {
		u.Name = req.Name
	}
	if req.Role != "" {
		u.Role = req.Role
	}
	if req.MaxConcurrent > 0 {
		u.MaxConcurrent = req.MaxConcurrent
	}
	if req.Password != "" {
		h, err := hashPwd(req.Password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := SaveUser(u); err != nil {
		return err
	}
	if req.ChannelIDs != nil {
		setAgentChannels(tenantID, id, req.ChannelIDs)
	}
	return nil
}

func DeleteAgent(tenantID, id uint) error {
	// 账号删除与渠道分配清理放在同一事务里，避免残留孤儿关联
	return DeleteUserWithBindings(tenantID, id)
}

func SetOnline(tenantID, id uint, online bool) error {
	return UpdateUserOnline(tenantID, id, online)
}

func setAgentChannels(tenantID, userID uint, ids []uint) {
	// 先清空该账号的渠道分配；删除失败不阻断主流程（与原逻辑一致）
	_ = DeleteAgentChannelsByUser(userID)
	// 只允许分配到本租户自己的渠道：isAdmin 传 true 表示取租户全部渠道
	valid, _ := listChannelsByTenant(tenantID, 0, true)
	allowed := map[uint]bool{}
	for _, c := range valid {
		allowed[c.ID] = true
	}
	for _, id := range ids {
		if !allowed[id] {
			continue
		}
		_ = createAgentChannel(&model.AgentChannel{UserID: userID, ChannelID: id})
	}
}

func getAgentVO(tenantID, id uint) (*dto.AgentVO, error) {
	u, err := FindUserByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	vo := &dto.AgentVO{
		ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
		Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
		Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
	}
	vo.Status = "offline"
	if u.Online {
		vo.Status = "online"
	}
	// 该账号可接待的渠道；查询失败保持空列表（与原逻辑一致）
	if chs, err := listChannelsByTenant(tenantID, id, false); err == nil {
		for _, ch := range chs {
			vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
		}
	}
	return vo, nil
}

// ---------------- 个人资料 / 密码 ----------------

// UpdateProfile 修改当前登录账号的昵称与头像，并向该账号的所有端广播 profile.updated。
// 返回更新后的账号（供 handler 直接回包）。
func UpdateProfile(tenantID, userID uint, name, avatar string) (*model.User, *AuthError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, authErr("param", "昵称不能为空")
	}
	if len([]rune(name)) > 20 {
		return nil, authErr("param", "昵称不能超过 20 个字")
	}
	avatar = strings.TrimSpace(avatar)

	if err := UpdateUserProfile(userID, name, avatar); err != nil {
		return nil, authErr("server", "保存失败")
	}

	fresh, err := FindUserByID(tenantID, userID)
	if err != nil {
		return nil, authErr("server", "保存失败")
	}

	// 广播资料变更，工作台/后台的头像与昵称即时刷新
	var tenant model.Tenant
	if t, terr := FindTenantByID(fresh.TenantID); terr == nil {
		tenant = *t
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
func ChangePassword(tenantID, userID uint, oldPassword, newPassword string) *AuthError {
	if oldPassword == "" || newPassword == "" {
		return authErr("param", "请填写原密码与新密码")
	}
	if len(newPassword) < 6 {
		return authErr("param", "新密码至少 6 位")
	}
	if oldPassword == newPassword {
		return authErr("param", "新密码不能与原密码相同")
	}

	fresh, err := FindUserByID(tenantID, userID)
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
	if err := UpdateUserPassword(userID, hash); err != nil {
		return authErr("server", "密码修改失败")
	}
	return nil
}
