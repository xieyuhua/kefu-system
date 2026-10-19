package customer

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 跨表只读：渠道 / 账号 ----------------

// findChannelByID 按租户 + 主键只读一条渠道（客户列表/详情回显渠道名与类型）。
func (r *Repository) findChannelByID(tenantID, id uint) (*model.Channel, error) {
	var c model.Channel
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// findUserBriefByID 按主键只读账号的展示字段（历史会话里的接待客服）。
func (r *Repository) findUserBriefByID(userID uint) (*model.User, error) {
	var u model.User
	if err := storage.DB().Select("id", "name", "role", "avatar", "online", "max_concurrent").
		Where("id = ?", userID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FirstTenantID 返回第一个租户的 ID。
// 用户端未登录、未显式传 ?tenant= 时用它兜底，便于演示环境直接打开链接。
func (r *Repository) FirstTenantID() (uint, error) {
	var t model.Tenant
	if err := storage.DB().Order("id asc").First(&t).Error; err != nil {
		return 0, err
	}
	return t.ID, nil
}
