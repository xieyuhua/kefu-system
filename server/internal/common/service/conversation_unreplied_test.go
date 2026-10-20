package service

// 「未回复」判定的单元测试（同包测试，直接调用包内私有函数）。
//
// 契约：
//   - 会话列表上的「未回复 N 秒」= 会话最后一条消息是客户发的，起点就是这条消息的时间；
//   - 客服回复 / 机器人回复（direction=out）之后必须归零，否则回复完列表还在催；
//   - 系统提示（转人工、结束会话、离线等）既不算客户消息也不算客服回复，不影响判定。
//
// 特别注意：本仓库里客服自己发的消息 SenderRole 也是 "user"（见 SendMessage / seed），
// 所以「只看 SenderRole」会把客服回复误判成客户消息 —— 下面的用例专门守住这条。

import (
	"testing"
	"time"

	"kefu-server/internal/common/model"
)

func TestIsCustomerMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  *model.Message
		want bool
	}{
		{
			name: "客户消息：direction=in + role=customer",
			msg:  &model.Message{Direction: "in", SenderRole: "customer", Type: "text"},
			want: true,
		},
		{
			name: "客服回复：direction=out + role=user（role 也叫 user，不能只看 role）",
			msg:  &model.Message{Direction: "out", SenderRole: "user", SenderID: 7, Type: "text"},
			want: false,
		},
		{
			name: "机器人兜底回复：direction=out + role=user + sender_id=0",
			msg:  &model.Message{Direction: "out", SenderRole: "user", SenderID: 0, Type: "text"},
			want: false,
		},
		{
			name: "系统消息：direction=system",
			msg:  &model.Message{Direction: "system", SenderRole: "system", Type: "system"},
			want: false,
		},
		{
			name: "历史数据：direction=in 但 role 缺失，仍算客户消息",
			msg:  &model.Message{Direction: "in", Type: "text"},
			want: true,
		},
		{
			name: "历史数据：direction 缺失但 role=customer，按客户消息兜底",
			msg:  &model.Message{SenderRole: "customer", Type: "text"},
			want: true,
		},
		{
			name: "role=client 的渠道接入方同样算客户",
			msg:  &model.Message{SenderRole: "client", Type: "text"},
			want: true,
		},
		{
			name: "type=system 即使 direction=in 也不算客户消息",
			msg:  &model.Message{Direction: "in", SenderRole: "system", Type: "system"},
			want: false,
		},
		{
			name: "nil 消息不算客户消息",
			msg:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCustomerMessage(tt.msg); got != tt.want {
				t.Fatalf("IsCustomerMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnrepliedTS(t *testing.T) {
	created := time.Date(2024, 5, 6, 8, 9, 10, 0, time.UTC)
	wantMS := created.UnixMilli()

	tests := []struct {
		name string
		msg  *model.Message
		want int64
	}{
		{
			name: "最后一条是客户消息：返回该消息时间的 Unix 毫秒",
			msg:  &model.Message{Direction: "in", SenderRole: "customer", Type: "text", Model: model.Model{CreatedAt: created}},
			want: wantMS,
		},
		{
			name: "最后一条是客服回复：0（未回复标记消失）",
			msg:  &model.Message{Direction: "out", SenderRole: "user", Type: "text", Model: model.Model{CreatedAt: created}},
			want: 0,
		},
		{
			name: "最后一条是系统提示：0（系统提示不代表客户在等）",
			msg:  &model.Message{Direction: "system", SenderRole: "system", Type: "system", Model: model.Model{CreatedAt: created}},
			want: 0,
		},
		{
			name: "没有消息（nil）：0",
			msg:  nil,
			want: 0,
		},
		{
			name: "客户消息但时间零值：0（前端据此不显示，避免天文数字）",
			msg:  &model.Message{Direction: "in", SenderRole: "customer", Type: "text"},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unrepliedTS(tt.msg); got != tt.want {
				t.Fatalf("unrepliedTS() = %d, want %d", got, tt.want)
			}
		})
	}
}
