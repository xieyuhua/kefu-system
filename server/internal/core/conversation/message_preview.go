package conversation

import (
	"unicode/utf8"

	"kefu-server/internal/platform/model"
)

// previewMaxRunes 会话摘要的最大字符数（按 rune 计）。
//
// 背景：conversations.last_message 字段是 varchar(512)，而客户消息本身没有长度限制。
// 若不截断，客户发一条 >512 字节的消息后 SaveConversation 会在 MySQL 严格模式下
// 报 1406（Data too long），接口 500 —— 此时消息已经入库，用户重发就产生重复消息。
// 100 个中文约 300 字节、100 个 emoji 约 400 字节，加上省略号（3 字节）都远小于 512 字节。
const previewMaxRunes = 100

// previewEllipsis 超长预览的截断标记（U+2026，UTF-8 编码 3 字节）。
const previewEllipsis = "…"

// messagePreview 生成会话摘要（last_message）的展示文案，并按 rune 截断到 previewMaxRunes。
//
// 所有消息类型都必须走同一条出口：非文本类型先取占位文案，文本 / 系统消息取 Content，
// 最后统一截断，避免任何一类消息绕过长度限制。
func messagePreview(m *model.Message) string {
	return truncatePreview(previewContent(m))
}

// previewContent 按消息类型取原始预览文案（不含截断逻辑）。
func previewContent(m *model.Message) string {
	switch m.Type {
	case "image":
		return "[图片]"
	case "video":
		return "[视频]"
	case "file":
		return "[文件]"
	case "order":
		return "[订单]"
	case "product":
		return "[商品]"
	case "system":
		return m.Content
	default:
		return m.Content
	}
}

// truncatePreview 把预览文案按 rune 截断到 previewMaxRunes，
// 超出部分丢弃并以 previewEllipsis 结尾；未超长时原样返回（不追加省略号）。
func truncatePreview(s string) string {
	if utf8.RuneCountInString(s) <= previewMaxRunes {
		return s
	}
	// 先转 []rune 再切片，保证不会把一个多字节字符切成半个（中文 / emoji 都安全）。
	runes := []rune(s)
	return string(runes[:previewMaxRunes]) + previewEllipsis
}
