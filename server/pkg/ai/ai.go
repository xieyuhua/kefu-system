// Package ai 大模型能力层：用 OpenAI 兼容协议调用 /chat/completions。
//
// 分层约定：本包只认识「连接参数 + 一段提示词」，不读写数据库、不解析后台设置项、
// 不认识 internal 下的任何包（settings 表里的 ai.* 由 common/service 翻译成这里的 Config）。
// 业务判断（要不要调、检索、失败降级）全部在业务层，本包只做一次 HTTP 调用。
//
// 形态约定（与全仓一致）：包级只保留构造函数 NewClient 与构造函数式入口，
// 业务能力一律是 *Client 的指针接收者方法；连接与配置是实例字段。
//
// 兼容性要点：
//   - BaseURL 是「根地址」（如 https://api.deepseek.com/v1），本包自行拼 /chat/completions，
//     因此接入 DeepSeek / OpenAI / 通义 / 本地 vLLM / Ollama 的 OpenAI 兼容层都是同一套代码；
//   - 鉴权固定 Bearer（API Key 为空视为配置不完整，由 NewClient 直接报错）；
//   - 只取首条回复文本（choices[0].message.content），多轮/工具调用不在本层处理。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// chatCompletionsPath OpenAI 兼容的对话补全路径（相对 BaseURL）。
	chatCompletionsPath = "/chat/completions"
	// DefaultTimeoutSeconds 未指定超时时的默认值（与后台 ai.timeout_seconds 默认值一致）。
	DefaultTimeoutSeconds = 20
	// pingMaxTokens 探测请求的 max_tokens：只要能证明「地址 + 鉴权 + 模型」可用即可。
	// 不要设得过小：带 reasoning 的模型会先花掉若干 token，8 个 token 时常常 finish_reason=length 且内容为空。
	pingMaxTokens = 64
	// errBodyLimit 失败时读回的响应体上限：只用于错误提示，避免把整段 HTML 塞进日志。
	errBodyLimit = 512
)

// Config 大模型连接配置。字段与后台设置项（ai.*）一一对应。
type Config struct {
	// BaseURL OpenAI 兼容接口根地址，不含 /chat/completions。
	BaseURL string
	// APIKey 接口密钥（Bearer）。
	APIKey string
	// Model 模型名，如 deepseek-chat。
	Model string
	// SystemPrompt 系统提示词；为空时只发用户消息。
	SystemPrompt string
	// TimeoutSeconds 单次请求超时（秒）；<=0 时取默认 20 秒。
	TimeoutSeconds int
}

// Normalize 返回规范化后的配置副本：去首尾空白、BaseURL 去尾部斜杠、超时兜底。
func (c Config) Normalize() Config {
	out := c
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.Model = strings.TrimSpace(out.Model)
	out.SystemPrompt = strings.TrimSpace(out.SystemPrompt)
	if out.TimeoutSeconds <= 0 {
		out.TimeoutSeconds = DefaultTimeoutSeconds
	}
	return out
}

// Complete 报告连接参数是否齐全（BaseURL / APIKey / Model）。
func (c Config) Complete() bool {
	return c.Validate() == nil
}

// Validate 校验连接参数，错误信息直接面向后台使用者（用于「测试连接」提示）。
func (c Config) Validate() error {
	cfg := c.Normalize()
	switch {
	case cfg.BaseURL == "":
		return errors.New("未配置 AI 接口地址（ai.base_url）")
	case !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://"):
		return fmt.Errorf("AI 接口地址必须以 http:// 或 https:// 开头：%s", cfg.BaseURL)
	case cfg.APIKey == "":
		return errors.New("未配置 AI 接口密钥（ai.api_key）")
	case cfg.Model == "":
		return errors.New("未配置模型名（ai.model）")
	}
	return nil
}

// Client OpenAI 兼容协议的对话客户端，持有规范化后的配置与独立的 HTTP 客户端。
type Client struct {
	cfg Config
	hc  *http.Client
}

