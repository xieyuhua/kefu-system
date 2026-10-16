package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"kefu-server/internal/cache"
	"kefu-server/internal/model"

	"github.com/gorilla/websocket"
)

// Event 下发给客户端的事件
type Event struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Client 一个 WebSocket 连接
//   - 客服端：Web 工作台 / PC 桌面端 / APP，UserID > 0
//   - 访客端：用户端聊天页，UserID == 0，ConvID 指向其会话
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	UserID   uint
	TenantID uint
	ConvID   uint   // 访客端所属会话
	Device   string // web | desktop | app
	closed   chan struct{}
	mu       sync.Mutex
}

// Hub 连接中心。分两类连接：
//   - 客服端 clients：按租户/用户广播
//   - 访客端 guests：按会话（conversation_id）定向投递
//
// 分布式部署时通过 Redis Pub/Sub 把事件广播到所有实例。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	guests  map[uint]map[*Client]bool // convID -> 连接集合

	broadcast chan broadcastMsg
}

type broadcastMsg struct {
	TenantID uint
	UserIDs  []uint // 客服端定向；为空表示广播给该租户所有客服连接
	ConvID   uint   // 访客端定向；>0 时只投递给该会话的访客连接
	Event    Event
}

var hubOnce sync.Once
var defaultHub *Hub

func HubInstance() *Hub {
	hubOnce.Do(func() {
		h := &Hub{
			clients:   map[*Client]bool{},
			guests:    map[uint]map[*Client]bool{},
			broadcast: make(chan broadcastMsg, 1024),
		}
		defaultHub = h
		go h.run()
		h.subscribeRedis()
		h.subscribeGuestRedis()
	})
	return defaultHub
}

// subscribeGuestRedis 订阅跨实例的用户端（访客）事件
func (h *Hub) subscribeGuestRedis() {
	cache.Get().Subscribe(context.Background(), "guest-events", func(env cache.Envelope) {
		if env.SenderID == cache.InstanceID {
			return
		}
		var ev Event
		b, err := json.Marshal(env.Data)
		if err != nil {
			return
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			ev = Event{Event: env.Event}
			_ = json.Unmarshal(b, &ev.Data)
		}
		if ev.Event == "" {
			ev.Event = env.Event
		}
		h.localDispatch(env.TenantID, nil, env.ConvID, ev)
	})
}

func (h *Hub) subscribeRedis() {
	// 订阅跨实例广播；内存驱动下为空操作（单机直连推送）
	cache.Get().Subscribe(context.Background(), "events", func(env cache.Envelope) {
		if env.SenderID == cache.InstanceID {
			return // 忽略自己发出的
		}
		var ev Event
		b, err := json.Marshal(env.Data)
		if err != nil {
			return
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			// 兼容直接放 Event 的情况
			ev = Event{Event: env.Event}
			_ = json.Unmarshal(b, &ev.Data)
		}
		if ev.Event == "" {
			ev.Event = env.Event
		}
		h.localDispatch(env.TenantID, nil, 0, ev)
	})
}

func (h *Hub) run() {
	for msg := range h.broadcast {
		h.localDispatch(msg.TenantID, msg.UserIDs, msg.ConvID, msg.Event)
	}
}

// localDispatch 只推给本实例的连接；同时把事件转发到 Redis 让其他实例也推。
// convID > 0 时投递给该会话的访客连接；否则按租户/用户投递客服端连接。
func (h *Hub) localDispatch(tenantID uint, userIDs []uint, convID uint, ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	target := map[uint]bool{}
	for _, u := range userIDs {
		target[u] = true
	}
	h.mu.RLock()
	// 客服端连接
	for c := range h.clients {
		if tenantID != 0 && c.TenantID != tenantID {
			continue
		}
		if len(target) > 0 && !target[c.UserID] {
			continue
		}
		push(c, b)
	}
	// 访客端连接（按会话定向）
	if convID > 0 {
		for c := range h.guests[convID] {
			push(c, b)
		}
	}
	h.mu.RUnlock()
}

// push 投递一条消息；发送队列满时丢弃最旧的一条，保证慢客户端不阻塞全局。
func push(c *Client, b []byte) {
	select {
	case c.send <- b:
		return
	default:
	}
	select {
	case <-c.send:
	default:
	}
	select {
	case c.send <- b:
	default:
	}
}

// Publish 对外入口：本地推送 + Redis 广播到其他实例。
// convID > 0 表示投递给用户端（访客），否则投递给客服端。
func (h *Hub) Publish(tenantID uint, userIDs []uint, ev Event) {
	h.broadcast <- broadcastMsg{TenantID: tenantID, UserIDs: userIDs, Event: ev}

	if cache.Get().Enabled() {
		env := cache.Envelope{
			TenantID:  tenantID,
			Event:     ev.Event,
			Data:      ev,
			SenderID:  cache.InstanceID,
			Timestamp: time.Now().UnixMilli(),
		}
		if err := cache.Get().Publish(context.Background(), "events", env); err != nil {
			log.Printf("[hub] publish error: %v", err)
		}
	}
}

