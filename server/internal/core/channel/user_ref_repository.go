package channel

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ListTenantAgents 列出租户下所有客服子账号（role = agent，按 id 升序）。
func (r *Repository) ListTenantAgents(tenantID uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ? AND role = 'agent'", tenantID).
		Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindAgentUsersByIDs 按主键批量取租户内的客服账号，用于校验账号归属本租户。
func (r *Repository) FindAgentUsersByIDs(tenantID uint, ids []uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}
