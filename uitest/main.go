// uitest-server：浏览器 UI 自动化测试平台的 Go 服务端。
//
//   - 内置浏览器：以独立用户目录启动 Edge/Chrome，通过 CDP 驱动；
//   - 录制：浏览器插件把操作步骤（含稳定 data-uitest-id 定位）上报到这里；
//   - 执行：先用 AI/规则生成数据准备，再把数据注入录制步骤跑真实浏览器；
//   - 观测：自带的网页控制台可以看到每条录制、每次运行、逐步日志与失败截图。
//
// 启动：go run . 或 go build -o uitest-server.exe . 后运行，默认 http://127.0.0.1:8490
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"uitest-server/internal/api"
	"uitest-server/internal/browser"
	"uitest-server/internal/bus"
	"uitest-server/internal/config"
	"uitest-server/internal/engine"
	"uitest-server/internal/model"
	"uitest-server/internal/store"
)

// Version 服务端版本号。
const Version = "1.0.0"

//go:embed all:web
var webFS embed.FS

//go:embed all:demo
var demoFS embed.FS

func main() {
	configPath := flag.String("config", "config.json", "配置文件路径")
	port := flag.Int("port", 0, "覆盖监听端口（默认取配置文件，8490）")
	headless := flag.Bool("headless", false, "内置浏览器使用无头模式")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[uitest] ")

	cfg := config.Load(*configPath)
	if *port > 0 {
		cfg.Server.Port = *port
	}
	if *headless {
		cfg.Browser.Headless = true
	}

	dataDir := config.AbsDataDir(cfg.Storage.Dir)
	db, err := store.Open(dataDir, cfg.Storage.KeepRuns)
	if err != nil {
		log.Fatalf("打开数据目录失败: %v", err)
	}
	defer db.Close()

	// 首次启动时把配置文件里的 AI / 浏览器设置灌入运行时设置
	st := db.Settings()
	changed := false
	if st.AI.BaseURL == "" && cfg.AI.BaseURL != "" {
		st.AI.BaseURL = cfg.AI.BaseURL
		changed = true
	}
	if st.AI.APIKey == "" && cfg.AI.APIKey != "" {
		st.AI.APIKey = cfg.AI.APIKey
		changed = true
	}
	if st.AI.Model == "" && cfg.AI.Model != "" {
		st.AI.Model = cfg.AI.Model
		changed = true
	}
	if st.Browser.Executable == "" && cfg.Browser.Executable != "" {
		st.Browser.Executable = cfg.Browser.Executable
		changed = true
	}
	if st.Browser.StepDelayMS == 0 {
		st.Browser.StepDelayMS = cfg.Browser.StepDelayMS
		changed = true
	}
	if st.Browser.ActionTimeoutMS == 0 {
		st.Browser.ActionTimeoutMS = cfg.Browser.ActionTimeoutMS
		changed = true
	}
	if st.AI.TimeoutSec == 0 {
		st.AI.TimeoutSec = cfg.AI.TimeoutSec
		changed = true
	}
	if changed {
		db.SaveSettings(st)
	}

	browserMgr := browser.NewManager(st.Browser,
		filepath.Join(dataDir, "browser-profile"),
		filepath.Join(dataDir, "logs"))
	eventBus := bus.New()
	runner := engine.New(db, browserMgr, eventBus)
	apiSrv := api.New(db, browserMgr, runner, eventBus, Version, cfg.Server.Port)

	root := http.NewServeMux()
	root.Handle("/api/", apiSrv.Handler())
	root.Handle("/data/runs/", http.StripPrefix("/data/runs/",
		http.FileServer(http.Dir(filepath.Join(dataDir, "runs")))))
	if sub, err := fs.Sub(demoFS, "demo"); err == nil {
		root.Handle("/demo/", http.StripPrefix("/demo/", noCache(http.FileServer(http.FS(sub)))))
	}
	root.Handle("/", consoleHandler())

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadHeaderTimeout: 15 * time.Second,
		// 不设 WriteTimeout：SSE 实时日志需要长连接
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		banner(cfg.Server.Host, cfg.Server.Port, dataDir, st)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到退出信号，正在关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = browserMgr.Close()
	log.Println("已退出")
}

func banner(host string, port int, dataDir string, st model.Settings) {
	base := fmt.Sprintf("http://%s:%d", host, port)
	exe, err := browser.FindExecutable(st.Browser.Executable)
	exeDesc := exe
	if err != nil {
		exeDesc = "未探测到（可在设置页手动填写）"
	}
	log.Printf("服务已启动：%s", base)
	log.Printf("  控制台      %s/", base)
	log.Printf("  示例被测页  %s/demo/   （可用来演练录制/执行）", base)
	log.Printf("  数据目录    %s", dataDir)
	log.Printf("  浏览器      %s", exeDesc)
	if st.AI.BaseURL == "" || st.AI.APIKey == "" {
		log.Printf("  AI 数据     未配置（使用内置规则生成，可在设置页填 base_url/api_key/model）")
	} else {
		log.Printf("  AI 数据     %s / %s", st.AI.BaseURL, st.AI.Model)
	}
	log.Printf("  插件录制    在 Edge/Chrome 中加载 uitest/extension 目录（详见 extension/README.md）")
}

// consoleHandler 托管内嵌的控制台静态资源。
//
// 注意：前端用 hash 路由，未命中的路径一律回退 index.html。这里必须直接写出
// index.html 的内容，不能把 r.URL.Path 改写成 "/index.html" 再交给 http.FileServer
// ——后者对以 /index.html 结尾的请求会 301 到 "./"，从而形成重定向死循环。
func consoleHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "控制台静态资源缺失", http.StatusInternalServerError)
		})
	}
	indexHTML, readErr := fs.ReadFile(sub, "index.html")
	fileServer := http.FileServer(http.FS(sub))

	serveIndex := func(w http.ResponseWriter) {
		if readErr != nil || len(indexHTML) == 0 {
			http.Error(w, "控制台 index.html 缺失：请确认 web/index.html 已随二进制打包", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w)
			return
		}
		if st, statErr := fs.Stat(sub, p); statErr != nil || st.IsDir() {
			serveIndex(w)
			return
		}
		if strings.HasSuffix(p, ".html") {
			w.Header().Set("Cache-Control", "no-store")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
