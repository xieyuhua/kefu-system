// 客服系统 / HTTP 系统并发压测台（独立项目）。
//
//	go run .                                  # Web 控制台：地址勘察 → 勾选接口 → 压测 → AI 分析
//	go run . -cli -c 20 -duration 30 -msg "你好"   # 命令行直接压测并输出分析
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
	"strings"
	"sync"
	"syscall"
	"time"

	"kefu-loadtest/internal/aiclient"
	"kefu-loadtest/internal/advise"
	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/console"
	"kefu-loadtest/internal/engine"
	"kefu-loadtest/internal/logbus"
)

//go:embed all:web
var webEmbed embed.FS

var (
	port      = flag.Int("port", 8600, "Web 控制台监听端口")
	reportDir = flag.String("report-dir", "reports", "报告输出目录")
	cliMode   = flag.Bool("cli", false, "命令行模式：不启动 Web 控制台，直接压测并打印分析")

	target   = flag.String("target", "http://127.0.0.1:8480", "被压测的系统地址")
	channel  = flag.String("channel", "web", "访客渠道标识，如 web / wechat / wecom")
	token    = flag.String("token", "", "需要登录的接口用的 Bearer Token")
	conc     = flag.Int("c", 20, "并发数量（虚拟用户数）")
	perUser  = flag.Int("n", 0, "每个并发用户发送条数，0 表示按 -duration 跑")
	dur      = flag.Int("duration", 15, "持续时长（秒），0 表示按条数跑")
	total    = flag.Int64("total", 0, "全局总条数上限，0 表示不限制")
	interval = flag.Int("interval", 500, "两条请求之间的间隔（毫秒）")
	jitter   = flag.Int("jitter", 150, "间隔随机抖动（±毫秒）")
	rampup   = flag.Int("rampup", 0, "并发爬坡总时长（毫秒）")
	timeout  = flag.Int("timeout", 15000, "单请求超时（毫秒）")
	uidMode  = flag.String("uid-mode", config.UIDModePerUser, "客户 ID 策略：per-user / single / random")
	uidPre   = flag.String("uid-prefix", "lt_", "客户 ID 前缀")
	nick     = flag.String("nick", "压测用户", "客户昵称前缀")
	msg      = flag.String("msg", "", "消息内容，多条用 | 分隔，支持 {{seq}} {{vu}} {{ts}} 占位符")
	msgFile  = flag.String("msg-file", "", "消息内容文件（一行一条）")
	method   = flag.String("method", "POST", "业务接口的请求方法")
	path     = flag.String("path", "/api/guest/message?channel={{channel}}&uid={{uid}}", "业务接口路径（支持占位符）")
	bodyTpl  = flag.String("body", "", "请求体模板，留空则按消息内容生成 {\"type\":\"text\",\"content\":\"...\"}")
	wsFlag   = flag.Bool("ws", false, "同时建立访客 WebSocket 长连接")
	skipInit = flag.Bool("skip-init", false, "跳过 /api/guest/init 初始化请求（仅对已存在的客户有效）")

	aiBase  = flag.String("ai-base", "", "大模型 base_url（默认读环境变量 KEFU_LT_AI_BASE_URL）")
	aiKey   = flag.String("ai-key", "", "大模型 api_key（默认读环境变量 KEFU_LT_AI_KEY）")
	aiModel = flag.String("ai-model", "", "大模型 model（默认读环境变量 KEFU_LT_AI_MODEL）")
)

func main() {
	flag.Parse()
	if !*cliMode {
		runServer()
		return
	}
	cfg, err := buildConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "参数错误："+err.Error())
		os.Exit(1)
	}
	runCLI(cfg)
}

