package handler

import (
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

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
func authFail(c *gin.Context, e *service.AuthError) {
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

// UpdateProfile 修改当前登录账号的昵称与头像
func UpdateProfile(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		response.Unauth(c, "未登录")
		return
	}
	var req UpdateProfileRequest
	if !bindJSON(c, &req) {
		return
	}
	fresh, aerr := service.UpdateProfile(u.TenantID, u.ID, req.Name, req.Avatar)
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

// ChangePassword 修改当前登录账号的密码
func ChangePassword(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		response.Unauth(c, "未登录")
		return
	}
	var req ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if aerr := service.ChangePassword(u.TenantID, u.ID, req.OldPassword, req.NewPassword); aerr != nil {
		authFail(c, aerr)
		return
	}
	response.OK(c, true)
}
