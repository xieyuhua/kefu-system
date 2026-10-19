package model

import (
	"time"
)

// Customer 客户（咨询者）
type Customer struct {
	Model
	TenantID   uint       `gorm:"index;not null" json:"tenant_id"`
	ChannelID  uint       `gorm:"index" json:"channel_id"`
	ExternalID string     `gorm:"size:128;index" json:"external_id"` // 渠道侧 openid
	Nickname   string     `gorm:"size:64" json:"nickname"`
	Avatar     string     `gorm:"size:255" json:"avatar"`
	Phone      string     `gorm:"size:32" json:"phone"`
	IP         string     `gorm:"size:64;index" json:"ip"` // 最近一次访问 IP
	City       string     `gorm:"size:64" json:"city"`     // IP 归属城市，如「广东 深圳」
	Remark     string     `gorm:"size:255" json:"remark"`  // 客服备注
	LastSeenAt *time.Time `json:"last_seen_at"`
}
