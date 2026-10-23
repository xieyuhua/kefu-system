package model

import (
	"time"
)

// Conversation 会话
type Conversation struct {
	Model
	TenantID      uint       `gorm:"index;not null" json:"tenant_id"`
	ChannelID     uint       `gorm:"index;not null" json:"channel_id"`
	CustomerID    uint       `gorm:"index;not null" json:"customer_id"`
	AgentID       uint       `gorm:"index" json:"agent_id"`                   // 0 表示待接待
	Status        string     `gorm:"size:16;default:'pending'" json:"status"` // pending | open | closed
	Unread        int        `gorm:"default:0" json:"unread"`
	LastMessage   string     `gorm:"size:512" json:"last_message"`
	LastMessageAt time.Time  `gorm:"index" json:"last_message_at"`
	Summary       string     `gorm:"size:255" json:"summary"`
	MessageCount  int        `gorm:"default:0" json:"message_count"`
	ClosedAt      *time.Time `json:"closed_at"`
	// Archived 归档：客服主动清理的工作台会话。归档后不出现在工作台列表，
	// 客户再次发消息会自动取消归档并重新变为待接待（消息不丢失）。
	// HandledByHuman 该会话是否已被人工接待过（接入或回复）。
	// 用于「机器人只回答尚未被接待的咨询」：客服离线上会话被释放回待接待后，
	// 机器人仍然不会抢答，只有用户重新发起新一轮咨询（结束/归档后再来）才恢复自动回复。
	HandledByHuman bool `gorm:"default:false" json:"handled_by_human"`

	Archived   bool       `gorm:"default:false;index" json:"archived"`
	ArchivedAt *time.Time `json:"archived_at"`
}
