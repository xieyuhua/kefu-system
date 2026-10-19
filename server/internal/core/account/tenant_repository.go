package account

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 租户（Tenant） ----------------

// FindTenantByID 按主键取租户（登录/个人资料里拼租户名与 Logo）。
func (r *Repository) FindTenantByID(id uint) (*model.Tenant, error) {
	var t model.Tenant
	if err := storage.DB().First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
