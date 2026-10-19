package model

import (
	"time"
)

// Tag 租户级客户标签库（管理员维护，客服可选用）
type Tag struct {
	Model
	TenantID uint   `gorm:"index;not null" json:"tenant_id"`
	Name     string `gorm:"size:32;not null" json:"name"`
	Color    string `gorm:"size:16;default:'blue'" json:"color"` // blue|green|orange|red|gray
	Sort     int    `gorm:"default:0" json:"sort"`
}

// CustomerTag 客户标签关联（多对多），带操作人与时间，便于追溯是谁打的标
type CustomerTag struct {
	TenantID   uint      `gorm:"primaryKey;index" json:"tenant_id"`
	CustomerID uint      `gorm:"primaryKey;index" json:"customer_id"`
	TagID      uint      `gorm:"primaryKey;index" json:"tag_id"`
	CreatedBy  uint      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}
