package service

// 本文件是 AI 应答的业务层：把 settings 表里的 ai.* / rag.* 翻译成 pkg/ai 的客户端，
// 并在「未命中自动回复规则」时按「检索该渠道自动回复规则 → 大模型 → 失败降级」的顺序生成回复。
//
// 设置项契约（settings 表逐项存储，后台设置页可维护，默认全部关闭）：
//
//	ai.enabled          true / false（默认 false：不配置就保持原有「规则 + 兜底话术」行为）
//	ai.base_url         OpenAI 兼容根地址，如 https://api.deepseek.com/v1
//	ai.api_key          接口密钥（GET /api/settings 脱敏为 ******，回写 ****** 表示沿用原值）
//	ai.model            模型名，如 deepseek-chat
//	ai.system_prompt    系统提示词（默认要求「只依据自动回复内容回答，不确定就转人工」）
//	ai.timeout_seconds  请求超时（默认 20 秒）
//	rag.top_k           检索条数（默认 3）
//	rag.min_score       最低相关度 0~1（默认 0.15，低于该值视为无命中）
//
// 失败降级是本内核的硬约定：任何环节出错（配置不全 / 检索无命中 / 超时 / 401 / 5xx / 解析失败）
// 都只返回空串并打日志，由调用方回退到「机器人兜底话术」，绝不让客户收不到回复。
//
// 形态约定：能力都是 AIService 的方法（指针接收者），检索内核在构造时建好。

import (
	"context"
	"errors"
	"log"
	"strings"

	"kefu-server/pkg/ai"
	"kefu-server/pkg/setting"
)

const (
	// MaskedAPIKey 脱敏占位：与对象存储的 ****** 同一口径，后台回写该值表示沿用已存密钥。
	MaskedAPIKey = "******"
	// aiLogPromptClip 日志里问题 / 提示词的截断长度：够定位问题，又不至于把日志刷爆。
	aiLogPromptClip = 60
)

// errAIRequestEmpty 「测试连接」没拿到配置时的统一提示。
var errAIRequestEmpty = errors.New("缺少 AI 接口配置")

// AIService AI 应答的业务内核：配置读写与脱敏、连通性测试、一次完整应答。
type AIService struct {
	rag *RagService
}

// NewAIService 创建业务层实例（自带检索内核依赖）。
func NewAIService() *AIService {
	return &AIService{rag: NewRagService()}
}

// ---------------- 配置读写 / 脱敏 ----------------

// LoadSetting 读取租户的 AI 配置（缺项落默认值，永不返回 nil）。
func (s *AIService) LoadSetting(tenantID uint) *setting.AISetting {
	return setting.AISettingOf(tenantID)
}

// MaskAPIKey 把密钥替换成 ******：GET /api/settings 下发前调用，空值保持空串
// （空串表示「还没配」，不能显示成已配置的掩码）。
func (s *AIService) MaskAPIKey(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return MaskedAPIKey
}

// PrepareAPIKeyValue 保存 ai.api_key 前的加工：回写 ****** 时取库中的原值，
// 其余情况（新密钥 / 清空）原样保存。语义与对象存储的 secret_key 完全一致。
func (s *AIService) PrepareAPIKeyValue(tenantID uint, value string) string {
	if strings.TrimSpace(value) != MaskedAPIKey {
		return value
	}
	return s.LoadSetting(tenantID).APIKey
}

// ---------------- 连通性测试 ----------------

