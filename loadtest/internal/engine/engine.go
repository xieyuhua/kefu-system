// Package engine 是压测执行引擎：按勾选好的接口场景拉起并发虚拟用户。
package engine

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/logbus"
	"kefu-loadtest/internal/metrics"
)

// scenario 是一个接口场景的运行态。
type scenario struct {
	spec  config.Scenario
	index int
	stats *metrics.Stats
}

// ScenarioSummary 是单个接口的指标。
type ScenarioSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Risk        string `json:"risk"`
	Concurrency int    `json:"concurrency"`
	Setup       string `json:"setup"`
	Auth        string `json:"auth"`
	metrics.Summary
}

// RunSummary 是整轮压测的指标（含各接口明细）。
type RunSummary struct {
	metrics.Summary
	TotalConcurrency int               `json:"total_concurrency"`
	Scenarios        []ScenarioSummary `json:"scenarios"`
}

// Run 代表一次压测任务。
type Run struct {
	cfg       config.Config
	base      string
	stats     *metrics.Stats // 全局汇总
	scenarios []*scenario
	startedAt time.Time
	client    *http.Client
	transport *http.Transport

	ctx      context.Context
	cancelFn context.CancelFunc
	done     chan struct{}
	started  atomic.Bool
	stopped  atomic.Bool
	finished atomic.Bool
	issued   atomic.Int64
	stopOnce sync.Once

	logger  func(level, msg string)
	logMu   sync.Mutex
	logLast map[string]time.Time
	logDrop map[string]int64
}

// New 校验参数并构造一次压测任务（尚未启动）。
func New(cfg config.Config) (*Run, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	concurrency := cfg.TotalConcurrency()
	tr := &http.Transport{
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: concurrency*2 + 32,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   !cfg.KeepAlive,
		DisableCompression:  true,
		ForceAttemptHTTP2:   false,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
	}
	now := time.Now()
	r := &Run{
		cfg:       cfg,
		base:      cfg.Target,
		client:    client,
		transport: tr,
		stats:     metrics.New(now),
		startedAt: now,
		done:      make(chan struct{}),
		logLast:   make(map[string]time.Time, 16),
		logDrop:   make(map[string]int64, 16),
	}
	for i, sp := range cfg.Scenarios {
		r.scenarios = append(r.scenarios, &scenario{spec: sp, index: i, stats: metrics.New(now)})
	}
	return r, nil
}

// SetLogger 注入日志输出回调。
func (r *Run) SetLogger(fn func(level, msg string)) { r.logger = fn }

// Config 返回本次任务的参数。
func (r *Run) Config() config.Config { return r.cfg }

// StartedAt 返回启动时间。
func (r *Run) StartedAt() time.Time { return r.startedAt }

// Stopped 表示是否被手动停止。
func (r *Run) Stopped() bool { return r.stopped.Load() }

// Finished 表示任务是否已经结束。
func (r *Run) Finished() bool { return r.finished.Load() }

// Done 在全部虚拟用户退出后关闭。
func (r *Run) Done() <-chan struct{} { return r.done }

// Summary 返回实时指标快照（含分接口明细）。
func (r *Run) Summary() RunSummary {
	out := RunSummary{
		Summary:          r.stats.Snapshot(),
		TotalConcurrency: r.cfg.TotalConcurrency(),
	}
	for _, sc := range r.scenarios {
		out.Scenarios = append(out.Scenarios, ScenarioSummary{
			ID:          sc.spec.ID,
			Name:        sc.spec.Name,
			Method:      sc.spec.Method,
			Path:        sc.spec.Path,
			Risk:        sc.spec.Risk,
			Concurrency: sc.spec.Concurrency,
			Setup:       sc.spec.Setup,
			Auth:        sc.spec.Auth,
			Summary:     sc.stats.SnapshotCompact(),
		})
	}
	return out
}

func (r *Run) logf(level, format string, args ...any) {
	if r.logger == nil {
		return
	}
	r.logger(level, fmt.Sprintf(format, args...))
}

func (r *Run) throttled(key string, window time.Duration) (int64, bool) {
	if r.logger == nil {
		return 0, false
	}
	r.logMu.Lock()
	defer r.logMu.Unlock()
	if last, ok := r.logLast[key]; ok && time.Since(last) < window {
		r.logDrop[key]++
		return 0, false
	}
	dropped := r.logDrop[key]
	r.logDrop[key] = 0
	r.logLast[key] = time.Now()
	return dropped, true
}

func (r *Run) logError(vu int, kind, action, detail string) {
	dropped, ok := r.throttled("err:"+kind, 2*time.Second)
	if !ok {
		return
	}
	if len(detail) > 200 {
		detail = detail[:200] + "..."
	}
	msg := fmt.Sprintf("并发#%d %s［%s］%s", vu+1, action, kind, detail)
	if dropped > 0 {
		msg += fmt.Sprintf(" · 此前 2 秒内同类 %d 条已折叠", dropped)
	}
	r.logf(logbus.LevelError, "%s", msg)
}