// buildConfig 从命令行参数组装压测配置（单接口模式）。
func buildConfig() (config.Config, error) {
	cfg := config.Default()
	cfg.Target = *target
	cfg.Channel = *channel
	cfg.Token = *token
	cfg.DurationSec = *dur
	cfg.TotalMessages = *total
	cfg.RampUpMs = *rampup
	cfg.TimeoutMs = *timeout
	cfg.UIDMode = *uidMode
	cfg.UIDPrefix = *uidPre
	cfg.NicknamePrefix = *nick
	cfg.UseWebSocket = *wsFlag
	cfg.SkipInit = *skipInit

	switch {
	case *msgFile != "":
		lines, err := readMessageFile(*msgFile)
		if err != nil {
			return cfg, err
		}
		cfg.Messages = lines
	case *msg != "":
		cfg.Messages = strings.Split(*msg, "|")
	}

	body := *bodyTpl
	if body == "" {
		body = `{"type":"text","content":"` + strings.ReplaceAll(cfg.Messages[0], `"`, `'`) + `"}`
	}
	setup := config.SetupGuestInit
	if *skipInit {
		setup = config.SetupNone
	}
	cfg.Scenarios = []config.Scenario{{
		ID:              "sc_cli",
		Name:            "命令行接口",
		Method:          *method,
		Path:            *path,
		Body:            body,
		Setup:           setup,
		Auth:            config.AuthNone,
		Concurrency:     *conc,
		IntervalMs:      *interval,
		JitterMs:        *jitter,
		PerUserMessages: *perUser,
		Risk:            config.RiskWrite,
	}}

	cfg.Normalize()
	return cfg, cfg.Validate()
}

func readMessageFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取消息文件失败：%w", err)
	}
	lines := make([]string, 0, 32)
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("消息文件里没有有效内容：%s", path)
	}
	return lines, nil
}

func aiConfig() aiclient.Config {
	cfg := aiclient.FromEnv()
	if *aiBase != "" {
		cfg.BaseURL = *aiBase
	}
	if *aiKey != "" {
		cfg.APIKey = *aiKey
	}
	if *aiModel != "" {
		cfg.Model = *aiModel
	}
	cfg.Normalize()
	return cfg
}

