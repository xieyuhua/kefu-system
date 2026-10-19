package service

// messagePreview 是包内私有函数，因此用同包测试（package service）直接调用。
//
// 契约：不同类型的消息在会话摘要里展示不同的占位文案；
// 图片 / 视频 / 文件 / 订单 / 商品 这些「非文本内容」统一显示中文标签，不把原始 JSON 塞进摘要；
// 文本与系统消息、未知类型则取 Content。
// 所有类型最后都走同一条出口 truncatePreview：超过 100 个字符时截断到 100 个字符 + 「…」，
// 保证写进 conversations.last_message（varchar(512)）时不会超长。

import (
	"strings"
	"testing"
	"unicode/utf8"

	"kefu-server/internal/common/model"
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
			// 修复点：超长内容必须截断到 100 个字符 + 「…」，
			// 否则 SaveConversation 写 varchar(512) 的 last_message 会 1406 报错。
			name: "超长文本：截断到 100 个字符 + …",
			msg:  &model.Message{Type: "text", Content: longText},
			want: strings.Repeat("客服您好，", 20) + "…",
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

// TestMessagePreviewLongTextRunes 固定「超长内容」的行为：
// 按 rune 截断到 100 个字符，再追加省略号「…」，总长 101 个字符。
// 重点是不能按字节截断——500 个中文若按字节切会把最后一个字切成半个非法 UTF-8。
func TestMessagePreviewLongTextRunes(t *testing.T) {
	const wantRunes = 500
	content := strings.Repeat("退", wantRunes)
	got := messagePreview(&model.Message{Type: "text", Content: content})

	if n := utf8.RuneCountInString(got); n != previewMaxRunes+1 {
		t.Fatalf("摘要字符数 = %d, 期望 %d（%d 个字符 + 1 个省略号）",
			n, previewMaxRunes+1, previewMaxRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("摘要 %q 未以省略号结尾", got)
	}
	if want := strings.Repeat("退", previewMaxRunes) + "…"; got != want {
		t.Errorf("摘要 = %q, 期望 %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Errorf("截断后的摘要不是合法 UTF-8: %q", got)
	}
	// 落库安全：截断后的字节数必须远小于 conversations.last_message 的 512 字节
	if len(got) > 512 {
		t.Errorf("摘要字节数 = %d, 超过 last_message 的 512 字节上限", len(got))
	}
}

// TestMessagePreviewTruncateBoundary 边界：正好 100 个字符不截断、101 个字符截断到 100 +「…」。
func TestMessagePreviewTruncateBoundary(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		want       string
		wantRunes  int
		wantSuffix bool // 是否应以省略号结尾
	}{
		{
			name:       "正好 100 个字符：原样返回，不加省略号",
			content:    strings.Repeat("好", previewMaxRunes),
			want:       strings.Repeat("好", previewMaxRunes),
			wantRunes:  previewMaxRunes,
			wantSuffix: false,
		},
		{
			name:       "101 个字符：截断到 100 + 省略号",
			content:    strings.Repeat("好", previewMaxRunes) + "多",
			want:       strings.Repeat("好", previewMaxRunes) + "…",
			wantRunes:  previewMaxRunes + 1,
			wantSuffix: true,
		},
		{
			name:       "99 个字符：原样返回，不加省略号",
			content:    strings.Repeat("好", previewMaxRunes-1),
			want:       strings.Repeat("好", previewMaxRunes-1),
			wantRunes:  previewMaxRunes - 1,
			wantSuffix: false,
		},
		{
			name:       "emoji（4 字节 / 字符）101 个：同样按 rune 截断，字节数不超过 512",
			content:    strings.Repeat("😀", previewMaxRunes) + "😀",
			want:       strings.Repeat("😀", previewMaxRunes) + "…",
			wantRunes:  previewMaxRunes + 1,
			wantSuffix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := messagePreview(&model.Message{Type: "text", Content: tt.content})
			if got != tt.want {
				t.Fatalf("messagePreview = %q, 期望 %q", got, tt.want)
			}
			if n := utf8.RuneCountInString(got); n != tt.wantRunes {
				t.Errorf("摘要字符数 = %d, 期望 %d", n, tt.wantRunes)
			}
			if suffix := strings.HasSuffix(got, "…"); suffix != tt.wantSuffix {
				t.Errorf("摘要以省略号结尾 = %v, 期望 %v", suffix, tt.wantSuffix)
			}
			if !utf8.ValidString(got) {
				t.Errorf("摘要不是合法 UTF-8: %q", got)
			}
			if len(got) > 512 {
				t.Errorf("摘要字节数 = %d, 超过 512 字节上限", len(got))
			}
		})
	}
}

// TestMessagePreviewAllTypesTruncated 所有消息类型都必须走同一条截断出口：
// 占位文案本身很短（不受影响），但任何类型都不允许把超长原文塞进 last_message。
func TestMessagePreviewAllTypesTruncated(t *testing.T) {
	long := strings.Repeat("很长的内容", 200) // 1000 个字符
	for _, typ := range []string{"", "text", "system", "voice", "image", "video", "file", "order", "product"} {
		m := &model.Message{Type: typ, Content: long}
		got := messagePreview(m)
		if n := utf8.RuneCountInString(got); n > previewMaxRunes+1 {
			t.Errorf("type=%q 的摘要字符数 = %d, 超过上限 %d", typ, n, previewMaxRunes+1)
		}
		if len(got) > 512 {
			t.Errorf("type=%q 的摘要字节数 = %d, 超过 512 字节上限", typ, len(got))
		}
		if got == "" {
			t.Errorf("type=%q 的摘要为空", typ)
		}
	}
}
