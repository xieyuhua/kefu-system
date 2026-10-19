package model

// Tenant 租户（SAAS 的一个商家）
type Tenant struct {
	Model
	Name   string `gorm:"size:64;not null" json:"name"`
	Logo   string `gorm:"size:255" json:"logo"`
	Status int    `gorm:"default:1" json:"status"` // 1 正常 0 停用
}
