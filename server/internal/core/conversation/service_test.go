package conversation

// messagePreview 是包内私有函数，因此用同包测试（package conversation）直接调用。
//
// 契约：不同类型的消息在会话摘要里展示不同的占位文案；
// 图片 / 视频 / 文件 / 订单 / 商品 这些「非文本内容」统一显示中文标签，不把原始 JSON 塞进摘要；
// 文本与系统消息、未知类型则原样返回 Content。

import (
	"strings"
	"testing"
	"unicode/utf8"

	"kefu-server/internal/platform/model"
)

func TestMessagePreview(t *testing.T) {
	longText := strings.Repeat("客服您好，", 100) // 500 个字符，远超列表摘要常见长度

	tests := []struct {
		name string
		msg  *model.Message
		want string
	}{
		{
			name: "文本消息：原样返回内容",
			msg:  &model.Message{Type: "text", Content: "你好，请问在吗"},
			want: "你好，请问在吗",
		},
		{
			name: "文本消息：内容为空返回空串",
			msg:  &model.Message{Type: "text", Content: ""},
			want: "",
		},
		{
			name: "Type 为空（历史数据未写 type）按文本返回内容",
			msg:  &model.Message{Content: "只有内容没有 type"},
			want: "只有内容没有 type",
		},
		{
			name: "系统消息：原样返回内容",
			msg:  &model.Message{Type: "system", Content: "会话已被客服接待"},
			want: "会话已被客服接待",
		},
		{
			name: "图片消息：显示 [图片]，不透出原始 JSON",
			msg:  &model.Message{Type: "image", Content: `{"url":"https://cdn.example.com/a.png","w":800}`},
			want: "[图片]",
		},
		{
			name: "视频消息：显示 [视频]",
			msg:  &model.Message{Type: "video", Content: `{"url":"https://cdn.example.com/a.mp4"}`},
			want: "[视频]",
		},
		{
			name: "文件消息：显示 [文件]",
			msg:  &model.Message{Type: "file", Content: `{"url":"https://cdn.example.com/a.pdf","name":"合同.pdf"}`},
			want: "[文件]",
		},
		{
			name: "订单卡片：显示 [订单]",
			msg:  &model.Message{Type: "order", Content: `{"order_no":"SO20240101001","amount":199}`},
			want: "[订单]",
		},
		{
			name: "商品卡片：显示 [商品]",
			msg:  &model.Message{Type: "product", Content: `{"pid":"P10086","title":"春季外套"}`},
			want: "[商品]",
		},
		{
			name: "未知类型：按文本原样返回内容",
			msg:  &model.Message{Type: "voice", Content: "语音内容"},
			want: "语音内容",
		},
		{
			// 注意：当前实现没有截断逻辑，超长内容会原样写入 last_message（见汇报）。
			name: "超长文本：当前实现不截断，原样返回",
			msg:  &model.Message{Type: "text", Content: longText},
			want: longText,
		},
		{
			name: "多行文本：保留换行，不做压缩",
			msg:  &model.Message{Type: "text", Content: "第一行\n第二行"},
			want: "第一行\n第二行",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := messagePreview(tt.msg)
			if got != tt.want {
				t.Errorf("messagePreview(type=%q, content=%q) = %q, 期望 %q",
					tt.msg.Type, tt.msg.Content, got, tt.want)
			}
			if got != tt.want {
				return
			}
			// 非文本类型必须返回固定占位标签：既不能为空，也不能包含原始 JSON 的痕迹
			switch tt.msg.Type {
			case "image", "video", "file", "order", "product":
				if got == "" {
					t.Errorf("type=%q 的摘要不应为空", tt.msg.Type)
				}
				if strings.ContainsAny(got, "{}") {
					t.Errorf("type=%q 的摘要 %q 含 JSON 痕迹，应只显示占位文案", tt.msg.Type, got)
				}
				if !utf8.ValidString(got) {
					t.Errorf("type=%q 的摘要 %q 不是合法 UTF-8", tt.msg.Type, got)
				}
			}
		})
	}
}

// TestMessagePreviewLongTextRunes 单独固定「超长内容」的现状：
// 当前实现原样返回，字符数与输入完全一致（不是按字节截断，也没有省略号）。
func TestMessagePreviewLongTextRunes(t *testing.T) {
	const wantRunes = 500
	content := strings.Repeat("退", wantRunes)
	got := messagePreview(&model.Message{Type: "text", Content: content})

	if got != content {
		t.Fatalf("messagePreview 长文本 = %q（len=%d），期望与输入完全一致（len=%d）",
			got, len(got), len(content))
	}
	if n := utf8.RuneCountInString(got); n != wantRunes {
		t.Errorf("摘要字符数 = %d, 期望 %d", n, wantRunes)
	}
	if strings.Contains(got, "…") || strings.Contains(got, "...") {
		t.Errorf("摘要 %q 含省略号，与当前实现（不截断）不符", got)
	}
}
