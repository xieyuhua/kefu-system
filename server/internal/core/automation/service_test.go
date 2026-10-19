package automation

// 自动回复关键词判定（matchKeywords / defaultMatchType）是包内私有函数，
// 因此用同包测试直接调用。
//
// 调用约定（与 ReplyNow 一致）：matchKeywords 收到的 lowerQ 已经是小写化的客户问题，
// 关键词在函数内部做 TrimSpace + ToLower，因此「大小写不敏感」只对关键词一侧生效。

import (
	"testing"

	"kefu-server/internal/platform/model"
)

func TestMatchKeywords(t *testing.T) {
	tests := []struct {
		name      string
		keywords  string
		matchType string
		query     string // 模拟已小写化的客户提问
		want      bool
	}{
		// ---------- 精确匹配 ----------
		{
			name:     "精确匹配：完全一致命中",
			keywords: "退款", matchType: "equal", query: "退款", want: true,
		},
		{
			name:     "精确匹配：提问多出字不命中",
			keywords: "退款", matchType: "equal", query: "我要退款", want: false,
		},
		{
			name:     "精确匹配：多关键词中命中第二个",
			keywords: "发票,退款", matchType: "equal", query: "退款", want: true,
		},

		// ---------- 包含匹配 ----------
		{
			name:     "包含匹配：问题里含关键词命中",
			keywords: "退款", matchType: "contain", query: "我要退款怎么操作", want: true,
		},
		{
			name:     "包含匹配：不含关键词不命中",
			keywords: "发票", matchType: "contain", query: "我要退款", want: false,
		},
		{
			name:     "包含匹配：关键词比问题更长不命中",
			keywords: "退款流程说明", matchType: "contain", query: "退款", want: false,
		},

		// ---------- 前缀匹配 ----------
		{
			name:     "前缀匹配：问题以关键词开头命中",
			keywords: "退", matchType: "prefix", query: "退款流程是什么", want: true,
		},
		{
			name:     "前缀匹配：关键词出现在中间不命中",
			keywords: "退款", matchType: "prefix", query: "我要退款", want: false,
		},

		// ---------- 大小写 ----------
		{
			name:     "大小写：关键词全大写、问题小写时命中",
			keywords: "REFUND", matchType: "contain", query: "i want refund now", want: true,
		},
		{
			name:     "大小写：精确匹配下也按小写比较",
			keywords: "REFUND", matchType: "equal", query: "refund", want: true,
		},
		{
			name:     "大小写：关键词全大写但问题含大写不命中（调用方负责小写化）",
			keywords: "refund", matchType: "contain", query: "I want REFUND", want: false,
		},

		// ---------- 关键词切分与空白 ----------
		{
			name:     "多关键词：逗号分隔，命中任意一个即返回 true",
			keywords: "物流,发货,快递", matchType: "contain", query: "什么时候发货", want: true,
		},
		{
			name:     "关键词两侧空白与空段被忽略",
			keywords: " , 退货 ,, ", matchType: "contain", query: "我要申请退货", want: true,
		},
		{
			name:     "空关键词串：不命中",
			keywords: "", matchType: "contain", query: "随便问问", want: false,
		},
		{
			name:     "只有逗号与空段：不命中",
			keywords: " , ,, ", matchType: "contain", query: "随便问问", want: false,
		},
		{
			name:     "关键词为空串但匹配类型为精确：不命中（空串不与任何问题相等）",
			keywords: "", matchType: "equal", query: "", want: false,
		},

		// ---------- 匹配类型兜底 ----------
		{
			name:     "MatchType 为空：按包含处理（命中）",
			keywords: "物流", matchType: "", query: "物流到哪了", want: true,
		},
		{
			name:     "未知匹配类型 regex：兜底为包含（命中）",
			keywords: "退款", matchType: "regex", query: "我要退款", want: true,
		},
		{
			name:     "未知匹配类型：兜底为包含（不命中）",
			keywords: "退款", matchType: "fuzzy", query: "发票怎么开", want: false,
		},
		{
			// 现状说明：实现里没有正则引擎，meta 字符按普通字符参与包含匹配。
			name:     "正则写法不被解释：'退.*款' 匹配不到 '退款'",
			keywords: "退.*款", matchType: "regex", query: "退款", want: false,
		},
		{
			name:     "MatchType 大小写敏感：'EQUAL' 视为未知类型，按包含处理",
			keywords: "退款", matchType: "EQUAL", query: "我要退款", want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &model.AutoReply{Keywords: tt.keywords, MatchType: tt.matchType}
			got := matchKeywords(r, tt.query)
			if got != tt.want {
				t.Errorf("matchKeywords(keywords=%q, matchType=%q, query=%q) = %v, 期望 %v",
					tt.keywords, tt.matchType, tt.query, got, tt.want)
			}
		})
	}
}

