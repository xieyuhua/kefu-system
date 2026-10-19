package model

// Setting 租户级系统配置（键值对）
// 用于「默认图片」等可后台维护的全局设置
type Setting struct {
	Model
	TenantID uint   `gorm:"index;not null;uniqueIndex:idx_tenant_key" json:"tenant_id"`
	Key      string `gorm:"size:64;not null;uniqueIndex:idx_tenant_key" json:"key"`
	Value    string `gorm:"size:1024" json:"value"`
	Remark   string `gorm:"size:128" json:"remark"`
}
