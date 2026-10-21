// Package setting 租户系统配置（默认图片、提示音等）。
//
// 一开始是 service/setting.go + repository/setting.go 两层，后来整体搬入 platform：
// 公共账号服务的登录响应需要默认图片地址（DefaultImageURL），若留在业务层会形成
// 账号域 ⇄ setting 的环，因此把它当基础设施看待。业务层只调用本包的导出函数。
package setting

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm/clause"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// ---------------- 设置项 Key ----------------

const (
	// SettingDefaultImage 统一默认图片：客户/商品/订单无图时使用
	SettingDefaultImage = "default_image"
	// SettingMessageSound 新消息提示音：on=内置提示音（默认），off=静音，或填音频 URL
	SettingMessageSound = "message_sound"
	// SettingSystemName 系统名称：展示在后台顶栏与页面标题，留空则回退到租户名称
	SettingSystemName = "system.name"
	// SettingRobotFallback 机器人兜底话术：渠道开启机器人但未命中任何规则时的回复
	SettingRobotFallback = "robot.fallback_reply"
	// defaultRobotFallback 兜底话术的默认值（后台未配置或清空时使用）
	defaultRobotFallback = "您的问题我已记录，稍后将由人工客服为您详细解答～"
	// SettingObjectStorage 对象存储（S3 兼容）配置：value 是一段 JSON
	SettingObjectStorage = "storage.object"
	// SettingAssistantAvatar 智能助手（机器人）回复时展示的头像 URL，留空用内置头像
	SettingAssistantAvatar = "assistant.avatar"
)

// ---------------- AI 应答 + RAG 知识库设置项 ----------------
//
// 与对象存储不同，这些配置逐项存在 settings 表里（一个 key 一个值），后台设置页逐项编辑：
// 管理员只改超时 / top_k 这类小参数时不会因为整段 JSON 回写而覆盖别的字段。
// 全部默认关闭：未配置时机器人保持「关键词规则 + 兜底话术」的原有行为。
const (
	// SettingAIEnabled 是否启用 AI 应答（true 时才可能调用外部大模型）
	SettingAIEnabled = "ai.enabled"
	// SettingAIBaseURL OpenAI 兼容接口根地址，如 https://api.deepseek.com/v1
	SettingAIBaseURL = "ai.base_url"
	// SettingAIAPIKey API Key；GET /api/settings 下发时脱敏为 ******，回写 ****** 表示沿用原值
	SettingAIAPIKey = "ai.api_key"
	// SettingAIModel 模型名，如 deepseek-chat
	SettingAIModel = "ai.model"
	// SettingAISystemPrompt 系统提示词（约束模型只依据知识库回答）
	SettingAISystemPrompt = "ai.system_prompt"
	// SettingAITimeoutSeconds 单次请求超时（秒）
	SettingAITimeoutSeconds = "ai.timeout_seconds"
	// SettingRagTopK RAG 检索返回条数
	SettingRagTopK = "rag.top_k"
	// SettingRagMinScore RAG 最低相关度（0~1，低于该值视为无命中）
	SettingRagMinScore = "rag.min_score"
)

// DefaultAISystemPrompt AI 系统提示词的默认值：
// 只依据知识库回答，不确定就转人工（避免模型凭空编造售后政策）。
const DefaultAISystemPrompt = "你是企业在线客服助手。只能依据下面提供的「知识库片段」回答客户问题；" +
	"知识库没有覆盖的内容不要猜测或编造，直接回复：这个问题我需要转接人工客服为您确认。" +
	"回答使用简体中文，语气礼貌、简洁，不要输出标题、编号列表或 Markdown 标记。"

// AI 设置项的默认值（与 settingDefaults 同源，供后台表单与解析共用）。
const (
	// DefaultAITimeoutSeconds 默认请求超时（秒）
	DefaultAITimeoutSeconds = 20
	// DefaultRagTopK 默认检索条数
	DefaultRagTopK = 3
	// DefaultRagMinScore 默认最低相关度
	DefaultRagMinScore = 0.15
)

