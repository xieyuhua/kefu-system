package api

import (
	"net/http"

	"kefu-server/internal/auth"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WSHandler 实时通道：Web 工作台 / PC 桌面端 / APP 共用。
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
	case "desktop", "app":
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
