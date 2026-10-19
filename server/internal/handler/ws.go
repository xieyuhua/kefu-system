package handler

import (
	"net/http"

	"kefu-server/internal/auth"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   4096,
	CheckOrigin:       func(r *http.Request) bool { return true },
	EnableCompression: true,
}

// WSHandler 客服端实时通道：Web 工作台 / PC 桌面端 / APP 共用。
// 连接时通过 ?token= 鉴权，?device=web|desktop|app 标识端类型。
func WSHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Sec-WebSocket-Protocol")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	claims, err := auth.ParseToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "登录已失效"})
		return
	}

	device := c.Query("device")
	switch device {
	case "desktop", "app", "guest":
	default:
		device = "web"
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := service.NewClient(conn, claims.UserID, claims.TenantID, device)
	service.HubInstance().Add(client)

	go client.WritePump()
	go client.ReadPump()
}

// GuestWSHandler 用户端（访客）实时通道，无需登录，凭 channel + uid 定位会话。
// 连接后立即回放「离线期间累积的消息」（since_id 之后），保证不在线也不丢消息。
//
//	GET /ws/guest?channel=web&uid=u_10001&since_id=0&tenant=1
func GuestWSHandler(c *gin.Context) {
	tenantID := guestTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少租户参数 tenant"})
		return
	}
	ch, err := resolveChannel(c, tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "渠道不存在"})
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少参数 uid"})
		return
	}

	// 定位客户与会话（复用 GuestInit 的查找逻辑）
	cu, convID, err := service.FindGuestConversation(tenantID, ch.ID, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := service.NewGuestClient(conn, tenantID, convID, "guest")
	hub := service.HubInstance()
	hub.AddGuest(client)

	// ① 连接确认
	client.SendEvent("connected", map[string]interface{}{
		"conversation_id": convID,
		"customer_id":     cu.ID,
		"nickname":        cu.Nickname,
		"avatar":          cu.Avatar,
		"channel":         map[string]interface{}{"id": ch.ID, "type": ch.Type, "name": ch.Name},
	})

	// ② 离线消息回放：把 since_id 之后的消息一次性推给该端
	if msgs, err := service.ListMessagesSince(tenantID, convID, queryUint(c, "since_id"), 200); err == nil && len(msgs) > 0 {
		for _, m := range msgs {
			client.SendEvent("message.new", m)
		}
	}

	go client.WritePump()
	go client.ReadPump()
}
