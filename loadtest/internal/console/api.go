package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kefu-loadtest/internal/aiclient"
	"kefu-loadtest/internal/advise"
	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/discover"
	"kefu-loadtest/internal/logbus"
)

// NewServer 组装控制台 HTTP 路由。
func NewServer(web fs.FS, m *Manager) http.Handler {
	s := &Server{m: m, web: web}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/defaults", s.handleDefaults)

	mux.HandleFunc("GET /api/ai/config", s.handleAIConfigGet)
	mux.HandleFunc("POST /api/ai/config", s.handleAIConfigSet)
	mux.HandleFunc("POST /api/ai/discover", s.handleDiscover)
	mux.HandleFunc("POST /api/ai/advise", s.handleAdvise)

	mux.HandleFunc("POST /api/probe", s.handleProbe)
	mux.HandleFunc("POST /api/probe/one", s.handleProbeOne)

	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/stop", s.handleStop)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/report", s.handleReportJSON)
	mux.HandleFunc("GET /api/report.csv", s.handleReportCSV)
	mux.HandleFunc("GET /api/advice.md", s.handleAdviceMarkdown)
	mux.Handle("GET /", http.FileServerFS(web))
	return mux
}

// Server 持有控制台依赖。
type Server struct {
	m   *Manager
	web fs.FS
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "message": msg, "data": nil})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, map[string]any{"running": s.m.Running(), "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) handleDefaults(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, config.Default())
}

/* ------------------------------ AI 配置 ------------------------------ */

func (s *Server) handleAIConfigGet(w http.ResponseWriter, _ *http.Request) {
	cfg := s.m.AIConfig()
	writeOK(w, AIState{Ready: cfg.Ready(), BaseURL: cfg.BaseURL, Model: cfg.Model, KeyMask: cfg.MaskedKey()})
}

func (s *Server) handleAIConfigSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL    string `json:"base_url"`
		APIKey     string `json:"api_key"`
		Model      string `json:"model"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "参数格式错误："+err.Error())
		return
	}
	cur := s.m.AIConfig()
	cfg := aiclient.Config{
		BaseURL:    req.BaseURL,
		Model:      req.Model,
		TimeoutSec: req.TimeoutSec,
	}
	if strings.TrimSpace(req.APIKey) == "" || strings.Contains(req.APIKey, "****") {
		cfg.APIKey = cur.APIKey // 未填或填的是脱敏串，保留原密钥
	} else {
		cfg.APIKey = req.APIKey
	}
	s.m.SetAI(cfg)
	saved := s.m.AIConfig()
	writeOK(w, AIState{Ready: saved.Ready(), BaseURL: saved.BaseURL, Model: saved.Model, KeyMask: saved.MaskedKey()})
}

/* ------------------------------ 接口勘察 ------------------------------ */

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target      string `json:"target"`
		Channel     string `json:"channel"`
		Concurrency int    `json:"concurrency"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "参数格式错误："+err.Error())
		return
	}
	if strings.TrimSpace(req.Target) == "" {
		writeErr(w, http.StatusBadRequest, "请填写目标地址")
		return
	}

	ai := s.m.AI()
	s.m.Log(logbus.LevelInfo, fmt.Sprintf("开始勘察目标 %s（%s）", req.Target, aiSource(ai)))
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	res := discover.Run(ctx, req.Target, req.Channel, ai, req.Concurrency)

	for _, f := range res.Facts {
		level := logbus.LevelInfo
		switch f.Level {
		case "ok":
			level = logbus.LevelOK
		case "warn":
			level = logbus.LevelWarn
		case "error":
			level = logbus.LevelError
		}
		s.m.Log(level, "勘察｜"+f.Kind+"："+f.Detail)
	}
	if res.Error != "" {
		s.m.Log(logbus.LevelWarn, res.Error)
	}
	s.m.Log(logbus.LevelOK, fmt.Sprintf("勘察完成：整理出 %d 个候选接口（来源：%s，耗时 %.0fms）",
		len(res.Candidates), sourceText(res.Source), res.ElapsedMs))
	writeOK(w, res)
}

func aiSource(ai *aiclient.Client) string {
	if ai.Ready() {
		return "大模型 " + ai.Model() + " 整理"
	}
	return "未配置大模型，使用内置规则"
}

func sourceText(src string) string {
	if src == "ai" {
		return "大模型"
	}
	return "内置规则"
}

/* ------------------------------ 结果分析 ------------------------------ */

