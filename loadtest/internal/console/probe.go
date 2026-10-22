package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/engine"
)

// ProbeResult 是一次连通性自检的结果。
type ProbeResult struct {
	Target     string  `json:"target"`
	Channel    string  `json:"channel"`
	UID        string  `json:"uid"`
	InitOK     bool    `json:"init_ok"`
	InitMs     float64 `json:"init_ms"`
	InitDetail string  `json:"init_detail"`
	SendOK     bool    `json:"send_ok"`
	SendMs     float64 `json:"send_ms"`
	SendDetail string  `json:"send_detail"`
	MessageID  int64   `json:"message_id"`
	HasReply   bool    `json:"has_reply"`
	Pass       bool    `json:"pass"`
	Verdict    string  `json:"verdict"`
}

// OneResult 是「测这一个接口」的结果。
type OneResult struct {
	OK       bool    `json:"ok"`
	Status   int     `json:"status"`
	MS       float64 `json:"ms"`
	Kind     string  `json:"kind"`
	Detail   string  `json:"detail"`
	Body     string  `json:"body"`
	URL      string  `json:"url"`
	SetupErr string  `json:"setup_error,omitempty"`
	Verdict  string  `json:"verdict"`
}

// Probe 向目标发一次初始化 + 一次发消息，用于压测前确认链路可用。
func Probe(target, channel, uid string, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	target = strings.TrimRight(strings.TrimSpace(target), "/")
	if uid == "" {
		uid = fmt.Sprintf("probe_%d", time.Now().Unix()%100000)
	}
	if channel == "" {
		channel = "web"
	}
	res := ProbeResult{Target: target, Channel: channel, UID: uid}

	client := &http.Client{Timeout: timeout}
	base := fmt.Sprintf("%s/api/guest/init?channel=%s&uid=%s&nickname=%s",
		target, url.QueryEscape(channel), url.QueryEscape(uid), url.QueryEscape("连通性自检"))

	start := time.Now()
	resp, err := client.Get(base)
	res.InitMs = ms(time.Since(start))
	if err != nil {
		res.InitDetail = err.Error()
		res.Verdict = "初始化请求发不出去：" + hintForErr(err)
		return res
	}
	initBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		res.InitDetail = fmt.Sprintf("HTTP %d %s", resp.StatusCode, snippetStr(initBody))
		res.Verdict = "初始化接口返回非 200，检查目标地址是否指向客服系统后端"
		return res
	}
	var initResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Conversation struct {
				ID int64 `json:"id"`
			} `json:"conversation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(initBody, &initResp); err != nil {
		res.InitDetail = "响应不是合法 JSON：" + snippetStr(initBody)
		res.Verdict = "目标地址可能不是客服系统后端"
		return res
	}
	if initResp.Code != 0 {
		res.InitDetail = fmt.Sprintf("code=%d %s", initResp.Code, initResp.Message)
		res.Verdict = "初始化被拒绝：" + initResp.Message + "（渠道可能不存在或未启用）"
		return res
	}
	res.InitOK = true
	res.InitDetail = fmt.Sprintf("会话 ID=%d", initResp.Data.Conversation.ID)

	payload, _ := json.Marshal(map[string]string{
		"type":    "text",
		"content": "连通性自检消息 " + time.Now().Format("15:04:05"),
	})
	sendURL := fmt.Sprintf("%s/api/guest/message?channel=%s&uid=%s",
		target, url.QueryEscape(channel), url.QueryEscape(uid))
	start = time.Now()
	resp2, err := client.Post(sendURL, "application/json", bytes.NewReader(payload))
	res.SendMs = ms(time.Since(start))
	if err != nil {
		res.SendDetail = err.Error()
		res.Verdict = "初始化正常但发消息失败：" + hintForErr(err)
		return res
	}
	sendBody, _ := io.ReadAll(io.LimitReader(resp2.Body, 1<<16))
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		res.SendDetail = fmt.Sprintf("HTTP %d %s", resp2.StatusCode, snippetStr(sendBody))
		res.Verdict = "发消息接口返回非 200"
		return res
	}
	var sendResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Message struct {
				ID int64 `json:"id"`
			} `json:"message"`
			Reply json.RawMessage `json:"reply"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sendBody, &sendResp); err != nil {
		res.SendDetail = "响应不是合法 JSON：" + snippetStr(sendBody)
		res.Verdict = "发消息响应无法解析"
		return res
	}
	if sendResp.Code != 0 {
		res.SendDetail = fmt.Sprintf("code=%d %s", sendResp.Code, sendResp.Message)
		res.Verdict = "发消息被拒绝：" + sendResp.Message
		return res
	}
	res.SendOK = true
	res.MessageID = sendResp.Data.Message.ID
	reply := bytes.TrimSpace(sendResp.Data.Reply)
	res.HasReply = len(reply) > 0 && string(reply) != "null"
	res.SendDetail = fmt.Sprintf("消息 ID=%d", res.MessageID)
	res.Pass = true
	res.Verdict = fmt.Sprintf("连通正常：初始化 %.0fms，发消息 %.0fms", res.InitMs, res.SendMs)
	return res
}

