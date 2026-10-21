package service

// 渠道作用域判定（MatchChannel / RuleChannelIDs / ParseChannelIDs / FormatChannelIDs）是包内纯函数，
// 因此用同包测试直接调用。
//
// 口径（改造点）：规则的渠道命中条件只有一套判定——
//   - ChannelIDs 非空：会话渠道 ID 在列表里才算命中；
//   - ChannelIDs 为空：回退旧的单渠道语义（ChannelID = 0 全渠道，>0 只匹配该渠道）。
//
// 匹配链路（MatchRule）通过 IsGlobalChannelRule + MatchChannel 复用它，
// 这里锁住纯函数本身的行为，避免「多选」与「旧单渠道」两套判断各自漂移。

import (
	"reflect"
	"testing"

	"kefu-server/internal/common/model"
)

func TestMatchChannel(t *testing.T) {
	tests := []struct {
		name       string
		channelID  uint   // 规则旧的单渠道字段（0 = 全渠道）
		channelIDs string // 规则多选渠道（逗号分隔）
		convID     uint   // 会话所在渠道
		want       bool
	}{
		// ---------- 多选（ChannelIDs 非空） ----------
		{name: "多选：命中列表中的第一个渠道", channelIDs: "3,7", convID: 3, want: true},
		{name: "多选：命中列表中的第二个渠道", channelIDs: "3,7", convID: 7, want: true},
		{name: "多选：不在列表里的渠道不命中", channelIDs: "3,7", convID: 9, want: false},
		{name: "多选：单个渠道（等价旧写法）命中", channelIDs: "5", convID: 5, want: true},
		{name: "多选：单个渠道不命中其它渠道", channelIDs: "5", convID: 6, want: false},
		{name: "多选优先于旧单渠道字段：列表不含会话渠道时不命中", channelID: 3, channelIDs: "7,8", convID: 3, want: false},
		{name: "多选优先于旧单渠道字段：列表含会话渠道时命中", channelID: 9, channelIDs: "3,7", convID: 7, want: true},
		{name: "多选串含空格与空段仍可解析", channelIDs: " 3 , ,7 ", convID: 7, want: true},

		// ---------- 旧语义（ChannelIDs 为空） ----------
		{name: "旧语义：channel_id = 0 全渠道命中任意渠道", channelID: 0, convID: 42, want: true},
		{name: "旧语义：channel_id = 5 命中同渠道", channelID: 5, convID: 5, want: true},
		{name: "旧语义：channel_id = 5 不命中别的渠道", channelID: 5, convID: 6, want: false},
		{name: "旧语义：只有逗号等空段视为未配置 → 全渠道", channelIDs: " , ", convID: 42, want: true},
		{name: "旧语义：非法渠道串视为未配置 → 全渠道", channelIDs: "abc", convID: 42, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &model.AutoReply{ChannelID: tt.channelID, ChannelIDs: tt.channelIDs}
			if got := MatchChannel(r, tt.convID); got != tt.want {
				t.Errorf("MatchChannel(channel_id=%d, channel_ids=%q, conv=%d) = %v, 期望 %v",
					tt.channelID, tt.channelIDs, tt.convID, got, tt.want)
			}
		})
	}
}

// TestMatchChannelNilRule 空规则不能 panic，也不能把所有渠道都算命中。
func TestMatchChannelNilRule(t *testing.T) {
	if MatchChannel(nil, 3) {
		t.Error("MatchChannel(nil, 3) = true, 期望 false")
	}
	if ids := RuleChannelIDs(nil); len(ids) != 0 {
		t.Errorf("RuleChannelIDs(nil) = %v, 期望空列表", ids)
	}
}