func runCLI(cfg config.Config) {
	run, err := engine.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "参数错误："+err.Error())
		os.Exit(1)
	}

	aiCfg := aiConfig()
	ai := aiclient.New(aiCfg)

	fmt.Println("=== 并发压测 ===")
	fmt.Printf("目标      : %s\n", cfg.Target)
	fmt.Printf("接口      : %s %s\n", cfg.Scenarios[0].Method, cfg.Scenarios[0].Path)
	fmt.Printf("并发数量  : %d\n", cfg.TotalConcurrency())
	if cfg.DurationSec > 0 {
		fmt.Printf("运行时长  : %d 秒\n", cfg.DurationSec)
	} else {
		fmt.Printf("每并发条数: %d\n", cfg.Scenarios[0].PerUserMessages)
	}
	fmt.Printf("发送间隔  : %dms ±%dms\n", cfg.Scenarios[0].IntervalMs, cfg.Scenarios[0].JitterMs)
	switch {
	case aiCfg.Ready():
		fmt.Printf("AI 分析   : %s（%s）\n", aiCfg.Model, aiCfg.BaseURL)
	default:
		fmt.Println("AI 分析   : 未配置大模型，将使用内置规则分析（可用 -ai-base/-ai-key/-ai-model 或环境变量开启）")
	}
	fmt.Println()

	run.SetLogger(func(level, msg string) {
		tag := map[string]string{
			logbus.LevelInfo:  "信息",
			logbus.LevelOK:    "完成",
			logbus.LevelWarn:  "注意",
			logbus.LevelError: "错误",
		}[level]
		if tag == "" {
			tag = level
		}
		fmt.Printf("%s [%s] %s\n", time.Now().Format("15:04:05"), tag, msg)
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	var once sync.Once
	go func() {
		<-sig
		once.Do(func() {
			fmt.Println("\n收到中断，正在停止压测并生成报告…")
			run.Stop()
		})
	}()

	run.Start()
	<-run.Done()

	rep := engine.BuildReport(run, run.Stopped())
	if files, err := console.SaveReport(*reportDir, rep); err != nil {
		fmt.Fprintln(os.Stderr, "报告保存失败："+err.Error())
	} else {
		rep.Files = files
	}
	printReport(rep)

	// 分析 + 整改方案
	ctx := context.Background()
	res := advise.Run(ctx, rep, ai)
	if res.Error != "" {
		fmt.Fprintln(os.Stderr, res.Error)
	}
	fmt.Println()
	fmt.Printf("=== 分析与整改建议（来源：%s）===\n\n", map[string]string{"ai": "大模型", "rules": "内置规则"}[res.Source])
	fmt.Println(res.Text)
	if path, err := console.SaveAdvice(*reportDir, rep, &res); err == nil && path != "" {
		fmt.Println("\n分析报告已保存：" + path)
	}

	if !rep.Verdict.Pass {
		os.Exit(2)
	}
}

func printReport(rep *engine.Report) {
	s := rep.Summary
	fmt.Println()
	fmt.Println("=== 压测结果 ===")
	fmt.Printf("总耗时        : %.1f 秒\n", rep.DurationSec)
	fmt.Printf("发起/成功/失败: %d / %d / %d（失败率 %.2f%%）\n", s.Sent, s.Ok, s.Failed, s.FailRate*100)
	if s.Inflight > 0 {
		fmt.Printf("未完成        : %d 条（压测结束时仍在途）\n", s.Inflight)
	}
	fmt.Printf("吞吐          : 平均 %.1f 条/秒，峰值 %.0f 条/秒\n", s.AvgRPS, s.PeakRPS)
	fmt.Printf("延迟(ms)      : 平均 %.0f  P50 %.0f  P90 %.0f  P95 %.0f  P99 %.0f  最大 %.0f\n",
		s.Latency.Avg, s.Latency.P50, s.Latency.P90, s.Latency.P95, s.Latency.P99, s.Latency.Max)
	if s.InitOk+s.InitFailed > 0 {
		fmt.Printf("客户初始化    : 成功 %d / 失败 %d\n", s.InitOk, s.InitFailed)
	}
	if s.WsConnects > 0 {
		fmt.Printf("长连接        : 建立 %d 次，断线 %d 次，收到事件 %d 条\n",
			s.WsConnects, s.WsDisconnects, s.WsEvents)
	}
	if len(rep.Scenarios) > 1 {
		fmt.Println("分接口        :")
		for _, sc := range rep.Scenarios {
			fmt.Printf("  %-28s 发起 %5d  失败 %4d  P95 %6.0fms\n", sc.Name, sc.Sent, sc.Failed, sc.Latency.P95)
		}
	}
	if len(s.Errors) > 0 {
		fmt.Println("错误分布      :")
		for i, e := range s.Errors {
			if i >= 10 {
				fmt.Printf("  … 另有 %d 类错误，详见报告文件\n", len(s.Errors)-10)
				break
			}
			fmt.Printf("  [%6d] %s | %s\n", e.Count, e.Kind, e.Detail)
		}
	}
	fmt.Printf("稳定性结论    : %s\n", rep.Verdict.Level)
	for _, r := range rep.Verdict.Reasons {
		fmt.Println("  - " + r)
	}
	if len(rep.Files) > 0 {
		fmt.Println("报告文件      :")
		for _, f := range rep.Files {
			fmt.Println("  " + f)
		}
	}
}

func runServer() {
	sub, err := fs.Sub(webEmbed, "web")
	if err != nil {
		log.Fatalf("加载内置页面失败：%v", err)
	}
	mgr := console.NewManager(*reportDir)
	if cfg := aiConfig(); cfg.Ready() {
		mgr.SetAI(cfg)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           console.NewServer(sub, mgr),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Println("\n正在关闭控制台…")
		if mgr.Running() {
			mgr.Stop()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	fmt.Println("HTTP 并发压测台（地址勘察 → 勾选接口 → AI 分析）")
	fmt.Println(strings.Repeat("-", 56))
	fmt.Printf("控制台地址 : http://%s/\n", addr)
	fmt.Printf("报告目录   : %s\n", *reportDir)
	if cfg := mgr.AIConfig(); cfg.Ready() {
		fmt.Printf("大模型     : %s（%s）\n", cfg.Model, cfg.BaseURL)
	} else {
		fmt.Println("大模型     : 未配置（页面右上角「AI 设置」可填 base_url / model / key）")
	}
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println(strings.Repeat("-", 56))

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("控制台启动失败（端口 %d 可能被占用）：%v\n换个端口：go run . -port %d", *port, err, *port+1)
	}
}
