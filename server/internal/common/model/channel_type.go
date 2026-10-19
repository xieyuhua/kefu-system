package model

// ChannelType 渠道类型库：类型完全自定义、不限制枚举。
// 租户可新增任意类型（如 wechat / wecom / 小红书 / 快手 / 线下门店），并指定展示名与颜色。
type ChannelType struct {
	Model
	TenantID  uint   `gorm:"index;not null" json:"tenant_id"`
	Code      string `gorm:"size:32;not null" json:"code"` // 英文标识，如 wechat / xiaohongshu
	Name      string `gorm:"size:64;not null" json:"name"` // 显示名，如 微信客服 / 小红书
	Color     string `gorm:"size:16;default:'#07C160'" json:"color"`
	Icon      string `gorm:"size:64" json:"icon"` // emoji 或图标名
	Sort      int    `gorm:"default:0" json:"sort"`
	BuiltIn   bool   `gorm:"default:false" json:"built_in"`
	UsedCount int64  `gorm:"-" json:"used_count"`
}
