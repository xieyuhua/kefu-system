package model

// AutoReply 自动回复规则：关键词命中后机器人回复，或直接转人工
type AutoReply struct {
	Model
	TenantID        uint   `gorm:"index;not null" json:"tenant_id"`
	ChannelID       uint   `gorm:"index" json:"channel_id"` // 0 = 全渠道
	Name            string `gorm:"size:64" json:"name"`
	MatchType       string `gorm:"size:16;default:'contain'" json:"match_type"` // contain|equal|prefix
	Keywords        string `gorm:"size:512" json:"keywords"`                    // 逗号分隔
	Reply           string `gorm:"type:text" json:"reply"`
	ReplyType       string `gorm:"size:16;default:'text'" json:"reply_type"`
	TransferToHuman bool   `gorm:"default:false" json:"transfer_to_human"`
	Enabled         bool   `gorm:"default:true" json:"enabled"`
	Priority        int    `gorm:"default:0" json:"priority"`
	HitCount        int64  `gorm:"default:0" json:"hit_count"`
}
