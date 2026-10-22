package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"kefu-loadtest/internal/config"
)

// vu 是一个并发虚拟用户。
type vu struct {
	run  *Run
	sc   *scenario
	idx  int
	uid  string
	nick string
	seq  int64
}

func newVU(r *Run, sc *scenario, idx int) *vu {
	cfg := &r.cfg
	prefix := cfg.UIDPrefix + sc.spec.ID + "_"
	var uid string
	switch cfg.UIDMode {
	case config.UIDModeSingle:
		uid = cfg.UIDPrefix + "shared_single"
	case config.UIDModeRandom:
		uid = fmt.Sprintf("%s%08x", prefix, rand.Uint32())
	default:
		uid = fmt.Sprintf("%s%d", prefix, cfg.UIDStart+idx)
	}
	return &vu{
		run:  r,
		sc:   sc,
		idx:  idx,
		uid:  uid,
		nick: fmt.Sprintf("%s%d", cfg.NicknamePrefix, idx+1),
	}
}

// fail 记录一次失败的请求（计入成功率统计，并写一条日志）。
func (v *vu) fail(kind, detail string, status int, lat time.Duration) {
	v.sc.stats.Record(false, kind, detail, status, lat, v.idx)
	v.run.stats.Record(false, kind, detail, status, lat, v.idx)
	v.run.logError(v.idx, kind, "请求失败（"+v.sc.spec.Name+"）", detail)
}

// note 只登记异常（不计入成功率），例如初始化失败、长连接异常。
func (v *vu) note(kind, action, detail string, lat time.Duration) {
	v.sc.stats.RecordErrorOnly(v.idx, kind, detail, lat)
	v.run.stats.RecordErrorOnly(v.idx, kind, detail, lat)
	v.run.logError(v.idx, kind, action, detail)
}

func (v *vu) loop(ctx context.Context) {
	vctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if v.sc.spec.Setup == config.SetupGuestInit {
		v.doInit(vctx)
	}

	var wg sync.WaitGroup
	if v.run.cfg.UseWebSocket && v.sc.spec.Setup == config.SetupGuestInit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.wsLoop(vctx)
		}()
	}
	v.sendLoop(vctx)
	cancel()
	wg.Wait()
}

// doInit 模拟用户端页面加载时的初始化请求（客服系统里用来先建会话）。
func (v *vu) doInit(ctx context.Context) {
	cfg := &v.run.cfg
	u := fmt.Sprintf("%s/api/guest/init?channel=%s&uid=%s&nickname=%s",
		v.run.base, url.QueryEscape(cfg.Channel), url.QueryEscape(v.uid), url.QueryEscape(v.nick))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		v.note("build_request", "初始化失败", err.Error(), 0)
		v.countInit(false)
		return
	}
	req.Header.Set("User-Agent", "kefu-loadtest/1.0")
	start := time.Now()
	resp, err := v.run.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		kind, detail := classifyErr(err)
		v.note("init_"+kind, "初始化失败", detail, time.Since(start))
		v.countInit(false)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	if ok, kind, detail := Interpret(resp.StatusCode, body); !ok {
		v.note("init_"+kind, "初始化失败", detail, time.Since(start))
		v.countInit(false)
		return
	}
	v.countInit(true)
}

func (v *vu) countInit(ok bool) {
	v.sc.stats.RecordInit(ok)
	v.run.stats.RecordInit(ok)
}

func (v *vu) sendLoop(ctx context.Context) {
	spec := &v.sc.spec
	cfg := &v.run.cfg
	for {
		if ctx.Err() != nil {
			return
		}
		if spec.PerUserMessages > 0 && v.seq >= int64(spec.PerUserMessages) {
			return
		}
		if cfg.TotalMessages > 0 && v.run.issued.Add(1) > cfg.TotalMessages {
			return
		}
		v.seq++
		v.sendOnce(ctx)
		if !v.wait(ctx, spec) {
			return
		}
	}
}

func (v *vu) wait(ctx context.Context, spec *config.Scenario) bool {
	idle := func() bool {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	if spec.IntervalMs == 0 && spec.JitterMs == 0 {
		return idle()
	}
	d := time.Duration(spec.IntervalMs) * time.Millisecond
	if spec.JitterMs > 0 {
		d += time.Duration(rand.IntN(2*spec.JitterMs+1)-spec.JitterMs) * time.Millisecond
	}
	if d <= 0 {
		return idle()
	}
	return sleepCtx(ctx, d)
}

func (v *vu) sendOnce(ctx context.Context) {
	spec := &v.sc.spec
	cfg := &v.run.cfg

	v.sc.stats.StartRequest()
	v.run.stats.StartRequest()

	target := v.run.base + v.render(spec.Path)
	var bodyReader io.Reader
	if spec.Body != "" {
		bodyReader = bytes.NewReader([]byte(v.render(spec.Body)))
	}
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		v.fail("build_request", err.Error(), 0, 0)
		return
	}
	if spec.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "kefu-loadtest/1.0")
	for k, val := range spec.Headers {
		req.Header.Set(k, v.render(val))
	}
	if spec.Auth == config.AuthBearer && cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	start := time.Now()
	resp, err := v.run.client.Do(req)
	if err != nil {
		if ctx.Err() != nil { // 主动停止造成的中断不计入失败
			return
		}
		kind, detail := classifyErr(err)
		v.fail(kind, detail, 0, time.Since(start))
		return
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	lat := time.Since(start)
	if readErr != nil {
		if ctx.Err() != nil {
			return
		}
		v.fail("read_body", readErr.Error(), resp.StatusCode, lat)
		return
	}
	ok, kind, detail := Interpret(resp.StatusCode, body)
	if !ok {
		v.fail(kind, detail, resp.StatusCode, lat)
		return
	}
	v.sc.stats.Record(true, "", "", resp.StatusCode, lat, v.idx)
	v.run.stats.Record(true, "", "", resp.StatusCode, lat, v.idx)
}

