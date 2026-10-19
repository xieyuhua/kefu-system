package route

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"kefu-server/internal/core/account"
	"kefu-server/internal/core/customer"
	"kefu-server/internal/core/upload"
	"kefu-server/internal/modules/admin"
	"kefu-server/internal/modules/agent"
	"kefu-server/internal/modules/client"
	"kefu-server/internal/platform/config"
	"kefu-server/internal/platform/middleware"

	"github.com/gin-gonic/gin"
)

// spaEntry 返回指定入口 HTML（admin.html / chat.html / kefu.html / index.html）。
// 文件不存在时给出明确提示而不是 500，方便本地用 pnpm dev 调试。
func spaEntry(webDir, entry string) gin.HandlerFunc {
	file := filepath.Join(webDir, entry)
	return func(c *gin.Context) {
		if _, err := os.Stat(file); err != nil {
			c.String(http.StatusNotFound,
				"前端产物 %s 未找到：请把 web/dist 复制到 server/web 后再访问，或用 `pnpm dev` 启动前端开发服务。", entry)
			return
		}
		c.File(file)
	}
}

// ends 定义三个独立端：URL 前缀 → 入口 HTML
var ends = []struct {
	Prefix string
	Entry  string
}{
	{"admin", "admin.html"},
	{"chat", "chat.html"},
	{"kefu", "kefu.html"},
}

// WebDir 定位前端产物目录。优先用环境变量 KEFU_WEB_DIR，
// 其次取「可执行文件所在目录 /web」，最后回退到当前工作目录下的 web。
// 这样无论从哪个目录启动（go run . 或运行编译好的二进制）都能找到产物。
func WebDir() string {
	if v := os.Getenv("KEFU_WEB_DIR"); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "web")
		if _, statErr := os.Stat(dir); statErr == nil {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dir := filepath.Join(wd, "web")
		if _, statErr := os.Stat(dir); statErr == nil {
			return dir
		}
	}
	return "web"
}

// UploadDir 定位上传文件目录（用绝对路径，避免进程 CWD 不同导致图片 404）。
// 优先环境变量，其次可执行文件同级，最后回退到当前工作目录。
func UploadDir() string {
	if v := os.Getenv("KEFU_UPLOAD_DIR"); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "data", "uploads")
		if _, statErr := os.Stat(dir); statErr == nil {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dir := filepath.Join(wd, "data", "uploads")
		if _, statErr := os.Stat(dir); statErr == nil {
			return dir
		}
	}
	return config.C.Upload.Dir
}

// statOK 判断路径是否存在（供静态目录定位复用）
func statOK(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// absPath 把相对路径解析为绝对路径：优先以可执行文件目录为基准，其次当前工作目录。
// 静态资源（上传图片、演示图）必须用绝对路径注册，否则进程 CWD 变化时会 404，
// 进而被 NoRoute 兜底成 index.html（text/html），浏览器按 image/png 解析就报「图片加载异常」。
func absPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), p); statOK(cand) {
			return cand
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, p)
	}
	return p
}