// TestMatchKeywordsEmptyRuleNoPanic 规则字段全空时不能 panic，
// 且必须返回 false（不会把每条客户消息都当成命中）。
func TestMatchKeywordsEmptyRuleNoPanic(t *testing.T) {
	for i, r := range []*model.AutoReply{
		{},
		{Keywords: ""},
		{Keywords: " , ", MatchType: ""},
		{Keywords: ",,,", MatchType: "equal"},
	} {
		if got := matchKeywords(r, "任意提问"); got {
			t.Errorf("第 %d 个空规则返回 true，期望 false", i)
		}
	}
}

// TestMatchKeywordsMultipleKeywordsOrderIndependent 命中与否不应依赖关键词的书写顺序。
func TestMatchKeywordsMultipleKeywordsOrderIndependent(t *testing.T) {
	q := "请问什么时候发货"
	a := &model.AutoReply{Keywords: "物流,发货", MatchType: "contain"}
	b := &model.AutoReply{Keywords: "发货,物流", MatchType: "contain"}

	gotA, gotB := matchKeywords(a, q), matchKeywords(b, q)
	if gotA != true || gotB != true {
		t.Fatalf("两种关键词顺序的匹配结果 = (%v, %v), 期望都为 true", gotA, gotB)
	}
}

func TestDefaultMatchType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "equal 保留", in: "equal", want: "equal"},
		{name: "prefix 保留", in: "prefix", want: "prefix"},
		{name: "contain 保留", in: "contain", want: "contain"},
		{name: "空串兜底为 contain", in: "", want: "contain"},
		{name: "未知类型（regex）兜底为 contain", in: "regex", want: "contain"},
		{name: "未知类型（fuzzy）兜底为 contain", in: "fuzzy", want: "contain"},
		{name: "大小写敏感：EQUAL 不被识别，兜底为 contain", in: "EQUAL", want: "contain"},
		{name: "带空格的 contain 不被识别，兜底为 contain", in: " contain ", want: "contain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultMatchType(tt.in)
			if got != tt.want {
				t.Errorf("defaultMatchType(%q) = %q, 期望 %q", tt.in, got, tt.want)
			}
			// 输出必须始终是可执行的分支之一，保证 switch 里有对应的判定逻辑
			if got != "equal" && got != "prefix" && got != "contain" {
				t.Errorf("defaultMatchType(%q) = %q, 不在 equal/prefix/contain 之内", tt.in, got)
			}
		})
	}
}

// TestDefaultMatchTypeIdempotent 幂等性：重复调用结果不变（可安全用于落库前的规整）。
func TestDefaultMatchTypeIdempotent(t *testing.T) {
	for _, in := range []string{"equal", "prefix", "contain", "", "regex"} {
		once := defaultMatchType(in)
		twice := defaultMatchType(once)
		if once != twice {
			t.Errorf("defaultMatchType 不幂等：in=%q once=%q twice=%q", in, once, twice)
		}
	}
}
