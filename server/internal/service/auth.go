package service

import (
	"kefu-server/internal/auth"
	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

// AuthError 带 HTTP 语义的鉴权错误，供 handler 直接映射为响应（param/unauth/forbid/server）。
type AuthError struct {
	Status string
	Msg    string
}

func (e *AuthError) Error() string { return e.Msg }

func authErr(status, msg string) *AuthError { return &AuthError{Status: status, Msg: msg} }

// Login 校验账号密码并签发 token。
// 返回的 *AuthError 由 handler 按 Status 映射响应（账号或密码错误 → 401，停用 → 403）。
func Login(req dto.LoginRequest) (*dto.LoginResponse, *AuthError) {
	if req.Account == "" {
		return nil, authErr("param", "请输入账号和密码")
	}
	u, err := repository.FindUserByAccount(req.Account)
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
	return repository.FindUserByID(tenantID, userID)
}

// UserChannels 返回账号可见的渠道 ID：
// 管理员 = 租户全部渠道；普通客服 = 自己被分配的渠道。
func UserChannels(u *model.User) []uint {
	var ids []uint
	if u.Role == "admin" {
		chs, err := repository.ListChannelsByTenant(u.TenantID, u.ID, true)
		if err == nil {
			ids = make([]uint, 0, len(chs))
			for _, ch := range chs {
				ids = append(ids, ch.ID)
			}
		}
	} else {
		ids, _ = repository.ListAgentChannelIDsByUser(u.ID)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

// BuildCurrentUser 组装当前登录用户信息（含租户名/Logo 与可见渠道）。
func BuildCurrentUser(u *model.User) (*dto.CurrentUser, error) {
	var tenant model.Tenant
	if t, err := repository.FindTenantByID(u.TenantID); err == nil {
		tenant = *t
	}
	return &dto.CurrentUser{
		ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
		Account: u.Account, Avatar: u.Avatar, Online: u.Online,
		MaxConcurrent: u.MaxConcurrent, Channels: UserChannels(u),
		Tenant: dto.TenantBrief{
			ID: tenant.ID, Name: tenant.Name, Logo: tenant.Logo,
			DefaultImage: DefaultImageURL(tenant.ID),
		},
	}, nil
}
