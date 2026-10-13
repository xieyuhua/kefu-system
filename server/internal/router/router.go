package router

import (
	"net/http"
	"os"
	"path/filepath"

	"kefu-server/internal/api"
	"kefu-server/internal/config"
	"kefu-server/internal/middleware"

	"github.com/gin-gonic/gin"
)

// spaIndex 返回前端首页（SPA 回退）。目录不存在时给出明确提示而不是 500。
func spaIndex(webDir string) gin.HandlerFunc {
	index := filepath.Join(webDir, "index.html")
	return func(c *gin.Context) {
		if _, err := os.Stat(index); err != nil {
			c.String(http.StatusNotFound,
				"前端产物未找到：请把 web/dist 复制到 server/web 后再访问，或用 `pnpm dev` 启动前端开发服务。")
			return
		}
		c.File(index)
	}
}

func Setup() *gin.Engine {
	gin.SetMode(config.C.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())

	// 静态资源
	r.Static("/uploads", config.C.Upload.Dir)
	r.Static("/demo", filepath.Join("data", "demo"))

	// 前端三个独立入口：/ （用户端）、/chat/ （客服工作台）、/admin/ （管理后台）
	// 前端构建产物放在 server/web 目录下时由后端直接托管，且每个前缀都做 SPA 回退。
	webDir := "web"
	r.StaticFile("/", filepath.Join(webDir, "index.html"))
	r.Static("/assets", filepath.Join(webDir, "assets"))
	r.StaticFile("/favicon.svg", filepath.Join(webDir, "favicon.svg"))
	r.StaticFile("/icons.svg", filepath.Join(webDir, "icons.svg"))

	// 带前缀的入口：把 /chat/xxx、/admin/xxx 回退到各自的 index.html。
	// gin 不允许同时注册 "/chat" 与 "/chat/*any"，这里统一用 *any 带可选的尾部斜杠处理。
	for _, prefix := range []string{"chat", "admin"} {
		p := "/" + prefix
		r.GET(p, spaIndex(webDir))
		r.GET(p+"/*any", spaIndex(webDir))
	}
	// 其它未知路径也回退到首页（用户端 /c 等走 hash 路由）
	r.NoRoute(func(c *gin.Context) {
		if idx := len("/api"); len(c.Request.URL.Path) >= idx && c.Request.URL.Path[:idx] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "接口不存在"})
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})

	// WebSocket（token 走 query，APP/PC 桌面端同样适用）
	r.GET("/ws", api.WSHandler)

	g := r.Group("/api")
	{
		// 认证
		g.POST("/auth/login", api.Login)
		g.POST("/auth/logout", middleware.JWTAuth(), api.Logout)
		g.GET("/auth/me", middleware.JWTAuth(), api.Me)

		// 用户端（GET 参数传递渠道等信息，无需登录）
		g.GET("/guest/init", api.GuestInit)
		g.POST("/guest/message", api.GuestSend)
		g.GET("/guest/messages", api.GuestMessages)

		authed := g.Group("", middleware.JWTAuth())
		{
			// 渠道
			authed.GET("/channels", api.ListChannels)
			authed.POST("/channels", api.CreateChannel)
			authed.PUT("/channels/:id", api.UpdateChannel)
			authed.DELETE("/channels/:id", api.DeleteChannel)

			// 渠道二维码 / 接入地址
			authed.GET("/channels/:id/qr", api.ChannelQRInfo)
			authed.GET("/channels/:id/qr.png", api.ChannelQRPNG)
			// 渠道接待账号快速设置
			authed.GET("/channels/:id/agents", api.ListChannelAgents)
			authed.PUT("/channels/:id/agents", api.SetChannelAgents)

			// 渠道类型库（自定义类型，不限制枚举）
			authed.GET("/channel-types", api.ListChannelTypes)
			authed.POST("/channel-types", api.CreateChannelType)
			authed.PUT("/channel-types/:id", api.UpdateChannelType)
			authed.DELETE("/channel-types/:id", api.DeleteChannelType)

			// 子账号
			authed.GET("/agents", api.ListAgents)
			authed.POST("/agents", api.CreateAgent)
			authed.PUT("/agents/:id", api.UpdateAgent)
			authed.DELETE("/agents/:id", api.DeleteAgent)
			authed.POST("/agents/:id/online", api.SetOnline)

			// 会话
			authed.GET("/conversations", api.ListConversations)
			authed.GET("/conversations/stats", api.ConversationStats)
			authed.GET("/conversations/:id", api.GetConversation)
			authed.POST("/conversations/:id/claim", api.ClaimConversation)
			authed.POST("/conversations/:id/transfer", api.TransferConversation)
			authed.POST("/conversations/:id/close", api.CloseConversation)
			authed.POST("/conversations/:id/read", api.MarkRead)
			authed.GET("/conversations/:id/messages", api.ListMessages)
			authed.POST("/conversations/:id/messages", api.SendMessage)

			// 客户与历史档案
			authed.GET("/users", api.ListCustomers)
			authed.GET("/users/:id", api.GetCustomer)
			authed.PUT("/users/:id/remark", api.UpdateCustomerRemark)

			// 客户标签
			authed.GET("/tags", api.ListTags)
			authed.POST("/tags", api.CreateTag)
			authed.DELETE("/tags/:id", api.DeleteTag)
			authed.PUT("/users/:id/tags", api.SetCustomerTags)

			// 订单 / 商品
			authed.GET("/orders", api.ListOrders)
			authed.GET("/products", api.ListProducts)

			// 自动回复规则（关键词 → 机器人回复 / 转人工）
			authed.GET("/auto-replies", api.ListAutoReplies)
			authed.POST("/auto-replies", api.CreateAutoReply)
			authed.PUT("/auto-replies/:id", api.UpdateAutoReply)
			authed.DELETE("/auto-replies/:id", api.DeleteAutoReply)

			// 数据统计
			authed.GET("/stats/overview", api.StatsOverview)

			// 设备（APP / PC 桌面端推送注册）
			authed.GET("/devices", api.ListDevices)
			authed.POST("/devices", api.RegisterDevice)
			authed.DELETE("/devices/:id", api.DeleteDevice)

			// 离线消息同步
			authed.GET("/offline/messages", api.OfflineMessages)
			authed.GET("/offline/notifications", api.OfflineNotifications)
			authed.POST("/offline/notifications/read", api.ReadNotifications)

			// 演示
			authed.POST("/demo/incoming", api.DemoIncoming)
			authed.GET("/demo/products", api.DemoProducts)
		}
	}
	return r
}
