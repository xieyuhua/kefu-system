package handler

import (
	"net/http"

	"kefu-server/pkg/auth"
	"kefu-server/pkg/realtime"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 客服端 WebSocket ----------------

// WSHandler 客服端实时通道：Web 工作台 / PC 桌面端 / APP 共用。
// 连接时通过 ?token= 鉴权，?device=web|desktop|app 标识端类型。
func (h *Handler) WSHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Sec-WebSocket-Protocol")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	claims, err := auth.Default().ParseToken(token)
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

	conn, err := commonsvc.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := realtime.NewClient(conn, claims.UserID, claims.TenantID, device)
	realtime.HubInstance().Add(client)

	go client.WritePump()
	go client.ReadPump()
}
