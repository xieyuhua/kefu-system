package service

import (
	"strings"

	"kefu-server/internal/common/model"
)

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

// robotAnswer 渠道开启机器人且未命中规则时的兜底话术。
// 原实现在 service/demo.go，只有 ReplyNow 使用，因此随判定逻辑一起落在本模块。
func robotAnswer(q string) string {
	switch {
	case strings.Contains(q, "发货") || strings.Contains(q, "物流"):
		return "您好，订单会在付款后 48 小时内发出，物流单号可在「我的订单」中查看哦～"
	case strings.Contains(q, "退款") || strings.Contains(q, "退货"):
		return "支持 7 天无理由退货，请在订单详情页点击「申请退款」，我们会尽快为您处理。"
	case strings.Contains(q, "价格") || strings.Contains(q, "优惠"):
		return "当前商品正在参与春季促销，满 199 减 30，欢迎选购～"
	case strings.Contains(q, "你好") || strings.Contains(q, "在吗"):
		return "您好，我是智能客服助手，很高兴为您服务。请问有什么可以帮您？"
	default:
		return "您的问题我已记录，稍后将由人工客服为您详细解答～"
	}
}
