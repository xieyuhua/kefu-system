package repository

import (
	"kefu-server/internal/model"
)

// ---------------- 渠道类型库（ChannelType）数据访问 ----------------

// FindChannelTypeByCode 按租户 + 类型编码查询渠道类型；未命中返回 gorm.ErrRecordNotFound。
func FindChannelTypeByCode(tenantID uint, code string) (*model.ChannelType, error) {
	var t model.ChannelType
	if err := db.Where("tenant_id = ? AND code = ?", tenantID, code).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// FindChannelTypeByID 按租户 + 主键查询渠道类型；未命中返回 gorm.ErrRecordNotFound。
func FindChannelTypeByID(tenantID, id uint) (*model.ChannelType, error) {
	var t model.ChannelType
	if err := db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListChannelTypesByTenant 列出租户的渠道类型库（先按 sort 再按 id 升序）。
func ListChannelTypesByTenant(tenantID uint) ([]model.ChannelType, error) {
	var list []model.ChannelType
	if err := db.Where("tenant_id = ?", tenantID).
		Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountChannelsOfType 统计该类型编码下已有的渠道数（删除前校验是否仍被引用）。
func CountChannelsOfType(tenantID uint, code string) (int64, error) {
	var cnt int64
	if err := db.Model(&model.Channel{}).
		Where("tenant_id = ? AND type = ?", tenantID, code).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CreateChannelType 新增渠道类型，成功后回填主键与时间戳。
func CreateChannelType(t *model.ChannelType) error {
	return db.Create(t).Error
}

// SaveChannelType 保存渠道类型的全字段（名称/编码/颜色/图标/排序）。
func SaveChannelType(t *model.ChannelType) error {
	return db.Save(t).Error
}

// DeleteChannelType 按租户 + 主键删除渠道类型。
func DeleteChannelType(tenantID, id uint) error {
	return db.Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&model.ChannelType{}).Error
}