// DefaultObjectStorageJSON 对象存储配置的默认值（未配置时的初值，端点留空由使用者填写）。
// 与 common/service 里 ObjectStorage 的字段一一对应；连接参数的解析与校验在那一边。
const DefaultObjectStorageJSON = `{"enabled":false,"provider":"ctyun","endpoint":"","region":"","bucket":"","access_key":"","secret_key":"","public_base":"","path_prefix":"uploads/"}`

// defaultImageFallback 前端未配置时使用的内置兜底图
const defaultImageFallback = "/img/placeholder.svg"

var settingDefaults = map[string]struct {
	Value  string
	Remark string
}{
	SettingDefaultImage: {
		Value:  defaultImageFallback,
		Remark: "客户 / 商品 / 订单没有图片时展示的默认图片（留空则使用内置兜底图）",
	},
	SettingMessageSound: {
		Value:  "on",
		Remark: "新消息提示音：on=内置提示音（默认），off=静音，或填自定义音频 URL",
	},
	SettingObjectStorage: {
		Value:  DefaultObjectStorageJSON,
		Remark: "对象存储（S3 兼容）：enabled=true 时上传走对象存储，value 为 JSON 配置",
	},
	SettingAssistantAvatar: {
		// 默认留空：前端（用户端 / 客服工作台）各自回退到内置头像，避免把某个租户的 logo 写死成全局默认
		Value:  "",
		Remark: "智能助手（机器人）回复时展示的头像 URL；留空使用内置头像",
	},
	SettingSystemName: {
		Value:  "",
		Remark: "系统名称：展示在管理后台顶栏与浏览器标题；留空则使用租户名称",
	},
	SettingRobotFallback: {
		Value:  defaultRobotFallback,
		Remark: "渠道开启机器人但未命中任何自动回复规则时的兜底回复；留空则使用默认话术",
	},
	// ---------------- AI 应答（默认关闭） ----------------
	SettingAIEnabled: {
		Value:  "false",
		Remark: "是否启用 AI 应答：true 时未命中自动回复规则的消息会走「知识库检索 + 大模型」生成回复",
	},
	SettingAIBaseURL: {
		Value:  "",
		Remark: "OpenAI 兼容接口根地址，如 https://api.deepseek.com/v1（不含 /chat/completions）",
	},
	SettingAIAPIKey: {
		Value:  "",
		Remark: "大模型接口的 API Key；保存后只显示 ******，原样回写表示沿用已保存的 Key",
	},
	SettingAIModel: {
		Value:  "",
		Remark: "模型名，如 deepseek-chat / gpt-4o-mini（以服务商文档为准）",
	},
	SettingAISystemPrompt: {
		Value:  DefaultAISystemPrompt,
		Remark: "系统提示词：要求模型只依据知识库回答，不确定就转人工",
	},
	SettingAITimeoutSeconds: {
		Value:  "20",
		Remark: "单次请求超时（秒）；超时后自动降级为兜底话术",
	},
	SettingRagTopK: {
		Value:  "3",
		Remark: "知识库检索返回的条数（拼进提示词的知识片段数量）",
	},
	SettingRagMinScore: {
		Value:  "0.15",
		Remark: "最低相关度（0~1）：低于该值的知识片段视为未命中，不猜测、直接回兜底话术",
	},
}

// ---------------- 对外能力 ----------------

// GetSetting 读取配置项，不存在时返回默认值
func GetSetting(tenantID uint, key string) string {
	// 查不到（ErrRecordNotFound）或读库失败都落到默认值，与原逻辑一致
	if s, err := findSettingByKey(tenantID, key); err == nil {
		if v := strings.TrimSpace(s.Value); v != "" {
			return v
		}
	}
	if d, ok := settingDefaults[key]; ok {
		return d.Value
	}
	return ""
}

