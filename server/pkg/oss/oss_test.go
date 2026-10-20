package oss_test

// pkg/oss 的单元测试：配置规范化、对象名拼接、对外 URL、参数校验与错误分支。
//
// 不打真实对象存储：需要联网的分支（Upload / Delete / Ping 的服务端交互）
// 只覆盖「本地就能判定失败」的入参与不可达端点，真实上传由验收脚本验证。

import (
	"context"
	"strings"
	"testing"
	"time"

	"kefu-server/pkg/oss"
)

// fullConfig 一份字段齐全的配置（endpoint 带 https，便于断言 useSSL 分支）。
func fullConfig() oss.Config {
	return oss.Config{
		Endpoint:   "https://oos-cn.ctyunapi.cn",
		Region:     "",
		Bucket:     "kefu",
		AccessKey:  "AK",
		SecretKey:  "SK",
		PublicBase: "https://kefu.oos-cn.ctyunapi.cn",
		PathPrefix: "uploads/",
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		in    oss.Config
		check func(t *testing.T, got oss.Config)
	}{
		{
			name: "缺协议头补 https，去尾部斜杠",
			in:   oss.Config{Endpoint: " oos-cn.ctyunapi.cn/// ", Bucket: " /kefu/ ", AccessKey: " AK ", SecretKey: " SK "},
			check: func(t *testing.T, got oss.Config) {
				if got.Endpoint != "https://oos-cn.ctyunapi.cn" {
					t.Errorf("endpoint = %q, 期望 %q", got.Endpoint, "https://oos-cn.ctyunapi.cn")
				}
				if got.Bucket != "kefu" {
					t.Errorf("bucket = %q, 期望 %q", got.Bucket, "kefu")
				}
				if got.AccessKey != "AK" || got.SecretKey != "SK" {
					t.Errorf("密钥未去空白: ak=%q sk=%q", got.AccessKey, got.SecretKey)
				}
			},
		},
		{
			name: "http 端点保持 http",
			in:   oss.Config{Endpoint: "http://127.0.0.1:9000"},
			check: func(t *testing.T, got oss.Config) {
				if got.Endpoint != "http://127.0.0.1:9000" {
					t.Errorf("endpoint = %q, 期望 %q", got.Endpoint, "http://127.0.0.1:9000")
				}
			},
		},
		{
			name: "前缀缺省为 uploads/",
			in:   oss.Config{},
			check: func(t *testing.T, got oss.Config) {
				if got.PathPrefix != oss.DefaultPathPrefix {
					t.Errorf("path_prefix = %q, 期望 %q", got.PathPrefix, oss.DefaultPathPrefix)
				}
			},
		},
		{
			name: "前缀补尾随斜杠、去前导斜杠",
			in:   oss.Config{PathPrefix: "/images"},
			check: func(t *testing.T, got oss.Config) {
				if got.PathPrefix != "images/" {
					t.Errorf("path_prefix = %q, 期望 %q", got.PathPrefix, "images/")
				}
			},
		},
		{
			name: "public_base 去尾部斜杠",
			in:   oss.Config{PublicBase: "https://cdn.example.com/"},
			check: func(t *testing.T, got oss.Config) {
				if got.PublicBase != "https://cdn.example.com" {
					t.Errorf("public_base = %q, 期望 %q", got.PublicBase, "https://cdn.example.com")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.in
			tt.check(t, cfg.Normalize())
			// Normalize 不得修改调用方（返回副本）
			if cfg.Endpoint == "" && tt.in.Endpoint != "" {
				t.Errorf("Normalize 修改了原配置")
			}
		})
	}
}

func TestObjectKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		in     string
		want   string
	}{
		{"默认前缀 + 日期目录", "", "20250101/abc.png", "uploads/20250101/abc.png"},
		{"自定义前缀", "images/", "20250101/abc.png", "images/20250101/abc.png"},
		{"去掉对象名前导斜杠", "uploads/", "/20250101/abc.png", "uploads/20250101/abc.png"},
		{"对象名反斜杠归一化", "uploads/", `20250101\abc.png`, "uploads/20250101/abc.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := oss.Config{PathPrefix: tt.prefix}
			if got := cfg.ObjectKey(tt.in); got != tt.want {
				t.Fatalf("ObjectKey(%q) = %q, 期望 %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  oss.Config
		key  string
		want string
	}{
		{
			name: "优先 public_base（CDN）",
			cfg:  oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", Bucket: "kefu", PublicBase: "https://kefu.oos-cn.ctyunapi.cn"},
			key:  "uploads/20250101/abc.png",
			want: "https://kefu.oos-cn.ctyunapi.cn/uploads/20250101/abc.png",
		},
		{
			name: "public_base 为空回退 endpoint/bucket/key（https）",
			cfg:  oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", Bucket: "kefu"},
			key:  "uploads/abc.png",
			want: "https://oos-cn.ctyunapi.cn/kefu/uploads/abc.png",
		},
		{
			name: "public_base 为空回退 endpoint/bucket/key（http）",
			cfg:  oss.Config{Endpoint: "http://127.0.0.1:9000", Bucket: "kefu"},
			key:  "uploads/abc.png",
			want: "http://127.0.0.1:9000/kefu/uploads/abc.png",
		},
		{
			name: "public_base 尾部斜杠不会拼出双斜杠",
			cfg:  oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", Bucket: "kefu", PublicBase: "https://cdn.example.com/"},
			key:  "/uploads/abc.png",
			want: "https://cdn.example.com/uploads/abc.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			if got := cfg.URL(tt.key); got != tt.want {
				t.Fatalf("URL(%q) = %q, 期望 %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     oss.Config
		wantErr string // 空串表示期望通过
	}{
		{"齐全", fullConfig(), ""},
		{"缺 endpoint", oss.Config{Bucket: "kefu", AccessKey: "AK", SecretKey: "SK"}, "未配置对象存储端点"},
		{"缺 bucket", oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", AccessKey: "AK", SecretKey: "SK"}, "未配置对象存储桶"},
		{"缺 access_key", oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", Bucket: "kefu", SecretKey: "SK"}, "未配置对象存储 Access Key"},
		{"缺 secret_key", oss.Config{Endpoint: "https://oos-cn.ctyunapi.cn", Bucket: "kefu", AccessKey: "AK"}, "未配置对象存储 Secret Key"},
		{"endpoint 只有协议头", oss.Config{Endpoint: "https://", Bucket: "kefu", AccessKey: "AK", SecretKey: "SK"}, "对象存储端点不合法"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("期望校验通过，实际报错: %v", err)
				}
				if !cfg.Complete() {
					t.Fatalf("Complete() = false, 期望 true")
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错 %q，实际通过", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("错误信息 = %q, 期望包含 %q", err.Error(), tt.wantErr)
			}
			if cfg.Complete() {
				t.Fatalf("Complete() = true, 期望 false")
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("配置不齐全时报错", func(t *testing.T) {
		if _, err := oss.NewClient(oss.Config{}); err == nil {
			t.Fatal("期望 NewClient 报错，实际成功")
		}
	})

	t.Run("配置齐全时拿到规范化后的配置", func(t *testing.T) {
		cli, err := oss.NewClient(oss.Config{
			Endpoint:  "oos-cn.ctyunapi.cn/",
			Bucket:    "kefu",
			AccessKey: "AK",
			SecretKey: "SK",
		})
		if err != nil {
			t.Fatalf("NewClient 失败: %v", err)
		}
		cfg := cli.Config()
		if cfg.Endpoint != "https://oos-cn.ctyunapi.cn" {
			t.Errorf("生效 endpoint = %q, 期望补 https://", cfg.Endpoint)
		}
		if cfg.PathPrefix != oss.DefaultPathPrefix {
			t.Errorf("生效 path_prefix = %q, 期望 %q", cfg.PathPrefix, oss.DefaultPathPrefix)
		}
		if cli.Bucket() != "kefu" {
			t.Errorf("bucket = %q, 期望 kefu", cli.Bucket())
		}
		// public_base 留空时 URL 回退 endpoint/bucket/key
		if got, want := cli.URL("uploads/a.png"), "https://oos-cn.ctyunapi.cn/kefu/uploads/a.png"; got != want {
			t.Errorf("URL = %q, 期望 %q", got, want)
		}
	})
}

func TestUploadAndDeleteInvalidArgs(t *testing.T) {
	cli, err := oss.NewClient(fullConfig())
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	ctx := context.Background()

	if _, err := cli.Upload(ctx, "  ", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Error("空对象名应报错")
	}
	if _, err := cli.Upload(ctx, "uploads/a.txt", nil, 0, "text/plain"); err == nil {
		t.Error("nil 内容应报错")
	}
	if err := cli.Delete(ctx, ""); err == nil {
		t.Error("删除空对象名应报错")
	}
	if _, err := cli.Stat(ctx, ""); err == nil {
		t.Error("查询空对象名应报错")
	}
}

// TestPingUnreachable 端点不可达时必须返回错误（且不 panic），
// 这是「上传失败分支」与「测试连接失败」两块提示语的基础。
func TestPingUnreachable(t *testing.T) {
	cli, err := oss.NewClient(oss.Config{
		Endpoint:  "http://127.0.0.1:1",
		Bucket:    "kefu",
		AccessKey: "AK",
		SecretKey: "SK",
	})
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := cli.Ping(ctx); err == nil {
		t.Fatal("不可达端点期望 Ping 报错，实际成功")
	} else if !strings.Contains(err.Error(), "连接对象存储失败") {
		t.Fatalf("Ping 错误信息 = %q, 期望包含 %q", err.Error(), "连接对象存储失败")
	}
}
