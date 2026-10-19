package account

// 本文件是 core/account 的 HTTP 层。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。
// 命名约定：同包内 service 与 handler 会重名（如 Login），因此所有 HTTP 处理器统一带 Handler 后缀。
//
// 跨端端点（客服端 modules/agent 与后台端 modules/admin 各自登录页/资料弹窗都调）：
// LoginHandler / MeHandler / LogoutHandler / UpdateProfileHandler / ChangePasswordHandler
// —— 三端共用同一套口令与 token 语义，复制到端内只会让鉴权口径漂移，故留在账号内核。

import (
	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 认证 ----------------

// LoginHandler 登录：鉴权与用户信息组装都在 service 层，这里只负责参数绑定与响应映射。
func LoginHandler(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Param(c, "请输入账号和密码")
		return
	}
	res, aerr := Login(req)
	if aerr != nil {
		switch aerr.Status {
		case "param":
			response.Param(c, aerr.Msg)
		case "unauth":
			response.Unauth(c, aerr.Msg)
		case "forbid":
			response.Forbid(c, aerr.Msg)
		default:
			response.Server(c, aerr.Msg)
		}
		return
	}
	response.OK(c, res)
}

// MeHandler 当前登录用户信息（前端刷新后恢复登录态用）。
func MeHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		return
	}
	cur, err := BuildCurrentUser(u)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, cur)
}

// LogoutHandler 退出登录：吊销 token 并置离线。
func LogoutHandler(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if len(token) > 7 {
		token = token[7:]
	}
	if token != "" {
		auth.RevokeToken(token)
	}
	if u := authctx.CurrentUser(c); u != nil {
		_ = SetOnline(u.TenantID, u.ID, false)
		cache.Get().Del(cache.OnlineKey(u.TenantID, u.ID))
	}
	response.OK(c, true)
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

// authFail 把 service 返回的鉴权错误映射为响应
func authFail(c *gin.Context, e *AuthError) {
	switch e.Status {
	case "param":
		response.Param(c, e.Msg)
	case "unauth":
		response.Unauth(c, e.Msg)
	case "forbid":
		response.Forbid(c, e.Msg)
	default:
		response.Server(c, e.Msg)
	}
}

// UpdateProfileHandler 修改当前登录账号的昵称与头像
func UpdateProfileHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		response.Unauth(c, "未登录")
		return
	}
	var req UpdateProfileRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	fresh, aerr := UpdateProfile(u.TenantID, u.ID, req.Name, req.Avatar)
	if aerr != nil {
		authFail(c, aerr)
		return
	}
	response.OK(c, map[string]interface{}{
		"id":      fresh.ID,
		"name":    fresh.Name,
		"avatar":  fresh.Avatar,
		"account": fresh.Account,
		"role":    fresh.Role,
	})
}

// ChangePasswordHandler 修改当前登录账号的密码
func ChangePasswordHandler(c *gin.Context) {
	u := authctx.CurrentUser(c)
	if u == nil {
		response.Unauth(c, "未登录")
		return
	}
	var req ChangePasswordRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if aerr := ChangePassword(u.TenantID, u.ID, req.OldPassword, req.NewPassword); aerr != nil {
		authFail(c, aerr)
		return
	}
	response.OK(c, true)
}