// ListSettings 返回全部配置项（含默认值，便于后台渲染表单）
func ListSettings(tenantID uint) ([]map[string]interface{}, error) {
	rows := map[string]*model.Setting{}
	list, err := listSettingsByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		rows[s.Key] = s
	}

	// 以默认项为基础，叠加已保存值，再补充自定义项
	keys := make([]string, 0, len(settingDefaults)+len(rows))
	seen := map[string]bool{}
	for k := range settingDefaults {
		keys = append(keys, k)
		seen[k] = true
	}
	for _, s := range list {
		if !seen[s.Key] {
			keys = append(keys, s.Key)
			seen[s.Key] = true
		}
	}

	out := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		val := GetSetting(tenantID, k)
		remark := ""
		if d, ok := settingDefaults[k]; ok {
			remark = d.Remark
		} else if s, ok := rows[k]; ok {
			remark = s.Remark
		}
		out = append(out, map[string]interface{}{
			"key":    k,
			"value":  val,
			"remark": remark,
		})
	}
	return out, nil
}

// SaveSetting 保存单个配置项
func SaveSetting(tenantID uint, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("配置项不能为空")
	}
	value = strings.TrimSpace(value)

	// 用 upsert 一步写入：存在则更新 value（与 remark 同步），不存在则插入。
	// 这样既不走「先查后插」的竞态，也不依赖 findSettingByKey 的查询是否命中。
	row := model.Setting{TenantID: tenantID, Key: key, Value: value}
	if d, ok := settingDefaults[key]; ok {
		row.Remark = d.Remark
	}
	return storage.Default().DB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// DefaultImageURL 默认图片地址（供前端兜底）
func DefaultImageURL(tenantID uint) string {
	return GetSetting(tenantID, SettingDefaultImage)
}

// MessageSound 新消息提示音配置（on / off / 自定义 URL）
func MessageSound(tenantID uint) string {
	return GetSetting(tenantID, SettingMessageSound)
}

// AssistantAvatar 智能助手（机器人）回复时展示的头像地址（用户端 / 客服工作台共用）。
// 未配置或留空时返回空串：前端据此回退到各自的「内置头像」样式（不用默认图片兜底，
// 否则助手头像会变成客户 / 商品无图时的占位图，语义不对）。
func AssistantAvatar(tenantID uint) string {
	return strings.TrimSpace(GetSetting(tenantID, SettingAssistantAvatar))
}

// ---------------- 数据访问（原 repository/setting.go） ----------------

