package service_test

// 对象存储配置（settings 表 key=storage.object）业务层的单元测试：
// JSON 解析 / 长度上限 / 启用判定 / 密钥脱敏与回填 / 连通性测试的错误分支。
//
// 数据库：sqlite 内存库（与 common/repository 的测试同一套做法）——同一进程内
// 每个测试一个独立库名，SetMaxOpenConns(1) 保证多次查询命中同一个库，结束即释放。
// 真实上传（MinIO / 其它 S3 兼容服务）不在单测范围，由验收脚本验证。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/service"
	"kefu-server/pkg/oss"
	"kefu-server/pkg/setting"
	"kefu-server/pkg/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// objectStorageDBSeq 生成递增的库名，避免同包内不同测试复用同一个内存库。
var objectStorageDBSeq atomic.Uint64

// newObjectStorageTestDB 建一个只含 settings 表的内存库并注入全局连接。
func newObjectStorageTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:object-storage-test-%d?mode=memory&cache=shared", objectStorageDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存 sqlite 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
	storage.Default().SetDB(db)
	return db
}

// ossTestConfig 一份字段齐全的对象存储配置（endpoint 指向不可达端口以外的假地址）。
func ossTestConfig() *oss.Config {
	return &oss.Config{
		Endpoint:   "https://oos-cn.ctyunapi.cn",
		Region:     "",
		Bucket:     "kefu",
		AccessKey:  "AK",
		SecretKey:  "SK",
		PublicBase: "https://kefu.oos-cn.ctyunapi.cn",
		PathPrefix: "uploads/",
	}
}

func TestDefaultObjectStorageMatchesSettingDefault(t *testing.T) {
	svc := service.NewObjectStorageService()

	raw, err := svc.MarshalObjectStorage(svc.DefaultObjectStorage())
	if err != nil {
		t.Fatalf("MarshalObjectStorage 失败: %v", err)
	}
	// 业务层默认值与 pkg/setting 里的默认值必须同源：解析常量后重新序列化应完全一致。
	if raw != setting.DefaultObjectStorageJSON {
		t.Fatalf("默认配置 = %s\n期望与 setting.DefaultObjectStorageJSON 一致 = %s", raw, setting.DefaultObjectStorageJSON)
	}

	st := svc.DefaultObjectStorage()
	if st.Enabled {
		t.Error("默认配置应为停用（enabled=false）")
	}
	if st.Provider != service.DefaultObjectProvider {
		t.Errorf("默认 provider = %q, 期望 %q", st.Provider, service.DefaultObjectProvider)
	}
	if st.PathPrefix != oss.DefaultPathPrefix {
		t.Errorf("默认 path_prefix = %q, 期望 %q", st.PathPrefix, oss.DefaultPathPrefix)
	}
	if st.Ready() {
		t.Error("默认配置 Ready() 应为 false")
	}
}

func TestParseObjectStorage(t *testing.T) {
	svc := service.NewObjectStorageService()

	t.Run("空串返回默认配置", func(t *testing.T) {
		st, err := svc.ParseObjectStorage("   ")
		if err != nil {
			t.Fatalf("空串不应报错: %v", err)
		}
		if st.Enabled || st.Bucket != "" {
			t.Errorf("空串应返回停用的默认配置: %+v", st)
		}
	})

	t.Run("合法 JSON 解析并规范化", func(t *testing.T) {
		raw := `{"enabled":true,"provider":"ctyun","endpoint":"oos-cn.ctyunapi.cn/","bucket":"/kefu/",
			"access_key":"AK","secret_key":"SK","public_base":"https://kefu.oos-cn.ctyunapi.cn/","path_prefix":"/upload"}`
		st, err := svc.ParseObjectStorage(raw)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if !st.Enabled {
			t.Error("enabled 应为 true")
		}
		if st.Endpoint != "https://oos-cn.ctyunapi.cn" {
			t.Errorf("endpoint = %q, 期望补 https 并去尾斜杠", st.Endpoint)
		}
		if st.Bucket != "kefu" {
			t.Errorf("bucket = %q, 期望 kefu", st.Bucket)
		}
		if st.PublicBase != "https://kefu.oos-cn.ctyunapi.cn" {
			t.Errorf("public_base = %q, 期望去尾斜杠", st.PublicBase)
		}
		if st.PathPrefix != "upload/" {
			t.Errorf("path_prefix = %q, 期望 upload/", st.PathPrefix)
		}
		if !st.Ready() {
			t.Error("字段齐全且启用时 Ready() 应为 true")
		}
	})

	t.Run("provider 缺省补 ctyun", func(t *testing.T) {
		st, err := svc.ParseObjectStorage(`{"enabled":false,"bucket":"kefu"}`)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if st.Provider != service.DefaultObjectProvider {
			t.Errorf("provider = %q, 期望 %q", st.Provider, service.DefaultObjectProvider)
		}
		if st.PathPrefix != oss.DefaultPathPrefix {
			t.Errorf("path_prefix = %q, 期望默认值 %q", st.PathPrefix, oss.DefaultPathPrefix)
		}
	})

	for _, raw := range []string{`not-json`, `[1,2,3]`, `{"enabled":`} {
		if _, err := svc.ParseObjectStorage(raw); err == nil {
			t.Errorf("非法配置 %q 期望报错，实际通过", raw)
		}
	}
}

