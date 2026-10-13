package model

import (
	"time"
)

// 时间格式化统一使用 "2006-01-02 15:04:05"，前端直接展示。
type Model struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tenant 租户（SAAS 的一个商家）
type Tenant struct {
	Model
	Name   string `gorm:"size:64;not null" json:"name"`
	Logo   string `gorm:"size:255" json:"logo"`
	Status int    `gorm:"default:1" json:"status"` // 1 正常 0 停用
}

// User 后台账号：租户管理员 admin 或客服子账号 agent
type User struct {
	Model
	TenantID      uint   `gorm:"index;not null" json:"tenant_id"`
	Account       string `gorm:"size:64;uniqueIndex;not null" json:"account"`
	PasswordHash  string `gorm:"size:255;not null" json:"-"`
	Name          string `gorm:"size:64" json:"name"`
	Avatar        string `gorm:"size:255" json:"avatar"`
	Role          string `gorm:"size:16;default:'agent'" json:"role"` // admin | agent
	Online        bool   `gorm:"default:false" json:"online"`
	Enabled       bool   `gorm:"default:true" json:"enabled"` // 停用后不可登录、不可接待
	MaxConcurrent int    `gorm:"default:5" json:"max_concurrent"`
}

// Channel 渠道（微信/企微/网页/APP/微博/抖音/机器人）
type Channel struct {
	Model
	TenantID     uint   `gorm:"index;not null" json:"tenant_id"`
	Type         string `gorm:"size:32;not null" json:"type"`
	Name         string `gorm:"size:64;not null" json:"name"`
	AppID        string `gorm:"size:128" json:"app_id"`
	Enabled      bool   `gorm:"default:true" json:"enabled"`
	RobotEnabled bool   `gorm:"default:false" json:"robot_enabled"`
}

// AgentChannel 子账号可接待的渠道（多对多）
type AgentChannel struct {
	UserID    uint `gorm:"primaryKey;index" json:"user_id"`
	ChannelID uint `gorm:"primaryKey;index" json:"channel_id"`
}

// Customer 客户（咨询者）
type Customer struct {
	Model
	TenantID   uint       `gorm:"index;not null" json:"tenant_id"`
	ChannelID  uint       `gorm:"index" json:"channel_id"`
	ExternalID string     `gorm:"size:128;index" json:"external_id"` // 渠道侧 openid
	Nickname   string     `gorm:"size:64" json:"nickname"`
	Avatar     string     `gorm:"size:255" json:"avatar"`
	Phone      string     `gorm:"size:32" json:"phone"`
	Remark     string     `gorm:"size:255" json:"remark"` // 客服备注
	LastSeenAt *time.Time `json:"last_seen_at"`
}

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
}

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

// Order 历史订单
type Order struct {
	Model
	TenantID   uint       `gorm:"index;not null" json:"tenant_id"`
	CustomerID uint       `gorm:"index" json:"customer_id"`
	OrderNo    string     `gorm:"size:64;index" json:"order_no"`
	Title      string     `gorm:"size:255" json:"title"`
	Status     string     `gorm:"size:32" json:"status"`
	Amount     float64    `json:"amount"`
	Quantity   int        `json:"quantity"`
	Image      string     `gorm:"size:255" json:"image"`
	SKU        string     `gorm:"size:128" json:"sku"`
	PaidAt     *time.Time `json:"paid_at"`
}

// Product 商品（客服可发送商品卡片）
type Product struct {
	Model
	TenantID uint    `gorm:"index;not null" json:"tenant_id"`
	PID      string  `gorm:"size:64;index" json:"id"` // 对外商品 ID，如 P10086
	Title    string  `gorm:"size:255" json:"title"`
	Price    float64 `json:"price"`
	Image    string  `gorm:"size:255" json:"image"`
	Stock    int     `json:"stock"`
	SKU      string  `gorm:"size:128" json:"sku"`
	URL      string  `gorm:"size:255" json:"url"`
}

// Device 推送设备（APP / PC 桌面端预留）
type Device struct {
	Model
	TenantID  uint   `gorm:"index" json:"tenant_id"`
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	Device    string `gorm:"size:16" json:"device"`   // app | desktop | web
	Platform  string `gorm:"size:16" json:"platform"` // ios | android | windows | mac
	PushToken string `gorm:"size:255" json:"push_token"`
	Name      string `gorm:"size:64" json:"name"`
}

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
