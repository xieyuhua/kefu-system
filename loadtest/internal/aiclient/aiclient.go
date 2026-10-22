// Package aiclient 是对 OpenAI 兼容大模型接口的最小封装。
//
// 只要配置了 base_url + api_key + model，压测台的「接口勘察整理」和「结果分析整改」
// 就交给大模型来做；没配置或调用失败时，会自动退回内置规则引擎，保证离线也能用。
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config 是大模型接入配置。
type Config struct {
	BaseURL    string `json:"base_url"` // 例如 https://api.deepseek.com/v1
	APIKey     string `json:"api_key"`
	Model      string `json:"model"` // 例如 deepseek-chat / qwen-plus / gpt-4o-mini
	TimeoutSec int    `json:"timeout_sec"`
}

// Normalize 补齐默认值。
func (c *Config) Normalize() {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.Model = strings.TrimSpace(c.Model)
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 90
	}
	if c.TimeoutSec > 600 {
		c.TimeoutSec = 600
	}
}

// Ready 表示配置齐全，可以调用大模型。
func (c Config) Ready() bool {
	return c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

// MaskedKey 返回脱敏后的密钥，便于回显到页面。
func (c Config) MaskedKey() string {
	if c.APIKey == "" {
		return ""
	}
	if len(c.APIKey) <= 8 {
		return "********"
	}
	return c.APIKey[:4] + "****" + c.APIKey[len(c.APIKey)-4:]
}

// FromEnv 读取环境变量：KEFU_LT_AI_BASE_URL / KEFU_LT_AI_KEY / KEFU_LT_AI_MODEL / KEFU_LT_AI_TIMEOUT。
func FromEnv() Config {
	var c Config
	c.BaseURL = os.Getenv("KEFU_LT_AI_BASE_URL")
	c.APIKey = os.Getenv("KEFU_LT_AI_KEY")
	c.Model = os.Getenv("KEFU_LT_AI_MODEL")
	c.Normalize()
	return c
}

// Client 调用大模型。
type Client struct {
	cfg  Config
	http *http.Client
}

// New 创建客户端。
func New(cfg Config) *Client {
	cfg.Normalize()
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
		},
	}
}

// Ready 表示可以调用。
func (c *Client) Ready() bool { return c != nil && c.cfg.Ready() }

// Model 返回模型名。
func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.cfg.Model
}

// Config 返回当前配置。
func (c *Client) Config() Config {
	if c == nil {
		return Config{}
	}
	return c.cfg
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat 发一次对话请求，返回模型输出文本。
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	if !c.Ready() {
		return "", fmt.Errorf("未配置大模型（需要 base_url + api_key + model）")
	}
	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.2,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	endpoint := c.cfg.BaseURL
	if !strings.Contains(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用大模型失败：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("大模型返回 HTTP %d：%s", resp.StatusCode, snippet(body))
	}
	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("大模型响应无法解析：%s", snippet(body))
	}
	if cr.Error != nil && cr.Error.Message != "" {
		return "", fmt.Errorf("大模型报错：%s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("大模型没有返回内容：%s", snippet(body))
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// ExtractJSON 从模型输出里取出 JSON（容忍 ```json 代码块、前后废话）。
func ExtractJSON(text string, out any) error {
	cleaned := strings.TrimSpace(text)
	if i := strings.Index(cleaned, "```"); i >= 0 {
		rest := cleaned[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			cleaned = rest[:j]
		} else {
			cleaned = rest
		}
	}
	cleaned = strings.TrimSpace(cleaned)
	if err := json.Unmarshal([]byte(cleaned), out); err == nil {
		return nil
	}
	// 退一步：截取第一个 { 或 [ 到最后一个 } 或 ]
	for _, pair := range [][2]string{{"{", "}"}, {"[", "]"}} {
		start := strings.Index(cleaned, pair[0])
		end := strings.LastIndex(cleaned, pair[1])
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(cleaned[start:end+1]), out); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("模型输出里没有可解析的 JSON")
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
