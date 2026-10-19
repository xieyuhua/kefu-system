// Package route 路由组合入口：静态资源、SPA 兜底、健康检查，以及各端 /api 路由的汇总。
//
// 分工：
//   - 本文件：静态资源（/uploads、/demo、/assets、/img、favicon、icons）、三端 SPA 入口与兜底、
//     /health、根路径跳转；
//   - internal/common/route：跨端公共端点（登录 / 资料 / 上传 / 订单商品）；
//   - internal/{admin,agent,client}/route：各端专属端点，以及各自的 WebSocket 通道。
//
// 每条路由的 HTTP 方法、路径字符串、中间件与相对注册顺序在重排前后逐字一致。
package route

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	adminroute "kefu-server/internal/admin/route"
	agentroute "kefu-server/internal/agent/route"
	clientroute "kefu-server/internal/client/route"
	commonroute "kefu-server/internal/common/route"
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

// Setup 组装完整路由：静态资源 / SPA 入口 / 健康检查 / WebSocket，再按端汇总 /api 路由。
//
// 路由分三层归属：
//   - 本文件：静态资源、SPA 兜底、/health 与静态目录；
//   - internal/{admin,agent,client}/route：各端在 /api 下的注册（含各自的 WebSocket 通道）；
//   - internal/common/route：三端共用的登录 / 资料 / 上传 / 订单商品端点。
//
// 每条路由的 HTTP 方法、路径字符串、中间件与相对注册顺序与拆分前逐字一致。
func Setup() *gin.Engine {
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
			// 项目约定「HTTP 恒 200，错误看 body.code」：未匹配的 /api 路径也返回 200，
			// 由响应体里的 code=404 表达「接口不存在」，前端统一按 code 处理。
			c.JSON(http.StatusOK, gin.H{"code": 404, "message": "接口不存在"})
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})

	// WebSocket：客服端（需 token）与用户端/访客（凭 channel+uid）两条独立通道，
	// 归属各自的端（客服端 / 用户端），因此由各端 route 注册到引擎根。
	agentroute.RegisterWS(r)
	clientroute.RegisterWS(r)

	// /api 路由汇总：按端注册，路径 / 方法 / 中间件与拆分前逐字一致。
	g := r.Group("/api")
	authed := g.Group("", middleware.JWTAuth())
	{
		commonroute.Register(g, authed) // 登录 / 资料 / 上传 / 订单商品（跨端端点）
		clientroute.Register(g, authed) // 用户端（访客）
		adminroute.Register(g, authed)  // 管理后台
		agentroute.Register(g, authed)  // 客服端（工作台 / APP / 桌面端）
	}
	return r
}