// AITestRequest 「测试连接」按钮的请求体（字段与 ai.* 设置项一一对应）。
// APIKey 传 ****** 或留空时沿用库中已保存的密钥（后台表单不回显真实 Key）。
type AITestRequest struct {
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// TestAI 用给定（或库中）配置做一次极短的实调，验证地址 / 鉴权 / 模型名是否可用。
// 返回的 error 文案直接展示给后台使用者，因此保持中文可读。
func (s *AIService) TestAI(ctx context.Context, tenantID uint, req *AITestRequest) error {
	if req == nil {
		return errAIRequestEmpty
	}
	cur := s.LoadSetting(tenantID)
	cfg := ai.Config{
		BaseURL:        strings.TrimSpace(req.BaseURL),
		APIKey:         strings.TrimSpace(req.APIKey),
		Model:          strings.TrimSpace(req.Model),
		SystemPrompt:   cur.SystemPrompt,
		TimeoutSeconds: req.TimeoutSeconds,
	}
	// 表单里没填地址/模型时，用已保存的值兜底（只改密钥也能直接测）
	if cfg.BaseURL == "" {
		cfg.BaseURL = cur.BaseURL
	}
	if cfg.Model == "" {
		cfg.Model = cur.Model
	}
	if cfg.APIKey == "" || cfg.APIKey == MaskedAPIKey {
		cfg.APIKey = cur.APIKey
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = cur.TimeoutSeconds
	}
	client, err := ai.NewClient(cfg)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return client.Ping(ctx)
}

// ---------------- 一次完整应答 ----------------

// Answer 生成一条 AI 回复；返回空串表示「不要用 AI 回复」，调用方必须回退到兜底话术。
//
// 链路：配置就绪 → 检索该渠道的自动回复规则 → 拼提示词 → 调模型 → 返回文本。
// 任一环节不满足都直接返回空串（并打日志说明原因），不做「没有知识依据的猜测」。
func (s *AIService) Answer(tenantID, channelID uint, question string) string {
	question = strings.TrimSpace(question)
	if question == "" {
		return ""
	}
	cfg := s.LoadSetting(tenantID)
	if !cfg.Enabled {
		// 关闭时不打日志：这是默认状态，每次消息都刷一行会把日志淹没
		return ""
	}
	if !cfg.Ready() {
		log.Printf("[ai] 租户 %d 已启用 AI 应答但配置不完整（base_url/api_key/model），降级为兜底话术", tenantID)
		return ""
	}
	client, err := ai.NewClient(ai.Config{
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		Model:          cfg.Model,
		SystemPrompt:   cfg.SystemPrompt,
		TimeoutSeconds: cfg.TimeoutSeconds,
	})
	if err != nil {
		log.Printf("[ai] 租户 %d 构造 AI 客户端失败，降级为兜底话术: %v", tenantID, err)
		return ""
	}

	docs, scores, err := s.rag.Retrieve(tenantID, channelID, question, cfg.TopK, cfg.MinScore)
	if err != nil {
		log.Printf("[ai] 租户 %d 检索失败，降级为兜底话术: %v", tenantID, err)
		return ""
	}
	if len(docs) == 0 {
		// 检索无命中：不猜，直接走兜底话术（也不会产生一次模型调用费用）
		log.Printf("[ai] 租户 %d 检索无命中（阈值 %.2f，问题：%s），降级为兜底话术",
			tenantID, cfg.MinScore, clipLog(question))
		return ""
	}

	prompt := BuildRagPrompt(question, docs, scores)
	reply, err := client.Chat(context.Background(), prompt)
	if err != nil {
		log.Printf("[ai] 租户 %d 调用模型失败（命中 %d 条语料），降级为兜底话术: %v",
			tenantID, len(docs), err)
		return ""
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		log.Printf("[ai] 租户 %d 模型返回空文本，降级为兜底话术", tenantID)
		return ""
	}
	log.Printf("[ai] 租户 %d AI 应答成功（命中语料：%s）", tenantID, ragDocTitles(docs))
	return reply
}

// ragDocTitles 把命中的语料标题拼成一行，用于成功日志（便于复核回答依据）。
func ragDocTitles(docs []*RagDoc) string {
	titles := make([]string, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		if t := strings.TrimSpace(d.Title); t != "" {
			titles = append(titles, t)
		}
	}
	if len(titles) == 0 {
		return "（未命名语料）"
	}
	return strings.Join(titles, " / ")
}

// clipLog 截断日志里的用户输入，避免超长消息把日志刷爆。
func clipLog(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= aiLogPromptClip {
		return string(r)
	}
	return string(r[:aiLogPromptClip]) + "…"
}
