package model

// AgentChannel 子账号可接待的渠道（多对多）
type AgentChannel struct {
	UserID    uint `gorm:"primaryKey;index" json:"user_id"`
	ChannelID uint `gorm:"primaryKey;index" json:"channel_id"`
}