// ProbeOne 试跑一个接口场景（勾选后先验证单个接口能不能跑通）。
func ProbeOne(ctx context.Context, target, channel, uid, token string, spec config.Scenario, timeout time.Duration) OneResult {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	target = strings.TrimRight(strings.TrimSpace(target), "/")
	if channel == "" {
		channel = "web"
	}
	if uid == "" {
		uid = fmt.Sprintf("probe_%d", time.Now().Unix()%100000)
	}
	client := &http.Client{Timeout: timeout}
	res := OneResult{}

	if spec.Setup == config.SetupGuestInit {
		initURL := fmt.Sprintf("%s/api/guest/init?channel=%s&uid=%s&nickname=%s",
			target, url.QueryEscape(channel), url.QueryEscape(uid), url.QueryEscape("试跑探测"))
		start := time.Now()
		resp, err := client.Get(initURL)
		lat := ms(time.Since(start))
		if err != nil {
			res.Verdict = fmt.Sprintf("准备会话失败（%.0fms）：%s", lat, hintForErr(err))
			res.SetupErr = err.Error()
			return res
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if ok, kind, detail := engine.Interpret(resp.StatusCode, body); !ok {
			res.SetupErr = fmt.Sprintf("[%s] %s", kind, detail)
			res.Verdict = "准备会话失败：" + detail
			return res
		}
	}

	path := renderTpl(spec.Path, uid, channel)
	bodyStr := renderTpl(spec.Body, uid, channel)
	targetURL := path
	if !strings.HasPrefix(path, "http") {
		targetURL = target + path
	}
	res.URL = targetURL

	var reader io.Reader
	if bodyStr != "" {
		reader = bytes.NewReader([]byte(bodyStr))
	}
	req, err := http.NewRequestWithContext(ctx, spec.Method, targetURL, reader)
	if err != nil {
		res.Verdict = "请求构造失败：" + err.Error()
		return res
	}
	if bodyStr != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "kefu-loadtest/1.0")
	for k, v := range spec.Headers {
		req.Header.Set(k, renderTpl(v, uid, channel))
	}
	if spec.Auth == config.AuthBearer && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	start := time.Now()
	resp, err := client.Do(req)
	res.MS = ms(time.Since(start))
	if err != nil {
		res.Kind = "network"
		res.Detail = err.Error()
		res.Verdict = "请求发不出去：" + hintForErr(err)
		return res
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	res.Status = resp.StatusCode
	res.Body = snippetStr(body)
	res.OK, res.Kind, res.Detail = engine.Interpret(resp.StatusCode, body)
	if res.OK {
		res.Verdict = fmt.Sprintf("通过：HTTP %d，%.0fms", resp.StatusCode, res.MS)
	} else {
		res.Verdict = fmt.Sprintf("未通过：HTTP %d［%s］%s", resp.StatusCode, res.Kind, res.Detail)
	}
	return res
}

func renderTpl(tpl, uid, channel string) string {
	if !strings.Contains(tpl, "{{") {
		return tpl
	}
	return strings.NewReplacer(
		"{{uid}}", uid,
		"{{nick}}", "试跑客户",
		"{{channel}}", channel,
		"{{seq}}", "1",
		"{{vu}}", "1",
		"{{ts}}", strconv.FormatInt(time.Now().UnixMilli(), 10),
		"{{rand6}}", "123456",
		"{{rand}}", "123456",
	).Replace(tpl)
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func snippetStr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "（空响应体）"
	}
	if len(s) > 400 {
		s = s[:400] + "..."
	}
	return s
}

func hintForErr(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "connectex"):
		return "目标端口没有服务在监听 —— 后端没启动，或端口/地址填错了"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "dns"):
		return "域名解析不了，检查地址拼写"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return "请求超时，检查网络连通性/防火墙"
	default:
		return "检查目标地址、端口与网络"
	}
}
