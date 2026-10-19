package route

// Package route 客服端（agent，工作台 / APP / 桌面端）的路由注册。
//
// 只做注册：路径、方法、中间件与拆分前逐字一致，处理器取 agent/handler 的方法值。
// WebSocket 通道 /ws 挂在引擎根（不属于 /api），由 RegisterWS 单独注册。

import (
	"kefu-server/internal/agent/handler"

	"github.com/gin-gonic/gin"
)

// Register 注册客服端在 /api 下的路由。
//   - g：/api 分组（无鉴权）；
//   - authed：挂在 /api 下、已套 middleware.JWTAuth() 的子分组。
func Register(g *gin.RouterGroup, authed *gin.RouterGroup) {
	h := handler.NewHandler()

	// 会话
	authed.GET("/conversations", h.ListConversationsHandler)
	authed.GET("/conversations/stats", h.ConversationStatsHandler)
	authed.GET("/conversations/:id", h.GetConversationHandler)
	authed.POST("/conversations/:id/claim", h.ClaimConversationHandler)
	authed.POST("/conversations/:id/transfer", h.TransferConversationHandler)
	authed.POST("/conversations/:id/close", h.CloseConversationHandler)
	authed.POST("/conversations/:id/read", h.MarkReadHandler)
	// 会话归档 / 删除 / 消息搜索
	authed.POST("/conversations/archive-bulk", h.ArchiveConversationsBulkHandler)
	authed.POST("/conversations/:id/archive", h.ArchiveConversationHandler)
	authed.POST("/conversations/:id/unarchive", h.UnarchiveConversationHandler)
	authed.DELETE("/conversations/:id", h.DeleteConversationHandler)
	authed.GET("/conversations/:id/messages/search", h.SearchConversationMessagesHandler)
	authed.GET("/conversations/:id/messages", h.ListMessagesHandler)
	authed.POST("/conversations/:id/messages", h.SendMessageHandler)

	// 客户资料与标签
	authed.GET("/users/:id", h.GetCustomerHandler)
	authed.PUT("/users/:id/tags", h.SetCustomerTagsHandler)

	// 设备（APP / PC 桌面端推送注册）
	authed.GET("/devices", h.ListDevicesHandler)
	authed.POST("/devices", h.RegisterDeviceHandler)
	authed.DELETE("/devices/:id", h.DeleteDeviceHandler)

	// 离线消息同步
	authed.GET("/offline/messages", h.OfflineMessagesHandler)
	authed.GET("/offline/notifications", h.OfflineNotificationsHandler)
	authed.POST("/offline/notifications/read", h.ReadNotificationsHandler)

	// 演示
	authed.POST("/demo/incoming", h.DemoIncomingHandler)
	authed.GET("/demo/products", h.DemoProductsHandler)
}

// RegisterWS 注册客服端实时通道 /ws（挂在引擎根，需 token，不属于 /api）。
func RegisterWS(r *gin.Engine) {
	h := handler.NewHandler()
	r.GET("/ws", h.WSHandler)
}
