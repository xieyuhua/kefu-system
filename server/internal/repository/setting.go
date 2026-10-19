package repository

import (
	"kefu-server/internal/model"
)

// ---------------- 租户系统配置（Setting）数据访问 ----------------

// FindSettingByKey 按租户 + key 查询单个配置项；未命中返回 gorm.ErrRecordNotFound。
func FindSettingByKey(tenantID uint, key string) (*model.Setting, error) {
	var s model.Setting
	if err := db.Where("tenant_id = ? AND key = ?", tenantID, key).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSettingsByTenant 列出租户已保存的全部配置项（保持原有不排序的取数方式）。
func ListSettingsByTenant(tenantID uint) ([]*model.Setting, error) {
	var list []*model.Setting
	if err := db.Where("tenant_id = ?", tenantID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CreateSetting 新增配置项，成功后回填主键与时间戳。
func CreateSetting(s *model.Setting) error {
	return db.Create(s).Error
}

// SaveSetting 保存配置项全字段（已存在配置的更新路径）。
func SaveSetting(s *model.Setting) error {
	return db.Save(s).Error
}