// findSettingByKey 按租户 + key 查询单个配置项；未命中返回 gorm.ErrRecordNotFound。
func findSettingByKey(tenantID uint, key string) (*model.Setting, error) {
	var s model.Setting
	// 用结构体条件而不是裸 SQL：`key` 是 MySQL 保留字，写成 "key = ?" 会报语法错，
	// 被上层当成「不存在」→ 走 INSERT → 撞 idx_tenant_key 唯一索引（1062）。GORM 对结构体条件会自动加反引号。
	if err := storage.Default().DB().Where(&model.Setting{TenantID: tenantID, Key: key}).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// listSettingsByTenant 列出租户已保存的全部配置项（保持原有不排序的取数方式）。
func listSettingsByTenant(tenantID uint) ([]*model.Setting, error) {
	var list []*model.Setting
	if err := storage.Default().DB().Where("tenant_id = ?", tenantID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// createSetting 新增配置项，成功后回填主键与时间戳。
func createSetting(s *model.Setting) error {
	return storage.Default().DB().Create(s).Error
}

// saveSetting 保存配置项全字段（已存在配置的更新路径）。
func saveSetting(s *model.Setting) error {
	return storage.Default().DB().Save(s).Error
}

// RobotFallback 机器人兜底话术（渠道开启机器人但未命中规则时的回复）。
// 未配置时返回默认话术；显式配成空串表示「不回复」。
func RobotFallback(tenantID uint) string {
	if s, err := findSettingByKey(tenantID, SettingRobotFallback); err == nil {
		if v := strings.TrimSpace(s.Value); v != "" {
			return v
		}
	}
	// 未配置 / 清空：回退默认话术（避免「配置清掉后机器人彻底不回复」）
	return defaultRobotFallback
}

// ---------------- AI 应答 / RAG 设置 ----------------

// AISetting AI 应答与知识库检索的租户配置（全部来自 settings 表，默认关闭）。
// 这里只是数据载体：连接参数怎么用（鉴权、超时、请求体）由 pkg/ai 决定，
// 「什么时候该调 AI」的判定在 common/service 的 AI 应答内核里。
type AISetting struct {
	// Enabled 是否启用 AI 应答；false 时机器人只走「规则 → 兜底话术」。
	Enabled bool
	// BaseURL OpenAI 兼容接口根地址（不含 /chat/completions）。
	BaseURL string
	// APIKey 大模型接口密钥（库里是明文，仅在服务端使用，不下发给前端）。
	APIKey string
	// Model 模型名。
	Model string
	// SystemPrompt 系统提示词。
	SystemPrompt string
	// TimeoutSeconds 单次请求超时（秒）。
	TimeoutSeconds int
	// TopK 知识库检索条数。
	TopK int
	// MinScore 最低相关度（0~1）。
	MinScore float64
}

// AISettingOf 读取租户的 AI 应答配置（缺项一律落到默认值，绝不返回 nil）。
//
// 性能口径：AI 应答在「未命中规则」的消息热路径上，一次要读 8 个设置项；
// 逐项 GetSetting 就是 8 次查询，因此这里用一次 listSettingsByTenant 取回整租户的配置再落默认值。
// 读库失败时按「全部默认值」处理（即 AI 关闭），保证消息链路不受影响。
func AISettingOf(tenantID uint) *AISetting {
	raw := map[string]string{}
	if list, err := listSettingsByTenant(tenantID); err == nil {
		for _, s := range list {
			raw[s.Key] = strings.TrimSpace(s.Value)
		}
	}
	// get 与 GetSetting 同一口径：库里空值 → 回落默认值
	get := func(key string) string {
		if v := raw[key]; v != "" {
			return v
		}
		if d, ok := settingDefaults[key]; ok {
			return d.Value
		}
		return ""
	}
	cfg := &AISetting{
		Enabled:        parseBoolSetting(get(SettingAIEnabled), false),
		BaseURL:        get(SettingAIBaseURL),
		APIKey:         get(SettingAIAPIKey),
		Model:          get(SettingAIModel),
		SystemPrompt:   get(SettingAISystemPrompt),
		TimeoutSeconds: parseIntSetting(get(SettingAITimeoutSeconds), DefaultAITimeoutSeconds),
		TopK:           parseIntSetting(get(SettingRagTopK), DefaultRagTopK),
		MinScore:       parseFloatSetting(get(SettingRagMinScore), DefaultRagMinScore),
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultAISystemPrompt
	}
	// 检索参数做一次夹取：后台可能填 0 或负数，落到检索层会变成「不检索」或全量扫描
	if cfg.TopK <= 0 {
		cfg.TopK = DefaultRagTopK
	}
	if cfg.TopK > MaxRagTopK {
		cfg.TopK = MaxRagTopK
	}
	if cfg.MinScore < 0 {
		cfg.MinScore = 0
	}
	if cfg.MinScore > 1 {
		cfg.MinScore = 1
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = DefaultAITimeoutSeconds
	}
	return cfg
}

// MaxRagTopK 单次检索条数上限：拼进提示词的片段过多既贵又会稀释重点。
const MaxRagTopK = 10

// Ready 报告 AI 配置是否「已启用且连接参数齐全」：不齐全时调用方必须保持原有兜底行为。
func (c *AISetting) Ready() bool {
	if c == nil || !c.Enabled {
		return false
	}
	return c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

// parseBoolSetting 解析布尔配置：true/1/on/yes 为真（大小写不敏感），其余取默认值。
func parseBoolSetting(raw string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "on", "yes", "y":
		return true
	case "false", "0", "off", "no", "n":
		return false
	}
	return def
}

// parseIntSetting 解析整数配置：留空或非法时取默认值。
func parseIntSetting(raw string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return n
	}
	return def
}

// parseFloatSetting 解析浮点配置：留空或非法时取默认值。
func parseFloatSetting(raw string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		return f
	}
	return def
}
