// Package console 提供压测控制台：任务管理、AI 接入、HTTP 接口、报告落盘。
package console

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"kefu-loadtest/internal/aiclient"
	"kefu-loadtest/internal/advise"
	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/engine"
	"kefu-loadtest/internal/logbus"
	"kefu-loadtest/internal/metrics"
)

// Manager 管理压测任务、AI 配置与已有报告。
type Manager struct {
	mu         sync.Mutex
	run        *engine.Run
	last       *engine.Report
	lastAdvice *advise.Result
	reportDir  string
	logs       *logbus.Bus
	aiCfg      aiclient.Config
	ai         *aiclient.Client
}

// NewManager 创建任务管理器，reportDir 为报告落盘目录。
func NewManager(reportDir string) *Manager {
	m := &Manager{reportDir: reportDir, logs: logbus.New(3000)}
	m.SetAI(aiclient.FromEnv())
	return m
}

// Logs 返回序号大于 since 的日志与当前游标。
func (m *Manager) Logs(since int64) ([]logbus.Entry, int64) { return m.logs.Since(since) }

// Log 直接写一条控制台日志。
func (m *Manager) Log(level, msg string) { m.logs.Add(level, msg) }

// AI 返回当前大模型客户端（可能未配置）。
func (m *Manager) AI() *aiclient.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ai
}

// AIConfig 返回当前大模型配置。
func (m *Manager) AIConfig() aiclient.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.aiCfg
}

// SetAI 更新大模型配置。
func (m *Manager) SetAI(cfg aiclient.Config) {
	cfg.Normalize()
	m.mu.Lock()
	m.aiCfg = cfg
	m.ai = aiclient.New(cfg)
	m.mu.Unlock()
	if cfg.Ready() {
		m.logs.Add(logbus.LevelOK, fmt.Sprintf("已接入大模型：%s（%s）", cfg.Model, cfg.BaseURL))
	} else {
		m.logs.Add(logbus.LevelInfo, "未配置大模型，接口整理与结果分析将使用内置规则引擎")
	}
}

// SetAdvice 保存最近一次分析结果。
func (m *Manager) SetAdvice(a *advise.Result) {
	m.mu.Lock()
	m.lastAdvice = a
	m.mu.Unlock()
}

// Advice 返回最近一次分析结果。
func (m *Manager) Advice() *advise.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastAdvice
}

// ReportDir 返回报告目录。
func (m *Manager) ReportDir() string { return m.reportDir }

// Start 启动一次压测。
func (m *Manager) Start(cfg config.Config) (config.Config, error) {
	m.mu.Lock()
	if m.run != nil && !m.run.Finished() {
		m.mu.Unlock()
		return cfg, fmt.Errorf("已有压测任务在运行，请先停止")
	}
	run, err := engine.New(cfg)
	if err != nil {
		m.mu.Unlock()
		m.logs.Add(logbus.LevelError, "参数校验失败："+err.Error())
		return cfg, err
	}
	m.logs.Reset()
	m.lastAdvice = nil
	m.logs.Add(logbus.LevelInfo, fmt.Sprintf("收到压测指令：%d 个接口，总并发 %d",
		len(run.Config().Scenarios), run.Config().TotalConcurrency()))
	run.SetLogger(m.logs.Add)

	dir := m.reportDir
	m.run = run
	m.mu.Unlock()

	run.Start()

	go func(r *engine.Run) {
		<-r.Done()
		rep := engine.BuildReport(r, r.Stopped())
		if files, err := SaveReport(dir, rep); err == nil {
			rep.Files = files
			m.logs.Add(logbus.LevelInfo, fmt.Sprintf("报告已保存：%v", files))
		} else {
			m.logs.Add(logbus.LevelWarn, "报告保存失败："+err.Error())
		}
		level := logbus.LevelOK
		if !rep.Verdict.Pass {
			level = logbus.LevelError
		}
		m.logs.Add(level, "稳定性结论："+rep.Verdict.Level+" — "+joinReasons(rep.Verdict.Reasons))
		m.mu.Lock()
		m.last = rep
		m.mu.Unlock()
	}(run)
	return run.Config(), nil
}

func joinReasons(reasons []string) string {
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += "；"
		}
		out += r
	}
	return out
}

// Stop 停止当前任务并返回是否真的有任务在跑。
func (m *Manager) Stop() bool {
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	if run == nil || run.Finished() {
		return false
	}
	run.Stop()
	return true
}

// Running 返回是否有任务在跑。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.run != nil && !m.run.Finished()
}

