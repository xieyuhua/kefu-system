package dto

import "kefu-server/internal/common/model"

// LoginRequest / LoginResponse
type LoginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type TenantBrief struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Logo string `json:"logo"`
	/** 租户配置的默认图片（客户/商品/订单无图时兜底） */
	DefaultImage string `json:"default_image"`
}

type UserBrief struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	Online   bool   `json:"online"`
	TenantID uint   `json:"tenant_id"`
}

type LoginResponse struct {
	Token string      `json:"token"`
	User  CurrentUser `json:"user"`
}

type CurrentUser struct {
	ID            uint        `json:"id"`
	TenantID      uint        `json:"tenant_id"`
	Role          string      `json:"role"`
	Name          string      `json:"name"`
	Account       string      `json:"account"`
	Avatar        string      `json:"avatar"`
	Online        bool        `json:"online"`
	MaxConcurrent int         `json:"max_concurrent"`
	Channels      []uint      `json:"channels"`
	Tenant        TenantBrief `json:"tenant"`
}

// Channel
type ChannelRequest struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	AppID        string `json:"app_id"`
	Enabled      *bool  `json:"enabled"`
	RobotEnabled *bool  `json:"robot_enabled"`
	// 用户端展示控制
	HideHeader   *bool `json:"hide_header"`
	HideDebug    *bool `json:"hide_debug"`
	HideMoreMenu *bool `json:"hide_more_menu"`
	// 卡片能力开关：nil 表示不修改；创建时 nil 视为 true（保持存量行为）
	SupportOrderCard   *bool `json:"support_order_card"`
	SupportProductCard *bool `json:"support_product_card"`
}

type ChannelVO struct {
	ID           uint   `json:"id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	AppID        string `json:"app_id"`
	Enabled      bool   `json:"enabled"`
	RobotEnabled bool   `json:"robot_enabled"`
	HideHeader   bool   `json:"hide_header"`
	HideDebug    bool   `json:"hide_debug"`
	HideMoreMenu bool   `json:"hide_more_menu"`
	// 卡片能力开关（后台可配；关闭后该渠道不能收发对应卡片）
	SupportOrderCard   bool   `json:"support_order_card"`
	SupportProductCard bool   `json:"support_product_card"`
	CreatedAt          string `json:"created_at"`
	AgentCount         int64  `json:"agent_count"`
	PendingCount       int64  `json:"pending_count"`
	AccessPath         string `json:"access_path"` // 用户端接入路径，如 /c?channel=web
	AccessURL          string `json:"access_url"`  // 完整接入地址（基于后端 public_base），可直接对外使用
}

// ChannelAgentVO 渠道的接待账号（用于渠道列表快速设置）
type ChannelAgentVO struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Account  string `json:"account"`
	Avatar   string `json:"avatar"`
	Online   bool   `json:"online"`
	Enabled  bool   `json:"enabled"`
	Selected bool   `json:"selected"` // 是否已分配该渠道
}

// Agent
type AgentRequest struct {
	Account       string `json:"account"`
	Password      string `json:"password"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	MaxConcurrent int    `json:"max_concurrent"`
	ChannelIDs    []uint `json:"channel_ids"`
	Enabled       *bool  `json:"enabled"` // 启用/禁用，不传表示不修改
}

type ChannelSimple struct {
	ID   uint   `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type AgentVO struct {
	ID            uint            `json:"id"`
	Account       string          `json:"account"`
	Name          string          `json:"name"`
	Role          string          `json:"role"`
	Avatar        string          `json:"avatar"`
	Online        bool            `json:"online"`
	MaxConcurrent int             `json:"max_concurrent"`
	Status        string          `json:"status"`
	Enabled       bool            `json:"enabled"`
	Channels      []ChannelSimple `json:"channels"`
	ActiveCount   int64           `json:"active_count"`
}

// Conversation
type ChannelRef struct {
	ID   uint   `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	// 卡片能力开关（后台渠道配置）：随会话一起下发，
	// 客服工作台切换会话时据此重算「发送订单 / 发送商品」入口是否可用。
	SupportOrderCard   bool `json:"support_order_card"`
	SupportProductCard bool `json:"support_product_card"`
}

