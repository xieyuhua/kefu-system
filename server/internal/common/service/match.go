package service

import (
	"sort"
	"strconv"
	"strings"

	"kefu-server/internal/common/model"
)

// ---------------- 规则的渠道作用域（多选 + 旧单渠道语义） ----------------
//
// 命中条件只由 MatchChannel 判定：多选列表（ChannelIDs）非空时看列表，
// 为空时回退到旧的单渠道字段（ChannelID = 0 表示全渠道）。
// 匹配 / 触发链路不允许出现第二套渠道判断，改口径只改这里。

// channelIDSeparator 落库时多渠道 ID 的分隔符（与关键词的「逗号分隔」风格保持一致，
// 但只认半角逗号：这里的值由后端生成，不存在人工输入全角标点的场景）。
const channelIDSeparator = ","

// ParseChannelIDs 把逗号分隔的渠道 ID 串解析成 ID 列表：跳过空段与非法数字，
// 丢弃 0（0 不是真实渠道，代表「全渠道」），并按首次出现顺序去重。
func ParseChannelIDs(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []uint{}
	}
	ids := make([]uint, 0, 4)
	seen := make(map[uint]bool, 4)
	for _, seg := range strings.Split(raw, channelIDSeparator) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		n, err := strconv.ParseUint(seg, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		id := uint(n)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// FormatChannelIDs 把渠道 ID 列表规整成落库用的逗号分隔串：
// 丢弃 0、去重、按 ID 升序（同一条规则的落库结果与前端选择顺序无关）。
func FormatChannelIDs(ids []uint) string {
	clean := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return ""
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i] < clean[j] })
	parts := make([]string, 0, len(clean))
	for _, id := range clean {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, channelIDSeparator)
}

// RuleChannelIDs 规则实际生效的渠道 ID 列表（对外展示与逐条比对都用它）：
//   - ChannelIDs 非空 → 解析后的列表；
//   - ChannelIDs 为空 → 回退旧语义：ChannelID > 0 时只有这一个渠道，ChannelID = 0 表示全渠道 → 空列表。
func RuleChannelIDs(rule *model.AutoReply) []uint {
	if rule == nil {
		return []uint{}
	}
	if ids := ParseChannelIDs(rule.ChannelIDs); len(ids) > 0 {
		return ids
	}
	if rule.ChannelID > 0 {
		return []uint{rule.ChannelID}
	}
	return []uint{}
}

// IsGlobalChannelRule 规则是否「全渠道」（任何渠道的会话都适用）：
// 多选列表为空且旧的单渠道字段也是 0。
func IsGlobalChannelRule(rule *model.AutoReply) bool {
	return len(RuleChannelIDs(rule)) == 0
}

// MatchChannel 规则是否适用于指定渠道（纯函数，匹配 / 触发链路统一走它）：
//   - ChannelIDs 非空 → 会话渠道 ID 在列表里；
//   - ChannelIDs 为空 → 沿用旧逻辑：ChannelID == 0 全渠道，或 ChannelID == 会话渠道 ID。
func MatchChannel(rule *model.AutoReply, channelID uint) bool {
	if rule == nil {
		return false
	}
	if ids := ParseChannelIDs(rule.ChannelIDs); len(ids) > 0 {
		for _, id := range ids {
			if id == channelID {
				return true
			}
		}
		return false
	}
	if rule.ChannelID == 0 {
		return true
	}
	return rule.ChannelID == channelID
}

// keywordSeparators 关键词分隔符集合：半角/全角逗号、半角/全角分号与换行。
//
// 后台的关键词输入框是自由文本，中文输入法打出来的是全角「，」「；」，
// 只按半角逗号切分会把整串当成一个关键词（配置「你好，在吗」永远匹配不到），
// 这里统一把四种分隔符 + 换行都当作切分点。
const keywordSeparators = ",，;；\n\r"

// splitKeywords 按 keywordSeparators 把关键词配置切成独立关键词：
// 连续分隔符产生的空段由 strings.FieldsFunc 自动丢弃，每段再做 TrimSpace（由调用方执行）。
func splitKeywords(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return strings.ContainsRune(keywordSeparators, r)
	})
}

func matchKeywords(r *model.AutoReply, lowerQ string) bool {
	for _, kw := range splitKeywords(r.Keywords) {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw == "" {
			continue
		}
		switch r.MatchType {
		case "equal":
			if lowerQ == kw {
				return true
			}
		case "prefix":
			if strings.HasPrefix(lowerQ, kw) {
				return true
			}
		default: // contain
			if strings.Contains(lowerQ, kw) {
				return true
			}
		}
	}
	return false
}

func defaultMatchType(s string) string {
	switch s {
	case "equal", "prefix", "contain":
		return s
	}
	return "contain"
}

func orDefaultStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// ---------------- 关键词话术：已迁出代码 ----------------
//
// 历史背景：这里曾有一个 robotAnswer(q) 函数，用 if-else 写死 4 组关键词话术
// （发货/物流、退款/退货、价格/优惠、你好/在吗），未命中时返回空串交给兜底话术。
//
// 现在这些内置话术已经「从代码搬到数据」：pkg/seed 把它们灌成租户的自动回复规则
// （match_type = contain、channel_id = 0 全渠道、user_id = 0 公共规则、enabled = true），
// 管理员 / 客服可以直接在后台「自动回复」页编辑，改完立即生效。
//
// 因此本文件只保留判定用的纯函数契约：
//   - 命中规则 → 用规则里的 reply（MatchRule 负责）；
//   - 未命中 → 交给「AI 应答（可选）→ 兜底话术 robot.fallback_reply」这条链路（ReplyNow 负责）。
//
// 这里刻意不再保留「未命中返回空串」的占位函数：留下一个永远返回空串的函数，
// 只会给人「关键词话术还在代码里」的错觉。