// LastReport 返回最近一次报告。
func (m *Manager) LastReport() *engine.Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// Status 返回控制台状态。
func (m *Manager) Status() Status {
	m.mu.Lock()
	run := m.run
	last := m.last
	lastAdvice := m.lastAdvice
	aiCfg := m.aiCfg
	m.mu.Unlock()

	st := Status{
		ServerTime: time.Now().Format("2006-01-02 15:04:05"),
		AI: AIState{
			Ready:   aiCfg.Ready(),
			BaseURL: aiCfg.BaseURL,
			Model:   aiCfg.Model,
			KeyMask: aiCfg.MaskedKey(),
		},
	}
	if run == nil {
		st.Config = config.Default()
		st.Summary = engine.RunSummary{Summary: metrics.New(time.Now()).Snapshot()}
	} else {
		st.Running = !run.Finished()
		st.Config = run.Config()
		st.StartedAt = run.StartedAt().Format("2006-01-02 15:04:05")
		st.Summary = run.Summary()
		if run.Finished() {
			st.Progress = 1
		} else {
			st.Progress = progress(run)
		}
	}
	if last != nil {
		st.HasReport = true
		st.ReportAt = last.FinishedAt.Format("2006-01-02 15:04:05")
		v := last.Verdict
		st.Verdict = &v
		st.LastReportFiles = last.Files
	}
	if lastAdvice != nil {
		st.HasAdvice = true
		st.AdviceSource = lastAdvice.Source
	}
	return st
}

// AIState 是回显给页面的 AI 配置状态（不含明文密钥）。
type AIState struct {
	Ready   bool   `json:"ready"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	KeyMask string `json:"key_mask"`
}

// Status 是 /api/status 的响应体。
type Status struct {
	Running         bool              `json:"running"`
	Progress        float64           `json:"progress"`
	Config          config.Config     `json:"config"`
	Summary         engine.RunSummary `json:"summary"`
	StartedAt       string            `json:"started_at,omitempty"`
	HasReport       bool              `json:"has_report"`
	ReportAt        string            `json:"report_at,omitempty"`
	Verdict         *engine.Verdict   `json:"verdict,omitempty"`
	LastReportFiles []string          `json:"last_report_files,omitempty"`
	HasAdvice       bool              `json:"has_advice"`
	AdviceSource    string            `json:"advice_source,omitempty"`
	AI              AIState           `json:"ai"`
	ServerTime      string            `json:"server_time"`
}

func progress(run *engine.Run) float64 {
	cfg := run.Config()
	sum := run.Summary()
	switch {
	case cfg.TotalMessages > 0:
		return clamp(float64(sum.Sent) / float64(cfg.TotalMessages))
	case cfg.DurationSec > 0:
		return clamp(sum.ElapsedSec / float64(cfg.DurationSec))
	}
	planned := 0
	for _, s := range cfg.Scenarios {
		planned += s.Concurrency * s.PerUserMessages
	}
	if planned > 0 {
		return clamp(float64(sum.Sent) / float64(planned))
	}
	return 0
}

func clamp(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// SaveReport 把报告写入 reportDir，返回生成的文件路径。
func SaveReport(reportDir string, rep *engine.Report) ([]string, error) {
	if reportDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return nil, err
	}
	base := filepath.Join(reportDir, "run-"+rep.StartedAt.Format("20060102-150405"))
	jsonPath := base + ".json"
	if err := os.WriteFile(jsonPath, ReportJSON(rep), 0o644); err != nil {
		return nil, err
	}
	csvPath := base + ".csv"
	if err := os.WriteFile(csvPath, ReportCSV(rep), 0o644); err != nil {
		return []string{jsonPath}, err
	}
	return []string{jsonPath, csvPath}, nil
}

// SaveAdvice 把分析报告写成 markdown，返回文件路径。
func SaveAdvice(reportDir string, rep *engine.Report, a *advise.Result) (string, error) {
	if reportDir == "" || rep == nil || a == nil {
		return "", nil
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(reportDir, "advice-"+rep.StartedAt.Format("20060102-150405")+".md")
	header := fmt.Sprintf("# 压测分析与整改建议\n\n- 目标：%s\n- 时间：%s\n- 来源：%s\n\n",
		rep.Config.Target, rep.StartedAt.Format("2006-01-02 15:04:05"), a.Source)
	if err := os.WriteFile(path, []byte(header+a.Text+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ReportJSON 把报告序列化为带缩进的 JSON。
func ReportJSON(rep *engine.Report) []byte {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(b, '\n')
}

// ReportCSV 把每秒时序导出为 CSV（可用 Excel 直接打开做趋势图）。
func ReportCSV(rep *engine.Report) []byte {
	var buf writerBuffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"second", "sent", "ok", "fail", "avg_latency_ms"})
	for _, p := range rep.Summary.Series {
		_ = w.Write([]string{
			strconv.Itoa(p.Second),
			strconv.FormatInt(p.Sent, 10),
			strconv.FormatInt(p.Ok, 10),
			strconv.FormatInt(p.Fail, 10),
			strconv.FormatFloat(p.AvgMs, 'f', 2, 64),
		})
	}
	w.Flush()
	out := []byte{0xEF, 0xBB, 0xBF}
	return append(out, buf.b...)
}

type writerBuffer struct{ b []byte }

func (w *writerBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
