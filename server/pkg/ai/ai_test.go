package ai

// pkg/ai 的单元测试：用 httptest 起一个假的 OpenAI 兼容服务，
// 覆盖「正常回复 / 鉴权头 / 401 / 空 choices / 解析失败 / Ping / 配置校验」。
//
// 这组测试不依赖外部网络与真实 API Key，是 AI 调用链的第一道验收。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOpenAI 起一个假的 /v1/chat/completions 服务，把收到的请求体交回测试用例。
func fakeOpenAI(t *testing.T, handler func(w http.ResponseWriter, body []byte, auth string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("请求路径 = %q，期望 /v1/chat/completions", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		handler(w, body, r.Header.Get("Authorization"))
	}))
}

func TestNewClientValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "缺 base_url", cfg: Config{APIKey: "k", Model: "m"}, want: "ai.base_url"},
		{name: "base_url 没有协议头", cfg: Config{BaseURL: "api.deepseek.com/v1", APIKey: "k", Model: "m"}, want: "http://"},
		{name: "缺 api_key", cfg: Config{BaseURL: "http://x/v1", Model: "m"}, want: "ai.api_key"},
		{name: "缺 model", cfg: Config{BaseURL: "http://x/v1", APIKey: "k"}, want: "ai.model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.cfg)
			if err == nil {
				t.Fatalf("期望报错，实际成功")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("错误信息 %q 未包含 %q", err.Error(), tt.want)
			}
		})
	}
}

func TestClientConfigNormalize(t *testing.T) {
	cfg := Config{BaseURL: "  https://api.deepseek.com/v1///  ", APIKey: " sk-1 ", Model: " deepseek-chat ", TimeoutSeconds: 0}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	got := c.Config()
	if got.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.APIKey != "sk-1" || got.Model != "deepseek-chat" {
		t.Errorf("密钥/模型未去空白: %q / %q", got.APIKey, got.Model)
	}
	if got.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("超时未兜底: %d", got.TimeoutSeconds)
	}
}

func TestChatSuccess(t *testing.T) {
	var gotBody []byte
	srv := fakeOpenAI(t, func(w http.ResponseWriter, body []byte, auth string) {
		gotBody = body
		if auth != "Bearer sk-test" {
			t.Errorf("鉴权头 = %q，期望 Bearer sk-test", auth)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  这是模型回复  "}}]}`))
	})
	defer srv.Close()

	c, err := NewClient(Config{
		BaseURL: srv.URL + "/v1", APIKey: "sk-test", Model: "deepseek-chat",
		SystemPrompt: "只依据知识库回答", TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	reply, err := c.Chat(context.Background(), "退货怎么弄")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if reply != "这是模型回复" {
		t.Errorf("回复 = %q，期望去掉首尾空白后的模型文本", reply)
	}

	var req chatRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v (%s)", err, string(gotBody))
	}
	if req.Model != "deepseek-chat" {
		t.Errorf("请求体 model = %q", req.Model)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("messages 结构不对: %+v", req.Messages)
	}
	if req.Messages[1].Content != "退货怎么弄" {
		t.Errorf("用户消息 = %q", req.Messages[1].Content)
	}
}

func TestChatNon200(t *testing.T) {
	srv := fakeOpenAI(t, func(w http.ResponseWriter, _ []byte, _ string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})
	defer srv.Close()

	c, _ := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	_, err := c.Chat(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("期望 401 错误，实际: %v", err)
	}
}

func TestChatEmptyReply(t *testing.T) {
	srv := fakeOpenAI(t, func(w http.ResponseWriter, _ []byte, _ string) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
	})
	defer srv.Close()

	c, _ := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	if _, err := c.Chat(context.Background(), "hi"); err == nil {
		t.Fatal("空回复应返回错误（业务层据此降级）")
	}
}

func TestChatNoChoices(t *testing.T) {
	srv := fakeOpenAI(t, func(w http.ResponseWriter, _ []byte, _ string) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	})
	defer srv.Close()

	c, _ := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	if _, err := c.Chat(context.Background(), "hi"); err == nil {
		t.Fatal("缺 choices 应返回错误")
	}
}

func TestChatInvalidJSON(t *testing.T) {
	srv := fakeOpenAI(t, func(w http.ResponseWriter, _ []byte, _ string) {
		_, _ = w.Write([]byte(`<html>502 Bad Gateway</html>`))
	})
	defer srv.Close()

	c, _ := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	if _, err := c.Chat(context.Background(), "hi"); err == nil {
		t.Fatal("非 JSON 响应应返回错误")
	}
}

func TestPing(t *testing.T) {
	var gotBody []byte
	srv := fakeOpenAI(t, func(w http.ResponseWriter, body []byte, _ string) {
		gotBody = body
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	})
	defer srv.Close()

	c, err := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping 失败: %v", err)
	}
	var req chatRequest
	_ = json.Unmarshal(gotBody, &req)
	if req.MaxTokens != pingMaxTokens {
		t.Errorf("探测请求 max_tokens = %d，期望 %d（越省越好）", req.MaxTokens, pingMaxTokens)
	}
}

func TestPingUnauthorized(t *testing.T) {
	srv := fakeOpenAI(t, func(w http.ResponseWriter, _ []byte, _ string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	defer srv.Close()

	c, _ := NewClient(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"})
	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "AI 接口不可用") {
		t.Fatalf("Ping 应返回「AI 接口不可用」错误，实际: %v", err)
	}
}
