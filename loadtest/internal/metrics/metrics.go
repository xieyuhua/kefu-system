// Package metrics 负责压测过程的指标采集与快照聚合。
//
// 设计要点：
//   - 每秒一个计数行，用 atomic 累加，避免高并发下的锁竞争；
//   - 延迟用固定桶（对数刻度）直方图，运行中即可算 p50/p95/p99；
//   - 错误按「类型 + 详情」聚合并保留最近若干条样本，便于定位问题。
package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MaxSeconds 单次压测最长可记录的秒数（2 小时）。
const MaxSeconds = 7200

// 延迟桶边界（毫秒），升序。
var histBounds = []float64{
	1, 2, 3, 4, 5, 7, 10, 15, 20, 30, 40, 60, 80, 120, 160,
	200, 300, 400, 600, 800, 1200, 1600, 2000, 3000, 4000,
	6000, 8000, 12000, 20000,
}

const maxSamples = 200

type secRow struct {
	started atomic.Int64 // 该秒内「发起」的请求数（真正的施压速率）
	ok      atomic.Int64 // 该秒内「完成」且成功
	fail    atomic.Int64 // 该秒内「完成」但失败
	okLatUs atomic.Int64
}

// LatencyStat 延迟统计（单位毫秒）。
type LatencyStat struct {
	Avg float64 `json:"avg_ms"`
	P50 float64 `json:"p50_ms"`
	P90 float64 `json:"p90_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

// SeriesPoint 是每秒一条的时序点，供前端画图。
type SeriesPoint struct {
	Second int   `json:"second"`
	Sent   int64 `json:"sent"`
	Ok     int64 `json:"ok"`
	Fail   int64 `json:"fail"`
	AvgMs  float64 `json:"avg_ms"`
}

// ErrorStat 错误聚合项。
type ErrorStat struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Count  int64  `json:"count"`
}

// ErrorSample 单条错误样本。
type ErrorSample struct {
	At        string  `json:"at"`
	VU        int     `json:"vu"`
	Kind      string  `json:"kind"`
	Detail    string  `json:"detail"`
	LatencyMs float64 `json:"latency_ms"`
}

// Summary 是某一时刻的完整指标快照。
type Summary struct {
	ElapsedSec  float64 `json:"elapsed_sec"`
	Sent        int64   `json:"sent"`
	Ok          int64   `json:"ok"`
	Failed      int64   `json:"failed"`
	Inflight    int64   `json:"inflight"`
	FailRate    float64 `json:"fail_rate"`
	ReplyHits   int64   `json:"reply_hits"`
	InitOk      int64   `json:"init_ok"`
	InitFailed  int64   `json:"init_failed"`
	CurrentRPS  float64 `json:"current_rps"`
	AvgRPS      float64 `json:"avg_rps"`
	PeakRPS     float64 `json:"peak_rps"`
	WsConnects  int64   `json:"ws_connects"`
	WsDisconnects int64 `json:"ws_disconnects"`
	WsLive      int64   `json:"ws_live"`
	WsEvents    int64   `json:"ws_events"`
	Latency     LatencyStat   `json:"latency"`
	Errors      []ErrorStat   `json:"errors"`
	Samples     []ErrorSample `json:"samples"`
	Series      []SeriesPoint `json:"series"`
}

type errEntry struct {
	kind   string
	detail string
	count  int64
}

// Stats 是并发安全的指标收集器。
type Stats struct {
	start  time.Time
	frozen atomic.Int64 // 冻结时刻（unix 纳秒），0 表示未冻结
	secs   [MaxSeconds]secRow

	initOk    atomic.Int64
	initFail  atomic.Int64
	replyHits atomic.Int64

	wsConnects    atomic.Int64
	wsDisconnects atomic.Int64
	wsLive        atomic.Int64
	wsEvents      atomic.Int64

	histCounts []atomic.Int64
	histSumUs  atomic.Int64
	histCount  atomic.Int64
	histMaxUs  atomic.Int64

	errMu     sync.Mutex
	errCounts map[string]*errEntry
	samples   []ErrorSample
}

// New 创建统计器。
func New(start time.Time) *Stats {
	return &Stats{
		start:      start,
		histCounts: make([]atomic.Int64, len(histBounds)+1),
		errCounts:  make(map[string]*errEntry, 16),
	}
}

// Freeze 在压测结束时冻结时间轴，避免结束后曲线继续拉长、耗时继续增长。
func (s *Stats) Freeze() {
	if s.frozen.Load() == 0 {
		s.frozen.Store(time.Now().UnixNano())
	}
}

// now 返回当前计算用的时间（冻结后固定为冻结时刻）。
func (s *Stats) now() time.Time {
	if f := s.frozen.Load(); f > 0 {
		return time.Unix(0, f)
	}
	return time.Now()
}

func (s *Stats) second() int {
	sec := int(s.now().Sub(s.start) / time.Second)
	if sec < 0 {
		sec = 0
	}
	if sec >= MaxSeconds {
		sec = MaxSeconds - 1
	}
	return sec
}

// StartRequest 记录一次请求「发起」，用于吞吐曲线（按发起时刻统计，
// 而不是完成时刻，否则高延迟时完成会扎堆，曲线会变成锯齿）。
func (s *Stats) StartRequest() {
	s.secs[s.second()].started.Add(1)
}

// Record 记录一次业务请求结果。lat 为请求耗时，status 为 HTTP 状态码。
func (s *Stats) Record(ok bool, kind, detail string, status int, lat time.Duration, vu int) {
	row := &s.secs[s.second()]
	if ok {
		row.ok.Add(1)
		if lat > 0 {
			row.okLatUs.Add(lat.Microseconds())
		}
		s.observeHist(lat)
		return
	}
	row.fail.Add(1)
	if kind == "" {
		kind = "unknown"
	}
	if detail == "" {
		detail = "无详细信息"
	}
	if len(detail) > 300 {
		detail = detail[:300] + "..."
	}
	s.recordError(vu, kind, detail, lat, status)
}

// RecordErrorOnly 只登记错误样本与分类，不参与成功率统计（用于初始化、长连接阶段）。
func (s *Stats) RecordErrorOnly(vu int, kind, detail string, lat time.Duration) {
	if len(detail) > 300 {
		detail = detail[:300] + "..."
	}
	s.recordError(vu, kind, detail, lat, 0)
}

func (s *Stats) recordError(vu int, kind, detail string, lat time.Duration, _ int) {
	key := kind + "|" + detail
	s.errMu.Lock()
	e, ok := s.errCounts[key]
	if !ok {
		e = &errEntry{kind: kind, detail: detail}
		s.errCounts[key] = e
	}
	e.count++
	s.samples = append(s.samples, ErrorSample{
		At:        time.Now().Format("15:04:05"),
		VU:        vu,
		Kind:      kind,
		Detail:    detail,
		LatencyMs: float64(lat.Microseconds()) / 1000.0,
	})
	if len(s.samples) > maxSamples {
		s.samples = s.samples[len(s.samples)-maxSamples:]
	}
	s.errMu.Unlock()
}

func (s *Stats) observeHist(lat time.Duration) {
	us := lat.Microseconds()
	if us < 0 {
		us = 0
	}
	i := sort.SearchFloat64s(histBounds, float64(us)/1000.0)
	if i >= len(s.histCounts) {
		i = len(s.histCounts) - 1
	}
	s.histCounts[i].Add(1)
	s.histCount.Add(1)
	s.histSumUs.Add(us)
	for {
		cur := s.histMaxUs.Load()
		if us <= cur || s.histMaxUs.CompareAndSwap(cur, us) {
			return
		}
	}
}

// RecordInit 记录 /api/guest/init 结果。
func (s *Stats) RecordInit(ok bool) {
	if ok {
		s.initOk.Add(1)
		return
	}
	s.initFail.Add(1)
}

// RecordReply 记录一次命中机器人自动回复。
func (s *Stats) RecordReply() { s.replyHits.Add(1) }

// WsConnect / WsDisconnect / WsEvent 记录 WebSocket 通道状态。
func (s *Stats) WsConnect() {
	s.wsConnects.Add(1)
	s.wsLive.Add(1)
}

func (s *Stats) WsDisconnect() {
	s.wsDisconnects.Add(1)
	if s.wsLive.Add(-1) < 0 {
		s.wsLive.Store(0)
	}
}

// WsClose 记录由压测端主动关闭（不计入断线统计，只更新在线数）。
func (s *Stats) WsClose() {
	if s.wsLive.Add(-1) < 0 {
		s.wsLive.Store(0)
	}
}

func (s *Stats) WsEvent() { s.wsEvents.Add(1) }

// SnapshotCompact 返回不含每秒曲线的快照（用于分接口指标，减小传输体积）。
func (s *Stats) SnapshotCompact() Summary {
	sum := s.Snapshot()
	sum.Series = nil
	return sum
}

// Quantile 从直方图计算分位延迟（毫秒）。
func (s *Stats) quantile(q float64) float64 {
	total := s.histCount.Load()
	if total == 0 {
		return 0
	}
	target := q * float64(total)
	var cum int64
	lo := 0.0
	for i := range s.histCounts {
		c := s.histCounts[i].Load()
		if c == 0 {
			if i < len(histBounds) {
				lo = histBounds[i]
			}
			continue
		}
		cum += c
		hi := s.histBounds(i)
		if float64(cum) >= target {
			frac := (target - float64(cum-c)) / float64(c)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			return lo + (hi-lo)*frac
		}
		lo = hi
	}
	return float64(s.histMaxUs.Load()) / 1000.0
}

func (s *Stats) histBounds(i int) float64 {
	if i < len(histBounds) {
		return histBounds[i]
	}
	return float64(s.histMaxUs.Load()) / 1000.0
}

// Snapshot 返回当前指标快照。
func (s *Stats) Snapshot() Summary {
	elapsed := s.now().Sub(s.start)
	if elapsed < 0 {
		elapsed = 0
	}
	full := int(elapsed / time.Second)

	sum := Summary{ElapsedSec: elapsed.Seconds()}

	series := make([]SeriesPoint, 0, full+1)
	var peak float64
	limit := full
	if limit > MaxSeconds-1 {
		limit = MaxSeconds - 1
	}
	for i := 0; i <= limit; i++ {
		row := &s.secs[i]
		started := row.started.Load()
		ok := row.ok.Load()
		fail := row.fail.Load()
		sum.Sent += started
		sum.Ok += ok
		sum.Failed += fail
		p := SeriesPoint{Second: i, Sent: started, Ok: ok, Fail: fail}
		if ok > 0 {
			p.AvgMs = float64(row.okLatUs.Load()) / float64(ok) / 1000.0
		}
		// 只把「已经走完的整秒」放进曲线：
		// 正在进行的这一秒数据还没攒满，画上去会让曲线尾部无故往下掉。
		if i < full {
			if float64(started) > peak {
				peak = float64(started)
			}
			series = append(series, p)
		}
	}
	sum.Series = series
	sum.PeakRPS = peak

	if done := sum.Ok + sum.Failed; done > 0 {
		sum.FailRate = float64(sum.Failed) / float64(done)
	}
	// 已发起但压测结束时还没回来的请求（被停止/时长到点打断），单独展示，避免看着像丢数据
	if inflight := sum.Sent - sum.Ok - sum.Failed; inflight > 0 {
		sum.Inflight = inflight
	}
	if sec := elapsed.Seconds(); sec > 0.5 {
		sum.AvgRPS = float64(sum.Sent) / sec
	}

	// 最近 3 个完整秒的发起来计算当前 RPS
	var recent int64
	var recentSec int
	for i := full - 1; i >= 0 && recentSec < 3; i-- {
		if i >= MaxSeconds {
			continue
		}
		recent += s.secs[i].started.Load()
		recentSec++
	}
	if recentSec > 0 {
		sum.CurrentRPS = float64(recent) / float64(recentSec)
	}

	if n := s.histCount.Load(); n > 0 {
		sum.Latency = LatencyStat{
			Avg: float64(s.histSumUs.Load()) / float64(n) / 1000.0,
			P50: s.quantile(0.50),
			P90: s.quantile(0.90),
			P95: s.quantile(0.95),
			P99: s.quantile(0.99),
			Max: float64(s.histMaxUs.Load()) / 1000.0,
		}
	}

	sum.ReplyHits = s.replyHits.Load()
	sum.InitOk = s.initOk.Load()
	sum.InitFailed = s.initFail.Load()
	sum.WsConnects = s.wsConnects.Load()
	sum.WsDisconnects = s.wsDisconnects.Load()
	sum.WsLive = s.wsLive.Load()
	sum.WsEvents = s.wsEvents.Load()

	s.errMu.Lock()
	errs := make([]ErrorStat, 0, len(s.errCounts))
	for _, e := range s.errCounts {
		errs = append(errs, ErrorStat{Kind: e.kind, Detail: e.detail, Count: e.count})
	}
	samples := make([]ErrorSample, len(s.samples))
	copy(samples, s.samples)
	s.errMu.Unlock()

	sort.Slice(errs, func(i, j int) bool { return errs[i].Count > errs[j].Count })
	if len(errs) > 50 {
		errs = errs[:50]
	}
	// 最新的错误样本排在最前
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
	if len(samples) > 50 {
		samples = samples[:50]
	}
	sum.Errors = errs
	sum.Samples = samples
	return sum
}