// NewClient 用给定配置创建客户端；配置不完整时直接报错（调用方据此降级）。
func NewClient(cfg Config) (*Client, error) {
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		cfg: cfg,
		hc: &http.Client{
			// 超时由配置决定：后台可调 20 秒这类同步等待的上限
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}, nil
}

// Config 返回客户端生效的配置（已规范化）。
func (c *Client) Config() Config { return c.cfg }

// ---------------- OpenAI 兼容请求 / 响应体 ----------------

// chatMessage 一条对话消息（role: system / user / assistant）。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest POST /chat/completions 的请求体（只带本项目用得到的字段）。
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// chatResponse 只解析首条回复文本，其余字段忽略（各服务商扩展字段不统一）。
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat 调 OpenAI 兼容的 /chat/completions，返回首条回复文本。
// 上层约定：返回 error 时调用方必须降级（不要让客户等不到回复）。
func (c *Client) Chat(ctx context.Context, userPrompt string) (string, error) {
	reply, finish, err := c.chat(ctx, strings.TrimSpace(userPrompt), 0, true)
	if err != nil {
		return "", err
	}
	if reply == "" {
		// 空回复按失败处理：业务层据此降级，避免给客户发一条空白消息
		return "", fmt.Errorf("响应内容为空（finish_reason=%s）", finish)
	}
	return reply, nil
}

// Ping 连通性探测：验证「地址 + 鉴权 + 模型名」是否可用。
//
// 两个容易误判的点：
//  1. 探测请求**不带系统提示词**（系统提示词可能很长，会挤占输出预算）；
//  2. 输出被 max_tokens 截断（finish_reason=length）时内容可能为空，但这已经证明
//     接口可访问、鉴权通过、模型存在，应判定为「连接成功」而不是失败。
func (c *Client) Ping(ctx context.Context) error {
	reply, finish, err := c.chat(ctx, "ping", pingMaxTokens, false)
	if err != nil {
		return fmt.Errorf("AI 接口不可用：%w", err)
	}
	if reply == "" && finish != "length" {
		return fmt.Errorf("AI 接口不可用：响应内容为空（finish_reason=%s）", finish)
	}
	return nil
}

// chat 发一次对话补全请求。maxTokens > 0 时限制输出长度；withSystem 控制是否附带系统提示词。
// 返回「回复文本 + finish_reason」；内容为空不算错误，由调用方按场景判定（Ping 与 Chat 口径不同）。
func (c *Client) chat(ctx context.Context, userPrompt string, maxTokens int, withSystem bool) (string, string, error) {
	if strings.TrimSpace(userPrompt) == "" {
		return "", "", errors.New("提示词不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	msgs := make([]chatMessage, 0, 2)
	if withSystem && c.cfg.SystemPrompt != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: c.cfg.SystemPrompt})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: userPrompt})

	body, err := json.Marshal(chatRequest{Model: c.cfg.Model, Messages: msgs, MaxTokens: maxTokens})
	if err != nil {
		return "", "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := c.cfg.BaseURL + chatCompletionsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		// 超时 / DNS / 连接失败：保留原始错误便于业务层打日志定位
		return "", "", fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("接口返回 %d：%s", resp.StatusCode, snippet(raw))
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("解析响应失败（响应体：%s）: %w", snippet(raw), err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", "", fmt.Errorf("接口返回错误：%s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", "", fmt.Errorf("响应缺少 choices（响应体：%s）", snippet(raw))
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), out.Choices[0].FinishReason, nil
}

// snippet 截断响应体，避免把整段 HTML 错误页写进日志；按字符切分，不切坏中文。
func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	r := []rune(s)
	// errBodyLimit 是「字节数」量级的上限，中文按 3 字节估算即可，只用于日志截断
	if len(r) <= errBodyLimit {
		return s
	}
	return string(r[:errBodyLimit]) + "…"
}