func (r *Run) logWarn(key, format string, args ...any) {
	dropped, ok := r.throttled("warn:"+key, 3*time.Second)
	if !ok {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if dropped > 0 {
		msg += fmt.Sprintf(" · 此前 3 秒内同类 %d 条已折叠", dropped)
	}
	r.logf(logbus.LevelWarn, "%s", msg)
}

// Start 拉起所有场景的并发虚拟用户。
func (r *Run) Start() {
	if !r.started.CompareAndSwap(false, true) {
		return
	}
	base, cancel := context.WithCancel(context.Background())
	ctx := base
	var timeoutCancel context.CancelFunc
	if r.cfg.DurationSec > 0 {
		ctx, timeoutCancel = context.WithTimeout(base, time.Duration(r.cfg.DurationSec)*time.Second)
	}
	r.ctx = ctx
	r.cancelFn = func() {
		if timeoutCancel != nil {
			timeoutCancel()
		}
		cancel()
	}

	r.logStart()

	var stagger time.Duration
	if r.cfg.RampUpMs > 0 && r.cfg.TotalConcurrency() > 1 {
		stagger = time.Duration(r.cfg.RampUpMs) * time.Millisecond / time.Duration(r.cfg.TotalConcurrency())
	}

	var wg sync.WaitGroup
	globalIdx := 0
	for _, sc := range r.scenarios {
		if sc.spec.Concurrency <= 0 {
			continue
		}
		for i := 0; i < sc.spec.Concurrency; i++ {
			wg.Add(1)
			idx := globalIdx
			globalIdx++
			go func(sc *scenario, localIdx, idx int) {
				defer wg.Done()
				if stagger > 0 && idx > 0 {
					t := time.NewTimer(time.Duration(idx) * stagger)
					select {
					case <-ctx.Done():
						t.Stop()
						return
					case <-t.C:
					}
				}
				if ctx.Err() != nil {
					return
				}
				v := newVU(r, sc, localIdx)
				v.loop(ctx)
			}(sc, i, idx)
		}
	}

	go r.progressLogger(ctx)

	go func() {
		wg.Wait()
		r.transport.CloseIdleConnections()
		r.stats.Freeze()
		for _, sc := range r.scenarios {
			sc.stats.Freeze()
		}
		r.logFinal()
		r.finished.Store(true)
		close(r.done)
	}()
}

func (r *Run) logStart() {
	c := r.cfg
	var end string
	switch {
	case c.DurationSec > 0:
		end = fmt.Sprintf("运行 %d 秒", c.DurationSec)
	case c.TotalMessages > 0:
		end = fmt.Sprintf("总条数 %d", c.TotalMessages)
	default:
		end = "按每并发条数跑"
	}
	r.logf(logbus.LevelInfo, "开始压测：目标 %s，%d 个接口，总并发 %d，%s",
		c.Target, len(c.Scenarios), c.TotalConcurrency(), end)
	for _, s := range c.Scenarios {
		r.logf(logbus.LevelInfo, "· %s → %s %s（并发 %d，间隔 %dms±%dms，风险 %s）",
			s.Name, s.Method, s.Path, s.Concurrency, s.IntervalMs, s.JitterMs, s.Risk)
	}
	if c.Token != "" {
		r.logf(logbus.LevelInfo, "已配置登录 Token，需要鉴权的接口会带上 Authorization: Bearer")
	}
	if c.UseWebSocket {
		r.logf(logbus.LevelInfo, "已开启访客 WebSocket 长连接校验")
	}
	if c.UIDMode == config.UIDModeSingle {
		r.logf(logbus.LevelWarn, "客户 ID 策略=全部共用一个客户，所有并发都压在同一个会话上")
	}
}

func (r *Run) progressLogger(ctx context.Context) {
	if r.logger == nil {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.done:
			return
		case <-t.C:
			s := r.stats.Snapshot()
			r.logf(logbus.LevelInfo, "[%3.0fs] 发起 %d · 成功 %d · 失败 %d · 当前 %.0f/s · 平均 %.0fms · P95 %.0fms",
				s.ElapsedSec, s.Sent, s.Ok, s.Failed, s.CurrentRPS, s.Latency.Avg, s.Latency.P95)
		}
	}
}

func (r *Run) logFinal() {
	sum := r.Summary()
	s := sum.Summary
	level := logbus.LevelOK
	if s.Failed > 0 {
		level = logbus.LevelWarn
	}
	r.logf(level, "压测结束：耗时 %.1fs · 发起 %d · 成功 %d · 失败 %d · 失败率 %.2f%% · 平均 %.0fms · P95 %.0fms · P99 %.0fms",
		s.ElapsedSec, s.Sent, s.Ok, s.Failed, s.FailRate*100, s.Latency.Avg, s.Latency.P95, s.Latency.P99)
	if s.Inflight > 0 {
		r.logf(logbus.LevelInfo, "另有 %d 条请求压测结束时仍在途（被停止或到时长），未计入成功率", s.Inflight)
	}
	for _, sc := range sum.Scenarios {
		r.logf(logbus.LevelInfo, "· %s：发起 %d · 成功 %d · 失败 %d · P95 %.0fms · 平均 %.0fms",
			sc.Name, sc.Sent, sc.Ok, sc.Failed, sc.Latency.P95, sc.Latency.Avg)
	}
	if len(s.Errors) > 0 {
		r.logf(logbus.LevelError, "失败最多的是［%s］%s（共 %d 次）", s.Errors[0].Kind, s.Errors[0].Detail, s.Errors[0].Count)
	}
}

// Stop 主动停止压测（幂等）。
func (r *Run) Stop() {
	r.stopOnce.Do(func() {
		r.stopped.Store(true)
		if r.logger != nil {
			r.logf(logbus.LevelWarn, "收到停止指令，等待在途请求结束…")
		}
		if r.cancelFn != nil {
			r.cancelFn()
		}
	})
}

// Wait 阻塞等待任务结束，最多等 d。
func (r *Run) Wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-r.done:
		return true
	case <-t.C:
		return false
	}
}