func TestReadyRequiresAllFields(t *testing.T) {
	svc := service.NewObjectStorageService()

	tests := []struct {
		name string
		mut  func(cfg *oss.Config)
		want bool
	}{
		{"字段齐全且启用", func(cfg *oss.Config) {}, true},
		{"缺 secret_key", func(cfg *oss.Config) { cfg.SecretKey = "" }, false},
		{"缺 access_key", func(cfg *oss.Config) { cfg.AccessKey = "" }, false},
		{"缺 bucket", func(cfg *oss.Config) { cfg.Bucket = "" }, false},
		{"缺 endpoint", func(cfg *oss.Config) { cfg.Endpoint = "" }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ossTestConfig()
			tt.mut(cfg)
			st := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider, Config: *cfg}
			if got := st.Ready(); got != tt.want {
				t.Fatalf("Ready() = %v, 期望 %v", got, tt.want)
			}
		})
	}

	t.Run("停用时即使字段齐全也不是 Ready", func(t *testing.T) {
		st := &service.ObjectStorage{Enabled: false, Provider: service.DefaultObjectProvider, Config: *ossTestConfig()}
		if st.Ready() {
			t.Fatal("enabled=false 时 Ready() 应为 false")
		}
	})

	_ = svc
}

func TestSaveAndLoadObjectStorageSetting(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 7

	// 1) 未配置：读出来是停用的默认值
	st, err := svc.LoadObjectStorageSetting(tenantID)
	if err != nil {
		t.Fatalf("未配置时读取失败: %v", err)
	}
	if st.Enabled {
		t.Error("未配置时 enabled 应为 false")
	}

	// 2) 保存启用 + 齐全的配置
	saved := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider, Config: *ossTestConfig()}
	if err := svc.SaveObjectStorageSetting(tenantID, saved); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 3) settings 表里是一条紧凑 JSON（Value 字段上限 1024 字符）
	raw := setting.GetSetting(tenantID, setting.SettingObjectStorage)
	if raw == "" {
		t.Fatal("settings 表里没有 storage.object")
	}
	if strings.ContainsAny(raw, "\n\t") {
		t.Errorf("落库的 JSON 不是单行紧凑格式: %q", raw)
	}
	if len([]rune(raw)) > service.SettingValueMaxLen {
		t.Errorf("落库长度 %d 超过上限 %d", len([]rune(raw)), service.SettingValueMaxLen)
	}
	var probe map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("落库值不是合法 JSON: %v, raw=%s", err, raw)
	}
	for _, key := range []string{"enabled", "provider", "endpoint", "region", "bucket",
		"access_key", "secret_key", "public_base", "path_prefix"} {
		if _, ok := probe[key]; !ok {
			t.Errorf("落库 JSON 缺字段 %q: %s", key, raw)
		}
	}

	// 4) LoadObjectStorage：启用且齐全 → enabled=true，连接参数原样返回
	cfg, enabled, err := svc.LoadObjectStorage(tenantID)
	if err != nil {
		t.Fatalf("LoadObjectStorage 失败: %v", err)
	}
	if !enabled {
		t.Fatal("已启用且字段齐全时第二个返回值应为 true")
	}
	if cfg == nil || cfg.Bucket != "kefu" || cfg.SecretKey != "SK" {
		t.Fatalf("连接参数不对: %+v", cfg)
	}

	// 5) 停用后 → enabled=false（上传回到本地磁盘）
	saved.Enabled = false
	if err := svc.SaveObjectStorageSetting(tenantID, saved); err != nil {
		t.Fatalf("停用保存失败: %v", err)
	}
	if _, enabled, err = svc.LoadObjectStorage(tenantID); err != nil {
		t.Fatalf("LoadObjectStorage 失败: %v", err)
	} else if enabled {
		t.Fatal("enabled=false 时第二个返回值应为 false")
	}

	// 6) SaveObjectStorage 只改连接参数，不覆盖启用开关
	if err := svc.SaveObjectStorage(tenantID, ossTestConfig()); err != nil {
		t.Fatalf("SaveObjectStorage 失败: %v", err)
	}
	st, err = svc.LoadObjectStorageSetting(tenantID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if st.Enabled {
		t.Error("SaveObjectStorage 不应把停用状态改成启用")
	}
	if st.SecretKey != "SK" {
		t.Errorf("SaveObjectStorage 后 secret_key = %q, 期望 SK", st.SecretKey)
	}
}

