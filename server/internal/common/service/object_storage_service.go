package service

// 本文件是对象存储（S3 兼容协议）配置的业务层：
// 把 settings 表里的 storage.object（JSON 字符串）翻译成 pkg/oss 的连接参数，
// 并在启用时向 HTTP 层提供客户端。协议细节在 pkg/oss，本层只管「配置 ↔ 设置项」。
//
// 设置项契约（key = storage.object，value 为紧凑 JSON）：
//
//	{"enabled":false,"provider":"ctyun","endpoint":"https://oos-cn.ctyunapi.cn","region":"",
//	 "bucket":"kefu","access_key":"AK","secret_key":"SK",
//	 "public_base":"https://kefu.oos-cn.ctyunapi.cn","path_prefix":"uploads/"}
//
// 未配置 / enabled=false → 上传保持原有本地磁盘存储；启用且连接参数齐全才走对象存储。
//
// 形态约定：对外能力是 *ObjectStorageService 的方法（指针接收者）；
// 配置类型 ObjectStorage 只是数据载体（连接参数 + 启用开关）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"kefu-server/pkg/oss"
	"kefu-server/pkg/setting"
)

const (
	// DefaultObjectProvider 默认（也是目前唯一）的对象存储厂商。
	DefaultObjectProvider = "ctyun"
	// MaskedSecret 脱敏后的密钥占位：后台读取时替换真实密钥，回写该值表示沿用库中原值。
	MaskedSecret = "******"
	// SettingValueMaxLen settings.value 的字段上限（gorm size:1024），超长必须显式报错。
	SettingValueMaxLen = 1024
	// errObjectStorageEmpty 配置缺省时的统一提示。
	errObjectStorageEmpty = "缺少对象存储配置"
)

// ObjectStorage 后台可维护的对象存储配置：连接参数 + 启用开关 + 厂商。
// 字段名与 settings 表里 storage.object 的 JSON 一致（连接参数直接内嵌 pkg/oss.Config）。
type ObjectStorage struct {
	// Enabled 是否启用对象存储；false 时上传落本地磁盘。
	Enabled bool `json:"enabled"`
	// Provider 厂商标识（内部值 ctyun，界面上统一表述为 S3 兼容协议）。
	Provider string `json:"provider"`
	oss.Config
}

// ObjectStorageService 对象存储配置的读写、脱敏与连通性测试。
type ObjectStorageService struct{}

// NewObjectStorageService 创建业务层实例（无跨域依赖）。
func NewObjectStorageService() *ObjectStorageService { return &ObjectStorageService{} }

// ---------------- 配置解析 ----------------

// DefaultObjectStorage 未配置时的初值（与 pkg/setting 的默认值同源）。
func (s *ObjectStorageService) DefaultObjectStorage() *ObjectStorage {
	st := &ObjectStorage{Provider: DefaultObjectProvider}
	// 默认值以 pkg/setting 的常量为唯一来源；常量由本包维护，解析失败时退回零值配置。
	_ = json.Unmarshal([]byte(setting.DefaultObjectStorageJSON), st)
	if st.Provider == "" {
		st.Provider = DefaultObjectProvider
	}
	st.Config = st.Config.Normalize()
	return st
}

// ParseObjectStorage 解析 storage.object 的 JSON 文本；空串返回默认配置。
func (s *ObjectStorageService) ParseObjectStorage(raw string) (*ObjectStorage, error) {
	st := s.DefaultObjectStorage()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return st, nil
	}
	if !strings.HasPrefix(raw, "{") {
		return nil, fmt.Errorf("对象存储配置必须是 JSON 对象: %s", clip(raw))
	}
	if err := json.Unmarshal([]byte(raw), st); err != nil {
		return nil, fmt.Errorf("对象存储配置解析失败: %w", err)
	}
	if st.Provider == "" {
		st.Provider = DefaultObjectProvider
	}
	st.Config = st.Config.Normalize()
	return st, nil
}