// TestRuleChannelIDs 对外展示用的渠道列表：多选优先，没有多选时回退旧单渠道字段。
func TestRuleChannelIDs(t *testing.T) {
	tests := []struct {
		name       string
		channelID  uint
		channelIDs string
		want       []uint
	}{
		{name: "多选列表原样返回", channelIDs: "3,7", want: []uint{3, 7}},
		{name: "多选去重", channelIDs: "7,7,3", want: []uint{7, 3}},
		{name: "多选丢弃 0", channelIDs: "0,3", want: []uint{3}},
		{name: "旧单渠道回退成单元素列表", channelID: 5, want: []uint{5}},
		{name: "旧全渠道（0）返回空列表", channelID: 0, want: []uint{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &model.AutoReply{ChannelID: tt.channelID, ChannelIDs: tt.channelIDs}
			if got := RuleChannelIDs(r); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RuleChannelIDs(channel_id=%d, channel_ids=%q) = %v, 期望 %v",
					tt.channelID, tt.channelIDs, got, tt.want)
			}
		})
	}
}

// TestIsGlobalChannelRule 全渠道规则的判定口径：多选为空且旧字段为 0。
func TestIsGlobalChannelRule(t *testing.T) {
	tests := []struct {
		name       string
		channelID  uint
		channelIDs string
		want       bool
	}{
		{name: "全渠道（两个字段都空）", want: true},
		{name: "旧单渠道规则不算全渠道", channelID: 5, want: false},
		{name: "多选规则不算全渠道", channelIDs: "3,7", want: false},
		{name: "只有空白的多选串等于未配置", channelIDs: "  ", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &model.AutoReply{ChannelID: tt.channelID, ChannelIDs: tt.channelIDs}
			if got := IsGlobalChannelRule(r); got != tt.want {
				t.Errorf("IsGlobalChannelRule(channel_id=%d, channel_ids=%q) = %v, 期望 %v",
					tt.channelID, tt.channelIDs, got, tt.want)
			}
		})
	}
}

// TestFormatChannelIDs 落库格式：去重、丢弃 0、升序、空列表落空串（= 全渠道）。
func TestFormatChannelIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []uint
		want string
	}{
		{name: "单个渠道", in: []uint{5}, want: "5"},
		{name: "多个渠道按 ID 升序", in: []uint{7, 3}, want: "3,7"},
		{name: "去重", in: []uint{3, 7, 3}, want: "3,7"},
		{name: "丢弃 0（0 不是真实渠道）", in: []uint{0, 4}, want: "4"},
		{name: "只有 0 视为全渠道", in: []uint{0}, want: ""},
		{name: "空列表视为全渠道", in: []uint{}, want: ""},
		{name: "nil 视为全渠道", in: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatChannelIDs(tt.in); got != tt.want {
				t.Errorf("FormatChannelIDs(%v) = %q, 期望 %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseChannelIDs 解析落库串：跳过空段与非法值，保持顺序去重。
func TestParseChannelIDs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []uint
	}{
		{name: "逗号分隔", raw: "3,7", want: []uint{3, 7}},
		{name: "带空格", raw: " 3 , 7 ", want: []uint{3, 7}},
		{name: "跳过空段", raw: "3,,7,", want: []uint{3, 7}},
		{name: "跳过非法值", raw: "3,abc,7", want: []uint{3, 7}},
		{name: "丢弃 0", raw: "0,3", want: []uint{3}},
		{name: "空串", raw: "", want: []uint{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseChannelIDs(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseChannelIDs(%q) = %v, 期望 %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestFormatParseRoundTrip 落库串与解析结果互为逆运算（多选渠道的存储口径自洽）。
func TestFormatParseRoundTrip(t *testing.T) {
	for _, ids := range [][]uint{{3}, {3, 7}, {1, 2, 3, 4, 5}} {
		stored := FormatChannelIDs(ids)
		parsed := ParseChannelIDs(stored)
		if !reflect.DeepEqual(parsed, ids) {
			t.Errorf("FormatChannelIDs(%v) = %q → ParseChannelIDs = %v, 期望 %v", ids, stored, parsed, ids)
		}
	}
}