func TestLoadObjectStorageCorruptFallsBack(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 9

	if err := setting.SaveSetting(tenantID, setting.SettingObjectStorage, "这是一个不该出现的非 JSON 值"); err != nil {
		t.Fatalf("写入脏数据失败: %v", err)
	}

	cfg, enabled, err := svc.LoadObjectStorage(tenantID)
	if err != nil {
		t.Fatalf("配置损坏时不应把错误抛给上传链路: %v", err)
	}
	if enabled {
		t.Fatal("配置损坏时应按未启用处理")
	}
	if cfg != nil {
		t.Errorf("配置损坏时应返回 nil 配置，实际 %+v", cfg)
	}
}

func TestMaskAndRestoreObjectStorageSecret(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 11

	saved := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider, Config: *ossTestConfig()}
	if err := svc.SaveObjectStorageSetting(tenantID, saved); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	stored := setting.GetSetting(tenantID, setting.SettingObjectStorage)

	// 1) 脱敏：secret_key 变 ******，其余字段保持原值
	masked := svc.MaskObjectStorageSecret(stored)
	var got, want map[string]interface{}
	if err := json.Unmarshal([]byte(masked), &got); err != nil {
		t.Fatalf("脱敏结果不是合法 JSON: %v, %s", err, masked)
	}
	if err := json.Unmarshal([]byte(stored), &want); err != nil {
		t.Fatalf("原值不是合法 JSON: %v", err)
	}
	if got["secret_key"] != service.MaskedSecret {
		t.Fatalf("脱敏后 secret_key = %v, 期望 %q", got["secret_key"], service.MaskedSecret)
	}
	delete(got, "secret_key")
	delete(want, "secret_key")
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Fatalf("脱敏改动了其它字段: %s != %s", gb, wb)
	}

	// 2) 回填：把 ****** 写回去，密钥应恢复成库里的真实值（且不丢其它字段）
	restored, err := svc.RestoreObjectStorageSecret(tenantID, masked)
	if err != nil {
		t.Fatalf("回填失败: %v", err)
	}
	var back service.ObjectStorage
	if err := json.Unmarshal([]byte(restored), &back); err != nil {
		t.Fatalf("回填结果不是合法 JSON: %v, %s", err, restored)
	}
	if back.SecretKey != "SK" {
		t.Fatalf("回填后 secret_key = %q, 期望 SK", back.SecretKey)
	}
	if back.Bucket != "kefu" || back.Endpoint != "https://oos-cn.ctyunapi.cn" {
		t.Errorf("回填改动了其它字段: %+v", back)
	}

	// 3) 真实密钥回写：原样保留，不被库里的旧值覆盖
	next := strings.Replace(stored, `"secret_key":"SK"`, `"secret_key":"SK2"`, 1)
	out, err := svc.RestoreObjectStorageSecret(tenantID, next)
	if err != nil {
		t.Fatalf("回填失败: %v", err)
	}
	if !strings.Contains(out, `"secret_key":"SK2"`) {
		t.Fatalf("新密钥被丢弃: %s", out)
	}

	// 4) 非法 JSON 报错（不让脏值静默入库）
	if _, err := svc.RestoreObjectStorageSecret(tenantID, `not-json`); err == nil {
		t.Fatal("非法 JSON 期望报错")
	}
}

