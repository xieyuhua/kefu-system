package model

// Message 消息；type: text|image|order|product|system
// direction: in（客户→客服）| out（客服→客户）| system
type Message struct {
	Model
	TenantID       uint   `gorm:"index;not null" json:"tenant_id"`
	ConversationID uint   `gorm:"index;not null" json:"conversation_id"`
	SenderID       uint   `json:"sender_id"`        // 0=系统；否则 user_id 或 customer_id
	SenderRole     string `gorm:"size:16" json:"-"` // user | customer | system
	Type           string `gorm:"size:16;default:'text'" json:"type"`
	Direction      string `gorm:"size:8" json:"direction"`
	Content        string `gorm:"type:text" json:"content"`
	Read           bool   `gorm:"default:false" json:"read"`
}
