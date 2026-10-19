package model

// Notification 系统提醒（离线期间堆积，供 APP/PC 端重连后拉取）
type Notification struct {
	Model
	TenantID       uint   `gorm:"index" json:"tenant_id"`
	UserID         uint   `gorm:"index;not null" json:"user_id"`
	Type           string `gorm:"size:32" json:"type"` // conversation.assigned | conversation.transferred | message.new
	Title          string `gorm:"size:128" json:"title"`
	Body           string `gorm:"size:512" json:"body"`
	ConversationID uint   `json:"conversation_id"`
	Read           bool   `gorm:"default:false;index" json:"read"`
}