func TestPrepareObjectStorageValue(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 13

	saved := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider, Config: *ossTestConfig()}
	if err := svc.SaveObjectStorageSetting(tenantID, saved); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 1) 带空白 + ****** 的请求体：压成紧凑 JSON，密钥回填
	req := `{
		"enabled": true,
		"provider": "ctyun",
		"endpoint": "https://oos-cn.ctyunapi.cn",
		"bucket": "kefu",
		"access_key": "AK",
		"secret_key": "******",
		"public_base": "https://kefu.oos-cn.ctyunapi.cn",
		"path_prefix": "uploads/"
	}`
	value, err := svc.PrepareObjectStorageValue(tenantID, req)
	if err != nil {
		t.Fatalf("PrepareObjectStorageValue 失败: %v", err)
	}
	if strings.ContainsAny(value, "\n\t") {
		t.Errorf("未压缩成单行: %q", value)
	}
	if !strings.Contains(value, `"secret_key":"SK"`) {
		t.Fatalf("密钥未回填: %s", value)
	}

	// 2) 其它设置项不受影响
	if err := setting.SaveSetting(tenantID, setting.SettingMessageSound, "off"); err != nil {
		t.Fatalf("保存提示音失败: %v", err)
	}
	if got := setting.GetSetting(tenantID, setting.SettingMessageSound); got != "off" {
		t.Fatalf("提示音被污染: %q", got)
	}

	// 3) 超长配置要返回明确错误（settings.value 只有 1024 字符）
	long := *ossTestConfig()
	long.PathPrefix = strings.Repeat("a", service.SettingValueMaxLen)
	if _, err := svc.PrepareObjectStorageValue(tenantID, mustJSON(t, &service.ObjectStorage{
		Enabled: true, Provider: service.DefaultObjectProvider, Config: long,
	})); err == nil {
		t.Fatal("超长配置期望报错")
	} else if !strings.Contains(err.Error(), "过长") {
		t.Fatalf("超长错误提示不明确: %v", err)
	}

	// 4) MarshalObjectStorage 同样拦住超长
	if _, err := svc.MarshalObjectStorage(&service.ObjectStorage{Config: long}); err == nil {
		t.Fatal("MarshalObjectStorage 超长期望报错")
	}
}

func TestTestObjectStorage(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 17

	// 1) 字段不齐全：直接给出可读错误，不发请求
	incomplete := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider}
	err := svc.TestObjectStorage(context.Background(), tenantID, incomplete)
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("字段不齐全期望报「未配置」，实际: %v", err)
	}

	// 2) 端点不可达：返回带上下文的具体错误
	unreachable := &service.ObjectStorage{Enabled: true, Provider: service.DefaultObjectProvider, Config: oss.Config{
		Endpoint:  "http://127.0.0.1:1",
		Bucket:    "kefu",
		AccessKey: "AK",
		SecretKey: "SK",
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.TestObjectStorage(ctx, tenantID, unreachable); err == nil {
		t.Fatal("不可达端点期望报错")
	} else if !strings.Contains(err.Error(), "连接对象存储失败") {
		t.Fatalf("错误信息不明确: %v", err)
	}

	// 3) 密钥传 ****** 但库里没有已存密钥：明确提示未配置
	masked := *ossTestConfig()
	masked.SecretKey = service.MaskedSecret
	if err := svc.TestObjectStorage(ctx, tenantID, &service.ObjectStorage{
		Enabled: true, Provider: service.DefaultObjectProvider, Config: masked,
	}); err == nil {
		t.Fatal("库里无密钥时期望报错")
	} else if !strings.Contains(err.Error(), "Secret Key") {
		t.Fatalf("错误信息不明确: %v", err)
	}
}

// TestObjectClientDisabled 未启用时 ObjectClient 必须返回 nil 客户端（上传走本地）。
func TestObjectClientDisabled(t *testing.T) {
	newObjectStorageTestDB(t)
	svc := service.NewObjectStorageService()
	const tenantID uint = 19

	client, cfg, err := svc.ObjectClient(tenantID)
	if err != nil {
		t.Fatalf("未配置时不应报错: %v", err)
	}
	if client != nil {
		t.Fatal("未启用时客户端必须为 nil")
	}
	if cfg != nil {
		t.Fatalf("未启用时配置应为 nil，实际 %+v", cfg)
	}

	// 启用后应拿到客户端（不发起网络请求）
	if err := svc.SaveObjectStorageSetting(tenantID, &service.ObjectStorage{
		Enabled: true, Provider: service.DefaultObjectProvider, Config: *ossTestConfig(),
	}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	client, cfg, err = svc.ObjectClient(tenantID)
	if err != nil {
		t.Fatalf("启用后构造客户端失败: %v", err)
	}
	if client == nil {
		t.Fatal("启用且配置齐全时应返回客户端")
	}
	if cfg == nil || cfg.ObjectKey("20250101/a.png") != "uploads/20250101/a.png" {
		t.Fatalf("对象名拼接不对: %+v", cfg)
	}
}

// mustJSON 把配置序列化成 JSON（测试构造请求体用）。
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return string(b)
}
