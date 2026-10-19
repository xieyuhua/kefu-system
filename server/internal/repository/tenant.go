package repository

import "kefu-server/internal/model"

// ---------------- 租户（Tenant） ----------------

// FindTenantByID 按主键取租户（登录/个人资料里拼租户名与 Logo）。
func FindTenantByID(id uint) (*model.Tenant, error) {
	var t model.Tenant
	if err := DB().First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
