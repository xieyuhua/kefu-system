// Package automation 自动回复模块：关键词规则 → 机器人回复 / 转人工。
// 模块内按 handler / service / repository 分层。
//
// 依赖方向：automation → chat（落系统提示语、通知渠道客服、广播会话）、channel（渠道机器人开关）。
// 跨模块调用：全部走 chat / channel 导出的函数，不直接读写对方的表。
package automation

import (
	"errors"
	"strings"
	"time"

	"kefu-server/internal/modules/chat"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/util"
)

// ---------------- 自动回复规则 ----------------
// 关键词命中 → 机器人自动回复；若该规则标记为"转人工"，则直接把会话转为待人工接待。

type AutoReplyVO struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	ChannelID       uint   `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	MatchType       string `json:"match_type"` // contain | equal | prefix
	Keywords        string `json:"keywords"`   // 逗号分隔
	Reply           string `json:"reply"`
	ReplyType       string `json:"reply_type"` // text | product
	TransferToHuman bool   `json:"transfer_to_human"`
	Enabled         bool   `json:"enabled"`
	Priority        int    `json:"priority"`
	HitCount        int64  `json:"hit_count"`
	CreatedAt       string `json:"created_at"`
}

// AutoReplyRequest 复用 dto 定义（含 HasTransfer 判定）
type AutoReplyRequest = dto.AutoReplyRequest

func ListAutoReplies(tenantID uint) ([]AutoReplyVO, error) {
	list, err := ListAutoRepliesByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]AutoReplyVO, 0, len(list))
	for _, r := range list {
		out = append(out, toAutoReplyVO(r))
	}
	return out, nil
}

func toAutoReplyVO(r *model.AutoReply) AutoReplyVO {
	vo := AutoReplyVO{
		ID: r.ID, Name: r.Name, ChannelID: r.ChannelID,
		MatchType: r.MatchType, Keywords: r.Keywords, Reply: r.Reply,
		ReplyType: r.ReplyType, TransferToHuman: r.TransferToHuman,
		Enabled: r.Enabled, Priority: r.Priority, HitCount: r.HitCount,
		CreatedAt: util.FmtTime(r.CreatedAt), ChannelName: "全渠道",
	}
	if r.ChannelID > 0 {
		// 渠道名查不到时保留"全渠道"兜底文案，不影响规则列表展示（与原逻辑一致）
		if ch, err := FindAutoReplyChannel(r.ChannelID); err == nil {
			vo.ChannelName = ch.Name
		}
	}
	if vo.ReplyType == "" {
		vo.ReplyType = "text"
	}
	if vo.MatchType == "" {
		vo.MatchType = "contain"
	}
	return vo
}

func CreateAutoReply(tenantID uint, req AutoReplyRequest) (*AutoReplyVO, error) {
	if strings.TrimSpace(req.Keywords) == "" {
		return nil, errors.New("关键词不能为空")
	}
	if !req.HasTransfer() && strings.TrimSpace(req.Reply) == "" {
		return nil, errors.New("回复内容不能为空（转人工规则可留空）")
	}
	r := model.AutoReply{
		TenantID: tenantID, Name: req.Name, ChannelID: req.ChannelID,
		MatchType: defaultMatchType(req.MatchType), Keywords: req.Keywords,
		Reply: req.Reply, ReplyType: orDefaultStr(req.ReplyType, "text"),
		Enabled: true, Priority: req.Priority,
	}
	if req.TransferToHuman != nil {
		r.TransferToHuman = *req.TransferToHuman
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if err := InsertAutoReply(&r); err != nil {
		return nil, err
	}
	vo := toAutoReplyVO(&r)
	return &vo, nil
}

func UpdateAutoReply(tenantID, id uint, req AutoReplyRequest) error {
	r, err := FindAutoReplyByID(tenantID, id)
	if err != nil {
		return errors.New("规则不存在")
	}
	if req.Name != "" {
		r.Name = req.Name
	}
	if req.ChannelID > 0 || req.ChannelID == 0 && req.Name == "" {
		r.ChannelID = req.ChannelID
	}
	if req.MatchType != "" {
		r.MatchType = req.MatchType
	}
	if req.Keywords != "" {
		r.Keywords = req.Keywords
	}
	if req.Reply != "" {
		r.Reply = req.Reply
	}
	if req.ReplyType != "" {
		r.ReplyType = req.ReplyType
	}
	if req.TransferToHuman != nil {
		r.TransferToHuman = *req.TransferToHuman
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	r.Priority = req.Priority
	return SaveAutoReply(r)
}

func DeleteAutoReply(tenantID, id uint) error {
	return RemoveAutoReply(tenantID, id)
}

// MatchRule 按优先级找出命中的规则（优先匹配指定渠道的规则，其次全渠道规则）
func MatchRule(tenantID, channelID uint, question string) (*model.AutoReply, bool) {
	if strings.TrimSpace(question) == "" {
		return nil, false
	}
	// 规则读取失败按「没有规则」处理，不影响消息主流程（与原逻辑一致）
	rules, _ := ListEnabledAutoReplies(tenantID)
	q := strings.ToLower(strings.TrimSpace(question))

	// 先渠道专属，再全渠道
	for _, scope := range []uint{channelID, 0} {
		for _, r := range rules {
			if r.ChannelID != scope {
				continue
			}
			if matchKeywords(r, q) {
				r.HitCount++
				// 命中次数回写失败不阻断自动回复（与原逻辑一致）
				_ = UpdateAutoReplyHitCount(r.ID, r.HitCount)
				return r, true
			}
		}
	}
	return nil, false
}

// ReplyResult 自动回复判定结果
type ReplyResult struct {
	Transferred bool        // 是否命中转人工
	Message     interface{} // 机器人回复的消息 VO（可能为 nil）
}

// ReplyNow 同步执行一次自动回复判定（用户端接口即时返回用）
func ReplyNow(tenantID, convID, customerID, channelID uint, question string) *ReplyResult {
	conv, err := FindAutoReplyConversation(tenantID, convID)
	if err != nil {
		return nil
	}
	if conv.AgentID > 0 && conv.Status == "open" {
		return &ReplyResult{Transferred: false}
	}

	rule, matched := MatchRule(tenantID, channelID, question)

	if matched && rule.TransferToHuman {
		desc := ""
		if rule.Name != "" {
			desc = "（命中：" + rule.Name + "）"
		}
		chat.AppendSystemMessage(tenantID, convID, "客户请求人工客服"+desc+"，正在为您转接，请稍候")
		// 转人工：置为待接待并清空接待人（失败不阻断，后续由客服端手动接入）
		_ = UpdateAutoReplyConversation(convID, map[string]interface{}{
			"status": "pending", "agent_id": 0,
		})
		chat.NotifyChannelAgents(tenantID, channelID, convID, "待接待提醒", "有客户请求人工客服，请尽快接入")
		chat.BroadcastConversation(tenantID, convID)
		realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{
			Event: "conversation.updated",
			Data:  map[string]interface{}{"id": convID, "status": "pending", "transferred": true},
		})
		return &ReplyResult{Transferred: true}
	}

	var reply string
	if matched {
		reply = rule.Reply
	} else {
		if ch, err := FindAutoReplyChannel(channelID); err == nil && ch.RobotEnabled {
			reply = robotAnswer(question)
		}
	}
	if reply == "" {
		return &ReplyResult{Transferred: false}
	}
	m := model.Message{
		TenantID: tenantID, ConversationID: convID,
		SenderRole: "user", SenderID: 0,
		Type: "text", Direction: "out", Content: reply, Read: true,
	}
	if err := CreateAutoReplyMessage(&m); err != nil {
		return &ReplyResult{Transferred: false}
	}
	// 回写会话摘要（失败不阻断，消息本身已入库）
	_ = UpdateAutoReplyConversation(convID, map[string]interface{}{
		"last_message": reply, "last_message_at": time.Now(),
	})
	vo := chat.BuildMessageVO(&m)
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "message.new", Data: vo})
	realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{Event: "message.new", Data: vo})
	return &ReplyResult{Transferred: false, Message: vo}
}

func matchKeywords(r *model.AutoReply, lowerQ string) bool {
	for _, kw := range strings.Split(r.Keywords, ",") {
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
