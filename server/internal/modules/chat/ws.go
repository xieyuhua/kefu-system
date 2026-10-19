package chat

// 客服端 WebSocket 实时通道（原 handler/ws.go 的 WSHandler）。
// 用户端（访客）的同名通道在 guest 模块，复用本文件的 Upgrader。

import (
	"net/http"

	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/realtime"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Upgrader WebSocket 升级器。客服端与用户端（guest 模块）共用同一套参数，
// 因此导出给其它模块使用，避免两处配置漂移。
var Upgrader = websocket.Upgrader{
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

	conn, err := Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := realtime.NewClient(conn, claims.UserID, claims.TenantID, device)
	realtime.HubInstance().Add(client)

	go client.WritePump()
	go client.ReadPump()
}