type TagRef struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type CustomerRef struct {
	ID         uint     `json:"id"`
	ExternalID string   `json:"external_id"`
	Nickname   string   `json:"nickname"`
	Avatar     string   `json:"avatar"`
	Phone      string   `json:"phone"`
	OrderCount int64    `json:"order_count"`
	ChannelID  uint     `json:"channel_id"`
	Tags       []TagRef `json:"tags"`
	Remark     string   `json:"remark"`
	IP         string   `json:"ip"`
	City       string   `json:"city"`
	/** 客户是否在线（按最近活跃时间推断，2 分钟内活跃视为在线） */
	Online     bool   `json:"online"`
	LastSeenAt string `json:"last_seen_at"`
}

type ConversationVO struct {
	ID            uint         `json:"id"`
	ChannelID     uint         `json:"channel_id"`
	Channel       *ChannelRef  `json:"channel"`
	User          *CustomerRef `json:"user"`
	Status        string       `json:"status"`
	Agent         *UserBrief   `json:"agent"`
	Unread        int          `json:"unread"`
	LastMessage   string       `json:"last_message"`
	LastMessageAt string       `json:"last_message_at"`
	// LastMessageTS 最后一条消息时间的 Unix 毫秒时间戳（取 conversation.LastMessageAt），
	// 供前端做秒级「未回复时长」计算；字符串形式的 last_message_at 保持原样不变。
	LastMessageTS int64 `json:"last_message_ts"`
	// UnrepliedTS 最后一条「客户消息」的时间（Unix 毫秒）；仅当该会话最后一条消息来自客户
	// （即客服未回复）时 > 0，否则 0。前端据此在会话列表项上展示「未回复 N 秒」。
	UnrepliedTS  int64  `json:"unreplied_ts"`
	CreatedAt    string `json:"created_at"`
	ClosedAt     string `json:"closed_at"`
	OrderCount   int64  `json:"order_count"`
	MessageCount int    `json:"message_count"`
	Summary      string `json:"summary"`
	// Archived 是否已归档（客服清理后的会话，不出现在工作台列表）
	Archived   bool   `json:"archived"`
	ArchivedAt string `json:"archived_at"`
}

// Message
type SenderRef struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Role   string `json:"role"`
}

type MessageVO struct {
	ID             uint      `json:"id"`
	ConversationID uint      `json:"conversation_id"`
	Type           string    `json:"type"`
	Direction      string    `json:"direction"`
	Content        string    `json:"content"`
	Sender         SenderRef `json:"sender"`
	CreatedAt      string    `json:"created_at"`
	// CreatedTS 消息创建时间的 Unix 毫秒时间戳，与 createdAt 同源，
	// 供前端做秒级计时（created_at 只有秒级精度的字符串）。
	CreatedTS int64 `json:"created_ts"`
}