// MarshalObjectStorage 把配置序列化为紧凑 JSON，并校验 settings.value 的长度上限。
func (s *ObjectStorageService) MarshalObjectStorage(st *ObjectStorage) (string, error) {
	if st == nil {
		return "", errors.New("对象存储配置不能为空")
	}
	out := *st
	out.Config = out.Config.Normalize()
	if out.Provider == "" {
		out.Provider = DefaultObjectProvider
	}
	b, err := json.Marshal(&out) // json.Marshal 输出本身就是紧凑 JSON
	if err != nil {
		return "", fmt.Errorf("序列化对象存储配置失败: %w", err)
	}
	if n := len([]rune(string(b))); n > SettingValueMaxLen {
		return "", fmt.Errorf("对象存储配置过长（%d 字符），超过设置项上限 %d 字符，请缩短域名或对象前缀", n, SettingValueMaxLen)
	}
	return string(b), nil
}

// Ready 报告配置是否「已启用且连接参数齐全」：为 false 时上传必须落本地磁盘。
func (s *ObjectStorage) Ready() bool {
	if s == nil || !s.Enabled {
		return false
	}
	cfg := s.Config.Normalize()
	return cfg.Complete()
}

// ---------------- 读写设置项（settings 表，key = storage.object） ----------------

// LoadObjectStorageSetting 读取完整设置（含 enabled / provider），未配置时返回默认值。
func (s *ObjectStorageService) LoadObjectStorageSetting(tenantID uint) (*ObjectStorage, error) {
	return s.ParseObjectStorage(setting.GetSetting(tenantID, setting.SettingObjectStorage))
}

// SaveObjectStorageSetting 保存完整设置（含 enabled / provider），写入 settings 表。
func (s *ObjectStorageService) SaveObjectStorageSetting(tenantID uint, st *ObjectStorage) error {
	raw, err := s.MarshalObjectStorage(st)
	if err != nil {
		return err
	}
	return setting.SaveSetting(tenantID, setting.SettingObjectStorage, raw)
}

// LoadObjectStorage 读取租户的对象存储连接参数。
// 第二个返回值为 true 表示「已启用且连接参数齐全」，此时调用方应上传到对象存储；
// 为 false 时调用方必须保持原有本地磁盘存储（未配置 / 停用 / 配置损坏）。
func (s *ObjectStorageService) LoadObjectStorage(tenantID uint) (*oss.Config, bool, error) {
	st, err := s.LoadObjectStorageSetting(tenantID)
	if err != nil {
		// 配置损坏不阻塞上传：当作「未启用」处理并告警，避免整站上传不可用。
		log.Printf("[storage] 对象存储配置解析失败（租户 %d），按未启用处理: %v", tenantID, err)
		return nil, false, nil
	}
	cfg := st.Config
	if !st.Ready() {
		return &cfg, false, nil
	}
	return &cfg, true, nil
}

// SaveObjectStorage 保存连接参数（enabled / provider 沿用库中原值）。
// 需要同时切换启用状态时用 SaveObjectStorageSetting。
func (s *ObjectStorageService) SaveObjectStorage(tenantID uint, cfg *oss.Config) error {
	if cfg == nil {
		return errors.New("对象存储配置不能为空")
	}
	cur, err := s.LoadObjectStorageSetting(tenantID)
	if err != nil {
		return err
	}
	next := *cur
	next.Config = cfg.Normalize()
	if next.Provider == "" {
		next.Provider = DefaultObjectProvider
	}
	return s.SaveObjectStorageSetting(tenantID, &next)
}

// ObjectClient 构造租户的对象存储客户端。
// 未启用时返回 (nil, nil, nil)：调用方走原有本地磁盘逻辑。
func (s *ObjectStorageService) ObjectClient(tenantID uint) (*oss.Client, *oss.Config, error) {
	cfg, enabled, err := s.LoadObjectStorage(tenantID)
	if err != nil || !enabled || cfg == nil {
		return nil, nil, err
	}
	client, err := oss.NewClient(*cfg)
	if err != nil {
		return nil, cfg, fmt.Errorf("对象存储配置不可用: %w", err)
	}
	return client, cfg, nil
}

