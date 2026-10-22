// Package config 定义压测参数、默认值与校验规则。
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// UID 生成策略
const (
	UIDModePerUser = "per-user" // 每个并发用户一个独立客户（默认，最贴近真实场景）
	UIDModeSingle  = "single"   // 所有并发共用一个客户（压单会话）
	UIDModeRandom  = "random"   // 每次启动随机生成客户 ID
)

// 场景准备步骤
const (
	SetupNone      = "none"       // 直接发请求
	SetupGuestInit = "guest_init" // 先调一次访客初始化（客服系统需要先建会话）
)

// 鉴权方式
const (
	AuthNone   = "none"
	AuthBearer = "bearer"
)

// 风险等级（AI/规则给的建议，仅用于提示）
const (
	RiskRead    = "只读"
	RiskWrite   = "写入"
	RiskDanger  = "危险"
	RiskUnknown = "未知"
)

// Scenario 是一个可以勾选压测的接口场景。
type Scenario struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Method  string `json:"method"`
	Path    string `json:"path"` // 支持 {{uid}} {{channel}} {{seq}} {{ts}} {{rand6}} 占位符
	Body    string `json:"body"` // 请求体模板，GET 可留空
	Setup   string `json:"setup"`
	Auth    string `json:"auth"`
	Headers map[string]string `json:"headers,omitempty"`

	Concurrency     int `json:"concurrency"`
	IntervalMs      int `json:"interval_ms"`
	JitterMs        int `json:"jitter_ms"`
	PerUserMessages int `json:"per_user_messages"` // 0 = 只受时长限制

	Risk string `json:"risk"`
	Note string `json:"note"`
}

// Config 是一次压测任务的完整参数。
type Config struct {
	// 目标
	Target  string `json:"target"`  // 例如 http://127.0.0.1:8480
	Channel string `json:"channel"` // 访客渠道标识，如 web / wechat
	Token   string `json:"token"`   // 需要登录取数据的接口用（Bearer）

	// 全局节奏
	DurationSec int  `json:"duration_sec"` // 0 表示按条数跑
	TotalMessages int64 `json:"total_messages"`
	RampUpMs    int  `json:"ramp_up_ms"`
	TimeoutMs   int  `json:"timeout_ms"`
	KeepAlive   bool `json:"keep_alive"`

	// 客户模拟
	UIDMode        string `json:"uid_mode"`
	UIDPrefix      string `json:"uid_prefix"`
	UIDStart       int    `json:"uid_start"`
	NicknamePrefix string `json:"nickname_prefix"`
	UseWebSocket   bool   `json:"use_web_socket"`

	// 要压的接口（用户勾选后的清单）
	Scenarios []Scenario `json:"scenarios"`

	// 兼容旧版单接口模式的字段（scenarios 为空时用它合成一个场景）
	Concurrency     int      `json:"concurrency"`
	PerUserMessages int      `json:"per_user_messages"`
	IntervalMs      int      `json:"interval_ms"`
	JitterMs        int      `json:"jitter_ms"`
	Messages        []string `json:"messages"`
	MessageMode     string   `json:"message_mode"`
	MessageType     string   `json:"message_type"`
	SkipInit        bool     `json:"skip_init"`
}

// Default 返回一套可直接使用的默认参数（对准本机客服系统）。
func Default() Config {
	return Config{
		Target:          "http://127.0.0.1:8480",
		Channel:         "web",
		DurationSec:     10,
		TimeoutMs:       10000,
		KeepAlive:       true,
		UIDMode:         UIDModePerUser,
		UIDPrefix:       "lt_",
		UIDStart:        1,
		NicknamePrefix:  "压测用户",
		Concurrency:     20,
		IntervalMs:      1000,
		JitterMs:        200,
		MessageMode:     "roundrobin",
		MessageType:     "text",
		Messages:        []string{"压测消息 {{seq}}"},
	}
}

// 常见的「前端页面路径段」：命中就说明填的是页面地址，接口基址要退回到 host。
var pageSegments = map[string]bool{
	"kefu": true, "chat": true, "admin": true, "index": true,
	"c": true, "workbench": true, "login": true, "home": true, "web": true,
}

