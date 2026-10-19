package route

// Package route 后台端（admin）的 /api 路由注册。
//
// 只做注册：路径、方法、中间件与拆分前逐字一致，处理器取 admin/handler 的方法值。

import (
	"kefu-server/internal/admin/handler"

	"github.com/gin-gonic/gin"
)

// Register 注册后台端在 /api 下的路由。
//   - g：/api 分组（无鉴权）；
//   - authed：挂在 /api 下、已套 middleware.JWTAuth() 的子分组。
func Register(g *gin.RouterGroup, authed *gin.RouterGroup) {
	h := handler.NewHandler()

	// 渠道
	authed.GET("/channels", h.ListChannelsHandler)
	authed.POST("/channels", h.CreateChannelHandler)
	authed.PUT("/channels/:id", h.UpdateChannelHandler)
	authed.DELETE("/channels/:id", h.DeleteChannelHandler)
	// 重置渠道标识（app_id）：生成新的随机标识，旧链接失效
	authed.POST("/channels/:id/reset-app-id", h.ResetChannelAppIDHandler)

	// 渠道二维码 / 接入地址
	authed.GET("/channels/:id/qr", h.ChannelQRInfoHandler)
	authed.GET("/channels/:id/qr.png", h.ChannelQRPNGHandler)
	// 渠道接待账号快速设置
	authed.GET("/channels/:id/agents", h.ListChannelAgentsHandler)
	authed.PUT("/channels/:id/agents", h.SetChannelAgentsHandler)

	// 渠道类型库（自定义类型，不限制枚举）
	authed.GET("/channel-types", h.ListChannelTypesHandler)
	authed.POST("/channel-types", h.CreateChannelTypeHandler)
	authed.PUT("/channel-types/:id", h.UpdateChannelTypeHandler)
	authed.DELETE("/channel-types/:id", h.DeleteChannelTypeHandler)

	// 子账号
	authed.GET("/agents", h.ListAgentsHandler)
	authed.POST("/agents", h.CreateAgentHandler)
	authed.PUT("/agents/:id", h.UpdateAgentHandler)
	authed.DELETE("/agents/:id", h.DeleteAgentHandler)
	authed.POST("/agents/:id/online", h.SetOnlineHandler)

	// 客户与历史档案
	authed.GET("/conversation-records", h.SearchConversationRecordsHandler)
	authed.GET("/conversation-records/:id/messages", h.GetConversationMessagesHandler)
	authed.GET("/users", h.ListCustomersHandler)
	authed.PUT("/users/:id/remark", h.UpdateCustomerRemarkHandler)

	// 客户标签
	authed.GET("/tags", h.ListTagsHandler)
	authed.POST("/tags", h.CreateTagHandler)
	authed.DELETE("/tags/:id", h.DeleteTagHandler)

	// 自动回复规则（关键词 → 机器人回复 / 转人工）
	authed.GET("/auto-replies", h.ListAutoRepliesHandler)
	authed.POST("/auto-replies", h.CreateAutoReplyHandler)
	authed.PUT("/auto-replies/:id", h.UpdateAutoReplyHandler)
	authed.DELETE("/auto-replies/:id", h.DeleteAutoReplyHandler)

	// 数据统计
	authed.GET("/stats/overview", h.StatsOverviewHandler)

	// 系统配置（默认图片等）
	authed.GET("/settings", h.ListSettingsHandler)
	authed.PUT("/settings", h.SaveSettingHandler)

	// 设备推送自检（APP / PC 桌面端）
	authed.POST("/devices/test-push", h.TestDevicePushHandler)
}
