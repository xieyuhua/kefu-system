package api

import (
	"strings"

	"kefu-server/internal/auth"
	"kefu-server/internal/model"
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

	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.Param(c, "昵称不能为空")
		return
	}
	if len([]rune(name)) > 20 {
		response.Param(c, "昵称不能超过 20 个字")
		return
	}
	avatar := strings.TrimSpace(req.Avatar)

	updates := map[string]interface{}{"name": name}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	if err := db.Model(&model.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
		response.Server(c, "保存失败")
		return
	}

	// 广播资料变更，工作台/后台的头像与昵称即时刷新
	var fresh model.User
	db.First(&fresh, u.ID)
	var tenant model.Tenant
	db.First(&tenant, u.TenantID)
	service.HubInstance().Publish(tenantID(c), []uint{u.ID}, service.Event{
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
	if req.OldPassword == "" || req.NewPassword == "" {
		response.Param(c, "请填写原密码与新密码")
		return
	}
	if len(req.NewPassword) < 6 {
		response.Param(c, "新密码至少 6 位")
		return
	}
	if req.OldPassword == req.NewPassword {
		response.Param(c, "新密码不能与原密码相同")
		return
	}

	var fresh model.User
	if err := db.First(&fresh, u.ID).Error; err != nil {
		response.Unauth(c, "账号不存在")
		return
	}
	if !auth.CheckPassword(fresh.PasswordHash, req.OldPassword) {
		response.Param(c, "原密码不正确")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Server(c, "密码加密失败")
		return
	}
	if err := db.Model(&model.User{}).Where("id = ?", u.ID).
		Update("password_hash", hash).Error; err != nil {
		response.Server(c, "密码修改失败")
		return
	}
	response.OK(c, true)
}
