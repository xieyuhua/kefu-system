package service

import (
	"strings"

	"kefu-server/internal/auth"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

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

	if err := repository.UpdateUserProfile(userID, name, avatar); err != nil {
		return nil, authErr("server", "保存失败")
	}

	fresh, err := repository.FindUserByID(tenantID, userID)
	if err != nil {
		return nil, authErr("server", "保存失败")
	}

	// 广播资料变更，工作台/后台的头像与昵称即时刷新
	var tenant model.Tenant
	if t, terr := repository.FindTenantByID(fresh.TenantID); terr == nil {
		tenant = *t
	}
	HubInstance().Publish(tenantID, []uint{fresh.ID}, Event{
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

	fresh, err := repository.FindUserByID(tenantID, userID)
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
	if err := repository.UpdateUserPassword(userID, hash); err != nil {
		return authErr("server", "密码修改失败")
	}
	return nil
}
