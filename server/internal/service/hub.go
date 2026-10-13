package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"kefu-server/internal/cache"

	"github.com/gorilla/websocket"
)

// Event 下发给客户端的事件
type Event struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Client 一个 WebSocket 连接（Web 工作台 / PC 桌面端 / APP）
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	UserID   uint
	TenantID uint
	Device   string // web | desktop | app
	mu       sync.Mutex
}

// Hub 连接中心。分布式部署时通过 Redis Pub/Sub 把事件广播到所有实例。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool

	broadcast chan broadcastMsg
}

type broadcastMsg struct {
	TenantID uint
	UserIDs  []uint // 为空表示广播给该租户所有在线连接
	Event    Event
}

var hubOnce sync.Once
var defaultHub *Hub

func HubInstance() *Hub {
	hubOnce.Do(func() {
		h := &Hub{
			clients:   map[*Client]bool{},
			broadcast: make(chan broadcastMsg, 1024),
		}
		defaultHub = h
		go h.run()
		h.subscribeRedis()
	})
	return defaultHub
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
		h.localDispatch(env.TenantID, nil, ev)
	})
}

func (h *Hub) run() {
	for msg := range h.broadcast {
		h.localDispatch(msg.TenantID, msg.UserIDs, msg.Event)
	}
}

// localDispatch 只推给本实例的连接；同时把事件转发到 Redis 让其他实例也推。
func (h *Hub) localDispatch(tenantID uint, userIDs []uint, ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	target := map[uint]bool{}
	for _, u := range userIDs {
		target[u] = true
	}
	h.mu.RLock()
	for c := range h.clients {
		if tenantID != 0 && c.TenantID != tenantID {
			continue
		}
		if len(target) > 0 && !target[c.UserID] {
			continue
		}
		select {
		case c.send <- b:
		default:
			// 客户端处理慢，丢弃最旧的一条避免阻塞
			select {
			case <-c.send:
			default:
			}
			select {
			case c.send <- b:
			default:
			}
		}
	}
	h.mu.RUnlock()
}

// Publish 对外入口：本地推送 + Redis 广播到其他实例。
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

// Add 注册连接
func (h *Hub) Add(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

// Remove 注销连接
func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// ---- 连接读写循环 ----

// NewClient 创建一个连接（由 api 层调用，避免跨包访问未导出字段）
func NewClient(conn *websocket.Conn, userID, tenantID uint, device string) *Client {
	return &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		UserID:   userID,
		TenantID: tenantID,
		Device:   device,
		hub:      HubInstance(),
	}
}

// SetHub 绑定所属 Hub（Client 由 api 层创建，避免包循环依赖）
func (c *Client) SetHub(h *Hub) { c.hub = h }

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
