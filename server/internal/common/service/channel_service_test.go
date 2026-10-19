package service_test

// 渠道接入标识 / 接入地址的纯函数测试（AccessPath / ChannelKey / AccessURL）。
//
// 这三个函数不碰数据库，只依赖 model.Channel 与全局配置 config.C.Server.PublicBase，
// 因此用外部测试包 channel_test 只验证对外契约。

import (
	"net/url"
	"testing"

	"kefu-server/internal/common/config"
	"kefu-server/internal/common/model"
	"kefu-server/internal/common/service"
)

// withPublicBase 设置全局配置里的 public_base，并在用例结束后还原，
// 避免不同的用例之间（以及将来并行的用例）互相污染。
func withPublicBase(t *testing.T, base string) {
	t.Helper()
	old := config.C.Server.PublicBase
	config.C.Server.PublicBase = base
	t.Cleanup(func() { config.C.Server.PublicBase = old })
}

func TestChannelKey(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		id    uint
		want  string
	}{
		{
			name:  "有 app_id 时优先用 app_id",
			appID: "kefu_a1b2c3d4", id: 7,
			want: "kefu_a1b2c3d4",
		},
		{
			name:  "app_id 为空时回退到主键 id",
			appID: "", id: 42,
			want: "42",
		},
		{
			name:  "app_id 只有空白字符时回退到主键 id",
			appID: "   \t ", id: 9,
			want: "9",
		},
		{
			name:  "app_id 前后空白被裁掉",
			appID: "  kefu_x1  ", id: 3,
			want: "kefu_x1",
		},
		{
			name:  "app_id 与 id 同时为空/0 时返回 '0'",
			appID: "", id: 0,
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &model.Channel{Model: model.Model{ID: tt.id}, AppID: tt.appID}
			got := service.ChannelKey(c)
			if got != tt.want {
				t.Errorf("ChannelKey(app_id=%q, id=%d) = %q, 期望 %q", tt.appID, tt.id, got, tt.want)
			}
		})
	}
}

func TestAccessPath(t *testing.T) {
	tests := []struct {
		name string
		ch   *model.Channel
		want string
		// checkEscape 为 true 时额外校验 want 中的 key 部分是 url.QueryEscape 的结果
		checkEscape bool
	}{
		{
			name: "普通渠道：用 app_id 组成接入路径",
			ch:   &model.Channel{Model: model.Model{ID: 1}, Type: "wechat", AppID: "kefu_a1b2c3d4"},
			want: "/kefu/?channel=kefu_a1b2c3d4",
		},
		{
			name: "app_id 为空：用主键 id 组成接入路径",
			ch:   &model.Channel{Model: model.Model{ID: 88}, Type: "web"},
			want: "/kefu/?channel=88",
		},
		{
			name: "自定义类型（中文）不影响接入路径：仍然只看 app_id",
			ch:   &model.Channel{Model: model.Model{ID: 5}, Type: "小红书", AppID: "xhs_001"},
			want: "/kefu/?channel=xhs_001",
		},
		{
			name: "自定义类型且无 app_id：回退到 id",
			ch:   &model.Channel{Model: model.Model{ID: 6}, Type: "线下门店"},
			want: "/kefu/?channel=6",
		},
		{
			name:        "app_id 含需转义字符（空格 / & / =）时按 query 规则转义",
			ch:          &model.Channel{Model: model.Model{ID: 2}, Type: "custom", AppID: "a b&c=d"},
			want:        "/kefu/?channel=a+b%26c%3Dd",
			checkEscape: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.AccessPath(tt.ch)
			if got != tt.want {
				t.Errorf("AccessPath() = %q, 期望 %q", got, tt.want)
			}
			// 与 ChannelKey 的组合关系：路径恒为 "/kefu/?channel=" + QueryEscape(key)
			wantFromKey := "/kefu/?channel=" + url.QueryEscape(service.ChannelKey(tt.ch))
			if got != wantFromKey {
				t.Errorf("AccessPath() = %q, 与 channel 前缀 + QueryEscape(ChannelKey()) = %q 不一致", got, wantFromKey)
			}
			if tt.checkEscape {
				// 转义后不应再出现裸的 & 与 =，否则 query 参数会被截断
				key := got[len("/kefu/?channel="):]
				if key != url.QueryEscape(service.ChannelKey(tt.ch)) {
					t.Errorf("接入路径中的 channel 参数 key = %q, 期望转义结果 %q", key, url.QueryEscape(service.ChannelKey(tt.ch)))
				}
			}
		})
	}
}

func TestAccessURL(t *testing.T) {
	ch := &model.Channel{Model: model.Model{ID: 3}, Type: "wechat", AppID: "kefu_abc"}

	tests := []struct {
		name       string
		cfgBase    string // 全局配置 config.C.Server.PublicBase
		publicBase string // 调用参数
		want       string
	}{
		{
			name:    "publicBase 不带尾斜杠",
			cfgBase: "", publicBase: "https://kefu.example.com",
			want: "https://kefu.example.com/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 带一个尾斜杠：不会出现双斜杠",
			cfgBase: "", publicBase: "https://kefu.example.com/",
			want: "https://kefu.example.com/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 带多个尾斜杠：全部裁掉",
			cfgBase: "", publicBase: "https://kefu.example.com///",
			want: "https://kefu.example.com/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 带路径前缀（子目录部署）",
			cfgBase: "", publicBase: "https://example.com/shop/",
			want: "https://example.com/shop/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 带端口",
			cfgBase: "", publicBase: "http://127.0.0.1:8080/",
			want: "http://127.0.0.1:8080/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 为空：回退到全局配置的 public_base",
			cfgBase: "https://cfg.example.com", publicBase: "",
			want: "https://cfg.example.com/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 为空且全局配置带尾斜杠",
			cfgBase: "https://cfg.example.com/", publicBase: "",
			want: "https://cfg.example.com/kefu/?channel=kefu_abc",
		},
		{
			name:    "publicBase 与全局配置都为空：回退成相对路径",
			cfgBase: "", publicBase: "",
			want: "/kefu/?channel=kefu_abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPublicBase(t, tt.cfgBase)
			got := service.AccessURL(ch, tt.publicBase)
			if got != tt.want {
				t.Errorf("AccessURL(publicBase=%q, cfg=%q) = %q, 期望 %q", tt.publicBase, tt.cfgBase, got, tt.want)
			}
			// 不变式：完整地址必须以相对接入路径结尾，且不含 "//kefu"
			if path := service.AccessPath(ch); len(got) < len(path) || got[len(got)-len(path):] != path {
				t.Errorf("AccessURL() = %q 未以 AccessPath() = %q 结尾", got, path)
			}
			if len(got) >= 2 && containsDoubleSlashAfterScheme(got) {
				t.Errorf("AccessURL() = %q 出现了多余的连续斜杠", got)
			}
		})
	}
}

// containsDoubleSlashAfterScheme 判断 scheme:// 之后是否出现连续斜杠（"//"），
// 用于确认 TrimRight 没有把拼接处写成 "https://host//kefu"。
func containsDoubleSlashAfterScheme(s string) bool {
	idx := -1
	for i := 0; i+2 < len(s); i++ {
		if s[i] == ':' && s[i+1] == '/' && s[i+2] == '/' {
			idx = i + 3
			break
		}
	}
	if idx < 0 {
		return false
	}
	for i := idx; i+1 < len(s); i++ {
		if s[i] == '/' && s[i+1] == '/' {
			return true
		}
	}
	return false
}