type SendMessageRequest struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Customer detail
type OrderVO struct {
	ID        uint    `json:"id"`
	OrderNo   string  `json:"order_no"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	Amount    float64 `json:"amount"`
	Quantity  int     `json:"quantity"`
	Image     string  `json:"image"`
	SKU       string  `json:"sku"`
	CreatedAt string  `json:"created_at"`
	PaidAt    string  `json:"paid_at"`
}

type ProductVO struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Price float64 `json:"price"`
	Image string  `json:"image"`
	Stock int     `json:"stock"`
	SKU   string  `json:"sku"`
	URL   string  `json:"url"`
}

type CustomerListItem struct {
	ID                uint        `json:"id"`
	ExternalID        string      `json:"external_id"`
	Nickname          string      `json:"nickname"`
	Avatar            string      `json:"avatar"`
	Phone             string      `json:"phone"`
	Remark            string      `json:"remark"`
	Tags              []TagRef    `json:"tags"`
	IP                string      `json:"ip"`
	City              string      `json:"city"`
	Channel           *ChannelRef `json:"channel"`
	OrderCount        int64       `json:"order_count"`
	ConversationCount int64       `json:"conversation_count"`
	LastSeenAt        string      `json:"last_seen_at"`
	CreatedAt         string      `json:"created_at"`
}

type HistoryConversationVO struct {
	ID            uint        `json:"id"`
	Channel       *ChannelRef `json:"channel"`
	Status        string      `json:"status"`
	Agent         *UserBrief  `json:"agent"`
	MessageCount  int         `json:"message_count"`
	Summary       string      `json:"summary"`
	CreatedAt     string      `json:"created_at"`
	ClosedAt      string      `json:"closed_at"`
	LastMessageAt string      `json:"last_message_at"`
}

type CustomerDetail struct {
	User          CustomerDetailUser      `json:"user"`
	Orders        []OrderVO               `json:"orders"`
	Conversations []HistoryConversationVO `json:"conversations"`
}

type CustomerDetailUser struct {
	ID         uint        `json:"id"`
	ExternalID string      `json:"external_id"`
	Nickname   string      `json:"nickname"`
	Avatar     string      `json:"avatar"`
	Phone      string      `json:"phone"`
	Remark     string      `json:"remark"`
	Channel    *ChannelRef `json:"channel"`
	Tags       []TagRef    `json:"tags"`
	IP         string      `json:"ip"`
	City       string      `json:"city"`
	OrderCount int64       `json:"order_count"`
	CreatedAt  string      `json:"created_at"`
}

// Stats
type StatsVO struct {
	Pending int64 `json:"pending"`
	Open    int64 `json:"open"`
	Closed  int64 `json:"closed"`
	Unread  int64 `json:"unread"`
}

// Device / Notification
type DeviceRequest struct {
	Device    string `json:"device"`
	Platform  string `json:"platform"`
	PushToken string `json:"push_token"`
	Name      string `json:"name"`
}

type DeviceVO struct {
	ID        uint   `json:"id"`
	Device    string `json:"device"`
	Platform  string `json:"platform"`
	Name      string `json:"name"`
	PushToken string `json:"push_token"`
	CreatedAt string `json:"created_at"`
}

type NotificationVO struct {
	ID             uint   `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ConversationID uint   `json:"conversation_id"`
	Read           bool   `json:"read"`
	CreatedAt      string `json:"created_at"`
}

type AutoReplyRequest struct {
	Name string `json:"name"`
	// ChannelIDs 多选渠道：空列表（或只含 0）= 全渠道，仅管理员可用；
	// 客服至少要选一个自己已授权的渠道（业务层校验）。
	ChannelIDs []uint `json:"channel_ids"`
	// ChannelID 兼容旧客户端：0 = 全渠道，>0 = 单个渠道（等价于只选这一个）。
	// 指针用于区分「没传」与「显式传 0」：更新接口没传表示不改渠道。
	ChannelID       *uint  `json:"channel_id"`
	MatchType       string `json:"match_type"` // contain | equal | prefix
	Keywords        string `json:"keywords"`   // 逗号分隔
	Reply           string `json:"reply"`
	ReplyType       string `json:"reply_type"` // text | product
	TransferToHuman *bool  `json:"transfer_to_human"`
	Enabled         *bool  `json:"enabled"`
	Priority        int    `json:"priority"`
}

// HasTransfer 判断是否显式传了 transfer_to_human
func (r AutoReplyRequest) HasTransfer() bool { return r.TransferToHuman != nil }

// Demo
type DemoIncomingRequest struct {
	ChannelID uint   `json:"channel_id"`
	Nickname  string `json:"nickname"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	// UID 业务侧用户唯一标识。填写后：若该渠道下已存在此用户，则复用其客户与会话，
	// 可对同一用户反复发消息（模拟持续咨询）；留空则每次随机新建一个客户。
	UID string `json:"uid"`
}

// 内部：把 model 转成带可展示字符串的 VO 时用
type TimeFmt = string

var _ = model.Model{}