// SplitTarget 把你填的地址拆成「接口基址」和「页面地址」。
//
// 允许用户直接粘前端页面地址，例如：
//
//	http://127.0.0.1:8480/kefu/?channel=kefu_1edcf1a9&uid=20261022094940#/
//	→ base = http://127.0.0.1:8480 , page = http://127.0.0.1:8480/kefu
//
// 也保留接口路径前缀的写法：
//
//	https://api.example.com/api/v1 → base = https://api.example.com/api/v1
func SplitTarget(raw string) (base, pageURL string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" {
		return strings.TrimRight(raw, "/"), ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	isPage := false
	if path != "" {
		lower := strings.ToLower(path)
		last := lower
		if i := strings.LastIndex(lower, "/"); i >= 0 {
			last = lower[i+1:]
		}
		switch {
		case strings.HasSuffix(last, ".html"), strings.HasSuffix(last, ".htm"), strings.HasSuffix(last, ".php"):
			isPage = true
		case pageSegments[last]:
			isPage = true
		}
	}
	origin := u.Scheme + "://" + u.Host
	if isPage || path == "" {
		page := origin + path
		if page == origin {
			page = origin + "/"
		}
		return origin, page
	}
	return origin + path, origin + "/"
}

// DefaultScenario 返回一个场景的默认值。
func DefaultScenario() Scenario {
	return Scenario{
		Method:      "POST",
		Setup:       SetupNone,
		Auth:        AuthNone,
		Concurrency: 10,
		IntervalMs:  500,
		JitterMs:    100,
		Risk:        RiskUnknown,
	}
}

