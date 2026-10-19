package account

// 本文件是 account 模块的 HTTP 层（原 handler/auth.go + profile.go + base.go 的渠道/子账号部分）。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。
// 命名约定：同包内 service 与 handler 会重名（如 Login），因此所有 HTTP 处理器统一带 Handler 后缀。

import (
	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/cache"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/httpx"
	"kefu-server/internal/platform/response"

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
	u := httpx.CurrentUser(c)
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
	if u := httpx.CurrentUser(c); u != nil {
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
	u := httpx.CurrentUser(c)
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
	u := httpx.CurrentUser(c)
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

// ---------------- 子账号 ----------------

// ListAgentsHandler 客服列表。
//   - admin：看全部
//   - agent：带 ?scope=transfer 时可看「与自己接待渠道有交集」的在线客服（用于会话转接）
func ListAgentsHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) && c.Query("scope") != "transfer" {
		response.Forbid(c, "仅管理员可查看")
		return
	}
	list, err := ListAgents(httpx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if !httpx.IsAdmin(c) {
		// 非管理员只返回在线、且未停用的同事
		filtered := make([]dto.AgentVO, 0, len(list))
		for _, a := range list {
			if a.Online && a.Enabled && a.ID != httpx.UserID(c) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}
	response.OK(c, list)
}

func CreateAgentHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := CreateAgent(httpx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAgentHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := UpdateAgent(httpx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAgentHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := DeleteAgent(httpx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func SetOnlineHandler(c *gin.Context) {
	var body struct {
		Online bool `json:"online"`
	}
	_ = c.ShouldBindJSON(&body)
	target := httpx.UintParam(c, "id")
	cl := httpx.ClaimsFromContext(c)
	if cl == nil {
		return
	}
	if target != cl.UserID && cl.Role != "admin" {
		response.Forbid(c, "只能修改自己的在线状态")
		return
	}
	if err := SetOnline(httpx.TenantID(c), target, body.Online); err != nil {
		response.Server(c, err.Error())
		return
	}
	status := "offline"
	if body.Online {
		status = "online"
	}
	response.OK(c, map[string]interface{}{"online": body.Online, "status": status})
}