// Add 注册客服端连接
func (h *Hub) Add(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	// 标记客服在线（供列表展示「在线/离线」小圆点）
	if c.UserID > 0 {
		gormDB().Model(&model.User{}).Where("id = ?", c.UserID).Update("online", true)
	}
}

// AddGuest 注册访客（用户端）连接，按会话归类
func (h *Hub) AddGuest(c *Client) {
	h.mu.Lock()
	if h.guests[c.ConvID] == nil {
		h.guests[c.ConvID] = map[*Client]bool{}
	}
	h.guests[c.ConvID][c] = true
	h.mu.Unlock()
}

// Remove 注销连接（客服端或访客端通用）
func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	if c.ConvID > 0 {
		if set, ok := h.guests[c.ConvID]; ok {
			delete(set, c)
			if len(set) == 0 {
				delete(h.guests, c.ConvID)
			}
		}
	}
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
	}
	// 只关闭一次
	select {
	case <-c.closed:
	default:
		close(c.closed)
		close(c.send)
	}
	h.mu.Unlock()

	// 客服端断开：若该客服没有其它在线连接，则标记离线并释放其正在接待的会话，
	// 让同渠道的其他客服可以接手（管理员离线同样释放）。
	if c.UserID > 0 && !h.IsUserOnline(c.UserID) {
		gormDB().Model(&model.User{}).Where("id = ?", c.UserID).Update("online", false)
		releaseAgentConversations(c.TenantID, c.UserID)
	}
}

// IsUserOnline 判断某客服是否还有活跃的 WebSocket 连接
func (h *Hub) IsUserOnline(userID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.UserID == userID {
			return true
		}
	}
	return false
}

// releaseAgentConversations 把某客服正在接待的会话全部置回「待接待」。
// 用于客服（含管理员）离线后自动释放，避免会话被已离线的人长期占用。
func releaseAgentConversations(tenantID, agentID uint) {
	var convs []model.Conversation
	if err := gormDB().
		Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, "open").
		Find(&convs).Error; err != nil || len(convs) == 0 {
		return
	}
	now := time.Now()
	for _, c := range convs {
		c.AgentID = 0
		c.Status = "pending"
		c.LastMessageAt = now
		_ = gormDB().Save(&c).Error
		addSystemMessage(tenantID, c.ID, agentName(agentID)+" 已离线，会话回到待接待")
		vo := buildConversationVO(c)
		HubInstance().Publish(tenantID, nil, Event{Event: "conversation.updated", Data: vo})
		if m := lastSystemMessage(tenantID, c.ID); m != nil {
			HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
		}
	}
	log.Printf("[hub] 客服 #%d 离线，释放 %d 个会话回待接待", agentID, len(convs))
}

// GuestCount 返回某会话当前在线的访客连接数（用于判断是否在线）
func (h *Hub) GuestCount(convID uint) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.guests[convID])
}

// PublishToGuest 投递给指定会话的用户端连接（可跨实例）。
// 访客不在线时消息仍会入库，用户端重连后按 since_id 补拉，因此「不在线也不丢」。
func (h *Hub) PublishToGuest(tenantID, convID uint, ev Event) {
	h.broadcast <- broadcastMsg{TenantID: tenantID, ConvID: convID, Event: ev}

	if cache.Get().Enabled() {
		env := cache.Envelope{
			TenantID:  tenantID,
			ConvID:    convID,
			Event:     ev.Event,
			Data:      ev,
			SenderID:  cache.InstanceID,
			Timestamp: time.Now().UnixMilli(),
		}
		if err := cache.Get().Publish(context.Background(), "guest-events", env); err != nil {
			log.Printf("[hub] guest publish error: %v", err)
		}
	}
}

// ---- 连接读写循环 ----

// NewClient 创建一个客服端连接（由 api 层调用，避免跨包访问未导出字段）
func NewClient(conn *websocket.Conn, userID, tenantID uint, device string) *Client {
	return &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		closed:   make(chan struct{}),
		UserID:   userID,
		TenantID: tenantID,
		Device:   device,
		hub:      HubInstance(),
	}
}

// NewGuestClient 创建一个访客（用户端）连接
func NewGuestClient(conn *websocket.Conn, tenantID, convID uint, device string) *Client {
	return &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		closed:   make(chan struct{}),
		TenantID: tenantID,
		ConvID:   convID,
		Device:   device,
		hub:      HubInstance(),
	}
}

// SetHub 绑定所属 Hub（Client 由 api 层创建，避免包循环依赖）
func (c *Client) SetHub(h *Hub) { c.hub = h }

// SendEvent 向该连接推送一条事件（用于连接确认、离线消息回放）
func (c *Client) SendEvent(name string, data interface{}) {
	b, err := json.Marshal(Event{Event: name, Data: data})
	if err != nil {
		return
	}
	select {
	case c.send <- b:
	default:
	}
}

// WritePump 下发消息循环（含心跳）
func (c *Client) WritePump() { c.writePump() }

// ReadPump 读取循环（仅维持心跳，业务数据走 HTTP）
func (c *Client) ReadPump() { c.readPump() }

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.TextMessage, msg)
			c.mu.Unlock()
			if err != nil {
				return
			}
		case <-ticker.C:
			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Remove(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