// Normalize 补齐默认值并去掉首尾空白。
func (c *Config) Normalize() {
	d := Default()
	// 允许填前端页面地址，这里统一归一化成接口基址
	base, _ := SplitTarget(c.Target)
	c.Target = base
	c.Channel = strings.TrimSpace(c.Channel)
	c.Token = strings.TrimSpace(c.Token)
	c.UIDMode = strings.TrimSpace(c.UIDMode)
	c.UIDPrefix = strings.TrimSpace(c.UIDPrefix)
	c.NicknamePrefix = strings.TrimSpace(c.NicknamePrefix)
	c.MessageMode = strings.TrimSpace(c.MessageMode)
	c.MessageType = strings.TrimSpace(c.MessageType)

	if c.Target == "" {
		c.Target = d.Target
	}
	if c.Channel == "" {
		c.Channel = d.Channel
	}
	if c.UIDMode == "" {
		c.UIDMode = d.UIDMode
	}
	if c.UIDPrefix == "" {
		c.UIDPrefix = d.UIDPrefix
	}
	if c.NicknamePrefix == "" {
		c.NicknamePrefix = d.NicknamePrefix
	}
	if c.MessageMode == "" {
		c.MessageMode = d.MessageMode
	}
	if c.MessageType == "" {
		c.MessageType = d.MessageType
	}
	if c.TimeoutMs <= 0 {
		c.TimeoutMs = d.TimeoutMs
	}
	if c.UIDStart <= 0 {
		c.UIDStart = 1
	}
	if c.Concurrency <= 0 {
		c.Concurrency = d.Concurrency
	}
	if c.IntervalMs < 0 {
		c.IntervalMs = 0
	}
	if c.JitterMs < 0 {
		c.JitterMs = 0
	}
	if c.RampUpMs < 0 {
		c.RampUpMs = 0
	}
	if c.PerUserMessages < 0 {
		c.PerUserMessages = 0
	}
	if c.TotalMessages < 0 {
		c.TotalMessages = 0
	}
	if c.DurationSec < 0 {
		c.DurationSec = 0
	}

	cleaned := make([]string, 0, len(c.Messages))
	for _, m := range c.Messages {
		if m = strings.TrimSpace(m); m != "" {
			cleaned = append(cleaned, m)
		}
	}
	if len(cleaned) == 0 {
		cleaned = d.Messages
	}
	c.Messages = cleaned

	// 旧版单接口模式 → 合成一个场景
	if len(c.Scenarios) == 0 {
		body := `{"type":"text","content":"` + strings.ReplaceAll(cleaned[0], `"`, `'`) + `"}`
		setup := SetupGuestInit
		if c.SkipInit {
			setup = SetupNone
		}
		c.Scenarios = []Scenario{{
			ID:          "sc_guest_message",
			Name:        "访客发消息",
			Method:      "POST",
			Path:        "/api/guest/message?channel={{channel}}&uid={{uid}}",
			Body:        body,
			Setup:       setup,
			Auth:        AuthNone,
			Concurrency: c.Concurrency,
			IntervalMs:  c.IntervalMs,
			JitterMs:    c.JitterMs,
			PerUserMessages: c.PerUserMessages,
			Risk:        RiskWrite,
			Note:        "旧版单接口模式自动生成",
		}}
	}

	def := DefaultScenario()
	for i := range c.Scenarios {
		s := &c.Scenarios[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Method = strings.ToUpper(strings.TrimSpace(s.Method))
		s.Path = strings.TrimSpace(s.Path)
		s.Body = strings.TrimSpace(s.Body)
		s.Setup = strings.TrimSpace(s.Setup)
		s.Auth = strings.TrimSpace(s.Auth)
		s.Risk = strings.TrimSpace(s.Risk)

		if s.ID == "" {
			s.ID = fmt.Sprintf("sc_%d", i+1)
		}
		if s.Method == "" {
			if s.Body != "" {
				s.Method = "POST"
			} else {
				s.Method = "GET"
			}
		}
		if s.Name == "" {
			s.Name = s.Method + " " + s.Path
		}
		if s.Setup == "" {
			s.Setup = SetupNone
		}
		if s.Auth == "" {
			s.Auth = AuthNone
		}
		if s.Risk == "" {
			s.Risk = RiskUnknown
		}
		if s.Concurrency <= 0 {
			s.Concurrency = def.Concurrency
		}
		if s.IntervalMs < 0 {
			s.IntervalMs = 0
		}
		if s.JitterMs < 0 {
			s.JitterMs = 0
		}
		if s.PerUserMessages < 0 {
			s.PerUserMessages = 0
		}
	}
}

// TotalConcurrency 返回所有场景的并发之和。
func (c Config) TotalConcurrency() int {
	n := 0
	for _, s := range c.Scenarios {
		n += s.Concurrency
	}
	return n
}

// Validate 校验参数，返回中文错误提示。
func (c Config) Validate() error {
	u, err := url.Parse(c.Target)
	if err != nil || u.Host == "" {
		return fmt.Errorf("目标地址不合法：%s（示例 http://127.0.0.1:8480）", c.Target)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("目标地址必须以 http:// 或 https:// 开头")
	}
	if len(c.Scenarios) == 0 {
		return fmt.Errorf("请先分析并勾选至少一个接口")
	}
	total := 0
	for i, s := range c.Scenarios {
		if s.Path == "" {
			return fmt.Errorf("第 %d 个接口缺少路径", i+1)
		}
		if !strings.HasPrefix(s.Path, "/") && !strings.HasPrefix(s.Path, "http") {
			return fmt.Errorf("接口「%s」的路径需以 / 开头", s.Name)
		}
		switch s.Method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			return fmt.Errorf("接口「%s」的请求方法不合法：%s", s.Name, s.Method)
		}
		if s.Concurrency < 0 {
			return fmt.Errorf("接口「%s」的并发不能为负", s.Name)
		}
		if s.Auth == AuthBearer && c.Token == "" {
			return fmt.Errorf("接口「%s」需要登录 token，请在页面上填写 Token 或取消勾选它", s.Name)
		}
		total += s.Concurrency
	}
	if total < 1 {
		return fmt.Errorf("请至少给一个接口设置 ≥1 的并发")
	}
	if total > 20000 {
		return fmt.Errorf("总并发 %d 过大（上限 20000）", total)
	}
	if c.DurationSec == 0 && c.TotalMessages == 0 {
		for _, s := range c.Scenarios {
			if s.PerUserMessages == 0 {
				return fmt.Errorf("请设置运行时长，或给每个接口设置「每并发条数」，否则会无限发送")
			}
		}
	}
	switch c.UIDMode {
	case UIDModePerUser, UIDModeSingle, UIDModeRandom:
	default:
		return fmt.Errorf("客户 ID 策略不合法：%s", c.UIDMode)
	}
	return nil
}