// render 替换路径/请求体里的占位符。
func (v *vu) render(tpl string) string {
	if !strings.Contains(tpl, "{{") {
		return tpl
	}
	return strings.NewReplacer(
		"{{uid}}", v.uid,
		"{{nick}}", v.nick,
		"{{channel}}", v.run.cfg.Channel,
		"{{seq}}", strconv.FormatInt(v.seq, 10),
		"{{vu}}", strconv.Itoa(v.idx+1),
		"{{ts}}", strconv.FormatInt(time.Now().UnixMilli(), 10),
		"{{rand6}}", fmt.Sprintf("%06d", rand.IntN(1000000)),
		"{{rand}}", strconv.Itoa(rand.IntN(1000000)),
	).Replace(tpl)
}

// Interpret 统一判定一次响应是否成功：
//   - 2xx 算成功；
//   - 若响应体是 JSON 且带 code 字段而 code != 0，算业务失败；
//   - 3xx/4xx/5xx 都算失败（压测场景下跳转/参数错都说明用例不对）。
func Interpret(status int, body []byte) (bool, string, string) {
	if status >= 300 {
		return false, fmt.Sprintf("http_%d", status), snippet(body)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return true, "", ""
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var env struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(trimmed, &env); err == nil && env.Code != nil && *env.Code != 0 {
			return false, "biz_error", fmt.Sprintf("code=%d %s", *env.Code, env.Message)
		}
	}
	return true, "", ""
}

// wsLoop 维持访客 WebSocket 长连接，统计连接成功率、断线次数与收到的事件数。
func (v *vu) wsLoop(ctx context.Context) {
	cfg := &v.run.cfg
	wsURL := toWS(v.run.base) + "/ws/guest?channel=" + url.QueryEscape(cfg.Channel) +
		"&uid=" + url.QueryEscape(v.uid) + "&since_id=0"
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		conn, _, err := dialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			kind, detail := classifyErr(err)
			v.note("ws_"+kind, "长连接失败", detail, 0)
			if !sleepCtx(ctx, backoff) {
				return
			}
			if backoff < 15*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		v.run.stats.WsConnect()

		stop := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-stop:
			}
		}()
		v.readWS(conn)
		close(stop)
		conn.Close()

		if ctx.Err() != nil {
			v.run.stats.WsClose()
			return
		}
		v.run.stats.WsDisconnect()
		v.run.logWarn("ws_closed", "并发#%d WebSocket 断线，0.5 秒后重连", v.idx+1)

		if !sleepCtx(ctx, 500*time.Millisecond) {
			return
		}
	}
}

func (v *vu) readWS(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		v.run.stats.WsEvent()
	}
}

func classifyErr(err error) (string, string) {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout", "请求超时：" + trimErr(err)
	}
	msg := trimErr(err)
	low := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled", msg
	case strings.Contains(low, "connection refused"), strings.Contains(low, "connectex"):
		return "conn_refused", msg
	case strings.Contains(low, "no such host"):
		return "dns_error", msg
	case strings.Contains(low, "connection reset"), strings.Contains(low, "reset by peer"):
		return "conn_reset", msg
	case strings.Contains(low, "eof"):
		return "conn_closed", msg
	case strings.Contains(low, "too many open files"), strings.Contains(low, "insufficient buffer"):
		return "resource_exhausted", msg
	case strings.Contains(low, "wsarecv"), strings.Contains(low, "wsaeWOULDBLOCK"):
		return "socket_error", msg
	case strings.Contains(low, "dial tcp"), strings.Contains(low, "dial "):
		return "dial_error", msg
	default:
		return "network", msg
	}
}

// urlPrefixRe 去掉错误信息里的完整 URL 前缀，让同一类错误能聚合到一起。
var urlPrefixRe = regexp.MustCompile(`^(?:Get|Post|Put|Patch|Head|Delete) ".*?":\s*`)

func trimErr(err error) string {
	s := urlPrefixRe.ReplaceAllString(err.Error(), "")
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "响应体为空"
	}
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
}

func toWS(base string) string {
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://")
	}
	return "ws://" + strings.TrimPrefix(base, "http://")
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