func Setup() *gin.Engine {
	// 各端 handler 只在这里实例化一次（handler 内部自带 service / repository），
	// 路由注册时直接取方法值，避免每条路由各自 New 一遍。
	accH := account.NewHandler()   // 账号内核：登录 / 个人资料（跨端端点）
	custH := customer.NewHandler() // 客户内核：订单 / 商品（跨端端点）
	upH := upload.NewHandler()     // 附件上传（跨端端点）
	clientH := client.NewHandler() // 用户端（访客）
	agentH := agent.NewHandler()   // 客服端（工作台 / APP / 桌面端）
	adminH := admin.NewHandler()   // 管理后台

	gin.SetMode(config.C.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())

	r.StaticFS("/uploads", gin.Dir(absPath(config.C.Upload.Dir), false))
	r.Static("/demo", absPath(filepath.Join("data", "demo")))

	// 前端三端独立入口：/ 与 /kefu/（用户端）、/chat/（客服工作台）、/admin/（管理后台）
	// 前端 `pnpm build` 已直接把产物输出到 server/web，这里只读托管。
	webDir := WebDir()
	if _, err := os.Stat(webDir); err != nil {
		log.Printf("[router] 未找到前端产物目录 %s：请先执行 `cd web && pnpm install && pnpm build`", webDir)
	}
	// 带内容哈希的静态资源可长期缓存；找不到时返回 404，避免浏览器报 MIME 错误
	r.GET("/assets/*filepath", func(c *gin.Context) {
		p := filepath.Join(webDir, "assets", filepath.Clean("/"+c.Param("filepath")))
		if _, err := os.Stat(p); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.File(p)
	})
	// 前端 public 目录下的原样拷贝资源（兜底图等）
	r.Static("/img", filepath.Join(webDir, "img"))
	r.StaticFile("/favicon.svg", filepath.Join(webDir, "favicon.svg"))
	r.StaticFile("/icons.svg", filepath.Join(webDir, "icons.svg"))

	// 根路径：用户端（index.html）
	r.GET("/", spaEntry(webDir, "index.html"))

	// 每个前缀返回自己的入口 HTML，并对前缀下的任意路径做 SPA 回退。
	// gin 的路由树不允许同层同时出现 "/x" 与 "/x/*any"，
	// 因此只注册 "/x/*any"（它会同时匹配 "/x"），避免冲突。
	for _, e := range ends {
		entry := spaEntry(webDir, e.Entry)
		r.GET("/"+e.Prefix+"/*any", entry)
	}

	// 旧的单入口路径自动跳到对应端
	r.GET("/c", func(c *gin.Context) { c.Redirect(http.StatusFound, "/kefu/") })
	r.GET("/workbench", func(c *gin.Context) { c.Redirect(http.StatusFound, "/chat/") })

	// 无尾斜杠的 /admin、/chat、/kefu 补斜杠（gin 的 /*any 不匹配裸前缀）
	for _, e := range ends {
		prefix := "/" + e.Prefix
		r.GET(prefix, func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, prefix+"/") })
	}

	// 其它未知路径回退到用户端首页
	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= len("/api") && c.Request.URL.Path[:len("/api")] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "接口不存在"})
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})

	// WebSocket：客服端（需 token）与用户端/访客（凭 channel+uid）两条独立通道
	r.GET("/ws", agentH.WSHandler)
	r.GET("/ws/guest", clientH.GuestWSHandler)

	// 路由汇总：路径 / 方法 / 中间件与拆分前逐字一致，处理器改为各端 handler 的方法
	// （handler 实例在 Setup 开头创建一次；模块内方法统一带 Handler 后缀，避免与 service 同名）。
	g := r.Group("/api")
	{
		// 认证
		g.POST("/auth/login", accH.LoginHandler)
		g.POST("/auth/logout", middleware.JWTAuth(), accH.LogoutHandler)
		g.GET("/auth/me", middleware.JWTAuth(), accH.MeHandler)
		g.PUT("/auth/profile", middleware.JWTAuth(), accH.UpdateProfileHandler)
		g.PUT("/auth/password", middleware.JWTAuth(), accH.ChangePasswordHandler)

		// 用户端（GET 参数传递渠道等信息，无需登录）
		g.GET("/guest/init", clientH.GuestInitHandler)
		g.POST("/guest/message", clientH.GuestSendHandler)
		g.GET("/guest/messages", clientH.GuestMessagesHandler)
		// 用户端上传附件（图片/视频/文件）
		g.POST("/guest/upload", upH.UploadHandler)

		authed := g.Group("", middleware.JWTAuth())
		{
			// 渠道
			authed.GET("/channels", adminH.ListChannelsHandler)
			authed.POST("/channels", adminH.CreateChannelHandler)
			authed.PUT("/channels/:id", adminH.UpdateChannelHandler)
			authed.DELETE("/channels/:id", adminH.DeleteChannelHandler)
			// 重置渠道标识（app_id）：生成新的随机标识，旧链接失效
			authed.POST("/channels/:id/reset-app-id", adminH.ResetChannelAppIDHandler)

			// 渠道二维码 / 接入地址
			authed.GET("/channels/:id/qr", adminH.ChannelQRInfoHandler)
			authed.GET("/channels/:id/qr.png", adminH.ChannelQRPNGHandler)
			// 渠道接待账号快速设置
			authed.GET("/channels/:id/agents", adminH.ListChannelAgentsHandler)
			authed.PUT("/channels/:id/agents", adminH.SetChannelAgentsHandler)

			// 渠道类型库（自定义类型，不限制枚举）
			authed.GET("/channel-types", adminH.ListChannelTypesHandler)
			authed.POST("/channel-types", adminH.CreateChannelTypeHandler)
			authed.PUT("/channel-types/:id", adminH.UpdateChannelTypeHandler)
			authed.DELETE("/channel-types/:id", adminH.DeleteChannelTypeHandler)

			// 子账号
			authed.GET("/agents", adminH.ListAgentsHandler)
			authed.POST("/agents", adminH.CreateAgentHandler)
			authed.PUT("/agents/:id", adminH.UpdateAgentHandler)
			authed.DELETE("/agents/:id", adminH.DeleteAgentHandler)
			authed.POST("/agents/:id/online", adminH.SetOnlineHandler)

			// 会话
			authed.GET("/conversations", agentH.ListConversationsHandler)
			authed.GET("/conversations/stats", agentH.ConversationStatsHandler)
			authed.GET("/conversations/:id", agentH.GetConversationHandler)
			authed.POST("/conversations/:id/claim", agentH.ClaimConversationHandler)
			authed.POST("/conversations/:id/transfer", agentH.TransferConversationHandler)
			authed.POST("/conversations/:id/close", agentH.CloseConversationHandler)
			authed.POST("/conversations/:id/read", agentH.MarkReadHandler)
			// 会话归档 / 删除 / 消息搜索
			authed.POST("/conversations/archive-bulk", agentH.ArchiveConversationsBulkHandler)
			authed.POST("/conversations/:id/archive", agentH.ArchiveConversationHandler)
			authed.POST("/conversations/:id/unarchive", agentH.UnarchiveConversationHandler)
			authed.DELETE("/conversations/:id", agentH.DeleteConversationHandler)
			authed.GET("/conversations/:id/messages/search", agentH.SearchConversationMessagesHandler)
			authed.GET("/conversations/:id/messages", agentH.ListMessagesHandler)
			authed.POST("/conversations/:id/messages", agentH.SendMessageHandler)

			// 客户与历史档案
			authed.GET("/conversation-records", adminH.SearchConversationRecordsHandler)
			authed.GET("/conversation-records/:id/messages", adminH.GetConversationMessagesHandler)
			authed.GET("/users", adminH.ListCustomersHandler)
			authed.GET("/users/:id", agentH.GetCustomerHandler)
			authed.PUT("/users/:id/remark", adminH.UpdateCustomerRemarkHandler)

			// 客户标签
			authed.GET("/tags", adminH.ListTagsHandler)
			authed.POST("/tags", adminH.CreateTagHandler)
			authed.DELETE("/tags/:id", adminH.DeleteTagHandler)
			authed.PUT("/users/:id/tags", agentH.SetCustomerTagsHandler)

			// 订单 / 商品
			authed.GET("/orders", custH.ListOrdersHandler)
			authed.GET("/products", custH.ListProductsHandler)

			// 自动回复规则（关键词 → 机器人回复 / 转人工）
			authed.GET("/auto-replies", adminH.ListAutoRepliesHandler)
			authed.POST("/auto-replies", adminH.CreateAutoReplyHandler)
			authed.PUT("/auto-replies/:id", adminH.UpdateAutoReplyHandler)
			authed.DELETE("/auto-replies/:id", adminH.DeleteAutoReplyHandler)

			// 数据统计
			authed.GET("/stats/overview", adminH.StatsOverviewHandler)

			// 系统配置（默认图片等）
			authed.GET("/settings", adminH.ListSettingsHandler)
			authed.PUT("/settings", adminH.SaveSettingHandler)

			// 设备（APP / PC 桌面端推送注册）
			authed.GET("/devices", agentH.ListDevicesHandler)
			authed.POST("/devices", agentH.RegisterDeviceHandler)
			authed.DELETE("/devices/:id", agentH.DeleteDeviceHandler)
			authed.POST("/devices/test-push", adminH.TestDevicePushHandler)

			// 离线消息同步
			authed.GET("/offline/messages", agentH.OfflineMessagesHandler)
			authed.GET("/offline/notifications", agentH.OfflineNotificationsHandler)
			authed.POST("/offline/notifications/read", agentH.ReadNotificationsHandler)

			// 演示
			authed.POST("/demo/incoming", agentH.DemoIncomingHandler)
			authed.GET("/demo/products", agentH.DemoProductsHandler)

			// 上传附件（图片 / 视频 / 文件）
			authed.POST("/upload", upH.UploadHandler)
		}
	}
	return r
}