// ---------------- 密钥脱敏 / 回填 ----------------

// MaskObjectStorageSecret 把配置 JSON 里的 secret_key 脱敏为 ******，其余字段原样保留。
// 非法 JSON（历史脏数据）原样返回，避免把整段配置抹掉。
func (s *ObjectStorageService) MaskObjectStorageSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	fields, err := decodeObjectStorageFields(raw)
	if err != nil {
		return raw
	}
	fields["secret_key"] = json.RawMessage(strconv.Quote(MaskedSecret))
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return string(out)
}

// RestoreObjectStorageSecret 若 raw 里的 secret_key 是脱敏占位，用库里已存的密钥回填。
// 其它情况（真实密钥 / 空串 / 非法 JSON）原样返回。
func (s *ObjectStorageService) RestoreObjectStorageSecret(tenantID uint, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, nil
	}
	fields, err := decodeObjectStorageFields(trimmed)
	if err != nil {
		return "", err
	}
	var secret string
	if v, ok := fields["secret_key"]; ok {
		_ = json.Unmarshal(v, &secret)
	}
	if secret != MaskedSecret {
		return trimmed, nil
	}
	cur, err := s.LoadObjectStorageSetting(tenantID)
	if err != nil {
		return "", fmt.Errorf("读取已保存的对象存储密钥失败: %w", err)
	}
	fields["secret_key"] = json.RawMessage(strconv.Quote(cur.SecretKey))
	out, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("回填对象存储密钥失败: %w", err)
	}
	return string(out), nil
}

// PrepareObjectStorageValue 保存 storage.object 前的统一加工：
// 密钥脱敏占位回填库中原值 → 校验合法性 → 压成紧凑 JSON → 校验长度上限。
func (s *ObjectStorageService) PrepareObjectStorageValue(tenantID uint, raw string) (string, error) {
	restored, err := s.RestoreObjectStorageSecret(tenantID, raw)
	if err != nil {
		return "", err
	}
	if _, err := s.ParseObjectStorage(restored); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(restored)); err != nil {
		return "", fmt.Errorf("对象存储配置不是合法 JSON: %w", err)
	}
	value := buf.String()
	if n := len([]rune(value)); n > SettingValueMaxLen {
		return "", fmt.Errorf("对象存储配置过长（%d 字符），超过设置项上限 %d 字符，请缩短域名或对象前缀", n, SettingValueMaxLen)
	}
	return value, nil
}

// ---------------- 连通性测试 ----------------

// TestObjectStorage 用给定配置测试对象存储：构造客户端 → Ping（上传并删除一个探针对象）。
// secret_key 传 ****** 时沿用库里已存的密钥（后台表单不回显真实密钥）。
func (s *ObjectStorageService) TestObjectStorage(ctx context.Context, tenantID uint, st *ObjectStorage) error {
	if st == nil {
		return errors.New(errObjectStorageEmpty)
	}
	cfg := st.Config
	if strings.TrimSpace(cfg.SecretKey) == MaskedSecret {
		cur, err := s.LoadObjectStorageSetting(tenantID)
		if err != nil {
			return fmt.Errorf("读取已保存的对象存储密钥失败: %w", err)
		}
		if cur.SecretKey == "" {
			return errors.New("未配置对象存储 Secret Key")
		}
		cfg.SecretKey = cur.SecretKey
	}
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	client, err := oss.NewClient(cfg)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return client.Ping(ctx)
}

// ---------------- 包内工具 ----------------

// decodeObjectStorageFields 把配置 JSON 解析成「字段 → 原始值」，
// 用于只替换 secret_key 而不丢未知字段。
func decodeObjectStorageFields(raw string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, fmt.Errorf("对象存储配置不是合法 JSON: %w", err)
	}
	if fields == nil {
		return nil, errors.New("对象存储配置不是合法 JSON 对象")
	}
	return fields, nil
}

// clip 截断过长的原始文本，避免把整段配置塞进错误提示。
func clip(s string) string {
	const max = 48
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
