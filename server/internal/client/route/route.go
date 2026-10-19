package route

// Package route 用户端（client，访客）的路由注册。
//
// 只做注册：路径、方法、中间件与拆分前逐字一致，处理器取 client/handler 的方法值。
// 访客端点全部免登录（参数走 GET query），因此只用到 /api 分组本身；
// 访客 WebSocket /ws/guest 挂在引擎根（不属于 /api），由 RegisterWS 单独注册。

import (
	"kefu-server/internal/client/handler"

	"github.com/gin-gonic/gin"
)

// Register 注册用户端在 /api 下的路由（无需登录）。
//   - g：/api 分组（无鉴权）；
//   - authed：挂在 /api 下、已套 middleware.JWTAuth() 的子分组（用户端目前没有登录态端点）。
func Register(g *gin.RouterGroup, authed *gin.RouterGroup) {
	h := handler.NewHandler()

	g.GET("/guest/init", h.GuestInitHandler)
	g.POST("/guest/message", h.GuestSendHandler)
	g.GET("/guest/messages", h.GuestMessagesHandler)
}

// RegisterWS 注册访客实时通道 /ws/guest（挂在引擎根，凭 channel + uid，不属于 /api）。
func RegisterWS(r *gin.Engine) {
	h := handler.NewHandler()
	r.GET("/ws/guest", h.GuestWSHandler)
}