func (s *Server) handleAdvise(w http.ResponseWriter, r *http.Request) {
	rep := s.m.LastReport()
	if rep == nil {
		writeErr(w, http.StatusBadRequest, "还没有完成的压测报告，先跑一轮压测")
		return
	}
	ai := s.m.AI()
	s.m.Log(logbus.LevelInfo, "开始分析压测结果（"+aiSource(ai)+"）")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res := advise.Run(ctx, rep, ai)
	if res.Error != "" {
		s.m.Log(logbus.LevelWarn, res.Error)
	}
	path, err := SaveAdvice(s.m.ReportDir(), rep, &res)
	if err != nil {
		s.m.Log(logbus.LevelWarn, "分析报告保存失败："+err.Error())
	} else if path != "" {
		s.m.Log(logbus.LevelInfo, "分析报告已保存："+path)
	}
	s.m.SetAdvice(&res)
	s.m.Log(logbus.LevelOK, "分析完成（来源："+sourceText(res.Source)+"）")
	writeOK(w, map[string]any{
		"source": res.Source,
		"model":  res.Model,
		"text":   res.Text,
		"error":  res.Error,
		"file":   path,
	})
}

func (s *Server) handleAdviceMarkdown(w http.ResponseWriter, _ *http.Request) {
	a := s.m.Advice()
	if a == nil {
		writeErr(w, http.StatusNotFound, "还没有分析结果")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="advice.md"`)
	_, _ = w.Write([]byte(a.Text))
}

/* ------------------------------ 连通性 / 单接口试跑 ------------------------------ */

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target    string `json:"target"`
		Channel   string `json:"channel"`
		UID       string `json:"uid"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "参数格式错误："+err.Error())
		return
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	res := Probe(req.Target, req.Channel, req.UID, timeout)
	if res.Pass {
		s.m.Log(logbus.LevelOK, fmt.Sprintf("连通性自检通过：初始化 %.0fms，发消息 %.0fms（消息 ID=%d）",
			res.InitMs, res.SendMs, res.MessageID))
	} else {
		s.m.Log(logbus.LevelError, fmt.Sprintf("连通性自检失败：%s", res.Verdict))
	}
	writeOK(w, res)
}

func (s *Server) handleProbeOne(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target    string          `json:"target"`
		Channel   string          `json:"channel"`
		Token     string          `json:"token"`
		TimeoutMs int             `json:"timeout_ms"`
		Scenario  config.Scenario `json:"scenario"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "参数格式错误："+err.Error())
		return
	}
	if strings.TrimSpace(req.Scenario.Path) == "" {
		writeErr(w, http.StatusBadRequest, "接口路径为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res := ProbeOne(ctx, req.Target, req.Channel, "", req.Token, req.Scenario,
		time.Duration(req.TimeoutMs)*time.Millisecond)
	if res.OK {
		s.m.Log(logbus.LevelOK, fmt.Sprintf("试跑 %s %s 通过：HTTP %d，%.0fms",
			req.Scenario.Method, req.Scenario.Path, res.Status, res.MS))
	} else {
		s.m.Log(logbus.LevelError, fmt.Sprintf("试跑 %s %s 失败：%s",
			req.Scenario.Method, req.Scenario.Path, res.Verdict))
	}
	writeOK(w, res)
}

/* ------------------------------ 压测任务 ------------------------------ */

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	cfg := config.Default()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "参数格式错误："+err.Error())
		return
	}
	applied, err := s.m.Start(cfg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{"started": true, "config": applied})
}

func (s *Server) handleStop(w http.ResponseWriter, _ *http.Request) {
	stopped := s.m.Stop()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && s.m.Running() {
		time.Sleep(100 * time.Millisecond)
	}
	writeOK(w, map[string]any{"stop_requested": stopped, "running": s.m.Running()})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, s.m.Status())
}

// handleLogs 增量返回压测过程日志，前端用游标 since 拉取。
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	var since int64
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	entries, cursor := s.m.Logs(since)
	writeOK(w, map[string]any{"entries": entries, "cursor": cursor})
}

func (s *Server) handleReportJSON(w http.ResponseWriter, _ *http.Request) {
	rep := s.m.LastReport()
	if rep == nil {
		writeErr(w, http.StatusNotFound, "还没有已完成的压测报告")
		return
	}
	writeOK(w, rep)
}

func (s *Server) handleReportCSV(w http.ResponseWriter, _ *http.Request) {
	rep := s.m.LastReport()
	if rep == nil {
		writeErr(w, http.StatusNotFound, "还没有已完成的压测报告")
		return
	}
	name := fmt.Sprintf("loadtest-%s.csv", rep.StartedAt.Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(ReportCSV(rep))
}
