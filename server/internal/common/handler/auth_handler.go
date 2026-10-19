package handler

// 本文件是公共 HTTP 层里的账号部分：登录 / 登出 / 当前用户 / 改资料 / 改密码。
// handler 层不持有数据库连接：参数绑定 → 调公共 service → 写响应。
// 命名约定：同包内 service 与 handler 会重名（如 Login），因此所有 HTTP 处理器统一带 Handler 后缀，
// 并且全部是 *AuthHandler 的方法（指针接收者），实例由 internal/common/route 在注册时创建一次。
//
// 跨端端点（客服端 agent 与后台端 admin 各自登录页/资料弹窗都调）：
// LoginHandler / MeHandler / LogoutHandler / UpdateProfileHandler / ChangePasswordHandler
// —— 三端共用同一套口令与 token 语义，复制到端内只会让鉴权口径漂移，故留在公共层。

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/auth"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	"kefu-server/internal/common/service"
)

// AuthHandler 公共账号服务的 HTTP 层。
type AuthHandler struct {
	svc *service.AccountService
}

// NewAuthHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewAuthHandler() *AuthHandler { return &AuthHandler{svc: service.NewAccountService()} }

// ---------------- 认证 ----------------

// LoginHandler 登录：鉴权与用户信息组装都在 service 层，这里只负责参数绑定与响应映射。
func (h *AuthHandler) LoginHandler(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Default().Param(c, "请输入账号和密码")
		return
	}
	res, aerr := h.svc.Login(req)
	if aerr != nil {
		switch aerr.Status {
		case "param":
			response.Default().Param(c, aerr.Msg)
		case "unauth":
			response.Default().Unauth(c, aerr.Msg)
		case "forbid":
			response.Default().Forbid(c, aerr.Msg)
		default:
			response.Default().Server(c, aerr.Msg)
		}
		return
	}
	response.Default().OK(c, res)
}

// MeHandler 当前登录用户信息（前端刷新后恢复登录态用）。
func (h *AuthHandler) MeHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		return
	}
	cur, err := h.svc.BuildCurrentUser(u)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, cur)
}

// LogoutHandler 退出登录：吊销 token 并置离线。
func (h *AuthHandler) LogoutHandler(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if len(token) > 7 {
		token = token[7:]
	}
	if token != "" {
		auth.Default().RevokeToken(token)
	}
	if u := authctx.CurrentUser(c); u != nil {
		_ = h.svc.SetOnline(u.TenantID, u.ID, false)
		cache.Default().Get().Del(cache.Default().OnlineKey(u.TenantID, u.ID))
	}
	response.Default().OK(c, true)
}

// ---------------- 个人资料 ----------------

// UpdateProfileRequest 修改个人资料
type UpdateProfileRequest struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// ChangePasswordRequest 修改密码
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// authFail 把 service 返回的鉴权错误映射为响应（纯工具函数，无状态）
func authFail(c *gin.Context, e *service.AuthError) {
	switch e.Status {
	case "param":
		response.Default().Param(c, e.Msg)
	case "unauth":
		response.Default().Unauth(c, e.Msg)
	case "forbid":
		response.Default().Forbid(c, e.Msg)
	default:
		response.Default().Server(c, e.Msg)
	}
}

// UpdateProfileHandler 修改当前登录账号的昵称与头像
func (h *AuthHandler) UpdateProfileHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		response.Default().Unauth(c, "未登录")
		return
	}
	var req UpdateProfileRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	fresh, aerr := h.svc.UpdateProfile(u.TenantID, u.ID, req.Name, req.Avatar)
	if aerr != nil {
		authFail(c, aerr)
		return
	}
	response.Default().OK(c, map[string]interface{}{
		"id":      fresh.ID,
		"name":    fresh.Name,
		"avatar":  fresh.Avatar,
		"account": fresh.Account,
		"role":    fresh.Role,
	})
}

// ChangePasswordHandler 修改当前登录账号的密码
func (h *AuthHandler) ChangePasswordHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		response.Default().Unauth(c, "未登录")
		return
	}
	var req ChangePasswordRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if aerr := h.svc.ChangePassword(u.TenantID, u.ID, req.OldPassword, req.NewPassword); aerr != nil {
		authFail(c, aerr)
		return
	}
	response.Default().OK(c, true)
}
