package service

import (
	"errors"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
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
	var list []model.AutoReply
	if err := gormDB().Where("tenant_id = ?", tenantID).
		Order("priority desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]AutoReplyVO, 0, len(list))
	for _, r := range list {
		out = append(out, toAutoReplyVO(r))
	}
	return out, nil
}

func toAutoReplyVO(r model.AutoReply) AutoReplyVO {
	vo := AutoReplyVO{
		ID: r.ID, Name: r.Name, ChannelID: r.ChannelID,
		MatchType: r.MatchType, Keywords: r.Keywords, Reply: r.Reply,
		ReplyType: r.ReplyType, TransferToHuman: r.TransferToHuman,
		Enabled: r.Enabled, Priority: r.Priority, HitCount: r.HitCount,
		CreatedAt: fmtTime(r.CreatedAt), ChannelName: "全渠道",
	}
	if r.ChannelID > 0 {
		var ch model.Channel
		if gormDB().First(&ch, r.ChannelID).Error == nil {
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
	if err := gormDB().Create(&r).Error; err != nil {
		return nil, err
	}
	vo := toAutoReplyVO(r)
	return &vo, nil
}

func UpdateAutoReply(tenantID, id uint, req AutoReplyRequest) error {
	var r model.AutoReply
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&r).Error; err != nil {
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
	return gormDB().Save(&r).Error
}

func DeleteAutoReply(tenantID, id uint) error {
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.AutoReply{}).Error
}

// MatchRule 按优先级找出命中的规则（优先匹配指定渠道的规则，其次全渠道规则）
func MatchRule(tenantID, channelID uint, question string) (model.AutoReply, bool) {
	if strings.TrimSpace(question) == "" {
		return model.AutoReply{}, false
	}
	var rules []model.AutoReply
	gormDB().Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Order("priority desc, id asc").Find(&rules)
	q := strings.ToLower(strings.TrimSpace(question))

	// 先渠道专属，再全渠道
	for _, scope := range []uint{channelID, 0} {
		for _, r := range rules {
			if r.ChannelID != scope {
				continue
			}
			if matchKeywords(r, q) {
				r.HitCount++
				gormDB().Model(&model.AutoReply{}).Where("id = ?", r.ID).Update("hit_count", r.HitCount)
				return r, true
			}
		}
	}
	return model.AutoReply{}, false
}

// ReplyResult 自动回复判定结果
type ReplyResult struct {
	Transferred bool        // 是否命中转人工
	Message     interface{} // 机器人回复的消息 VO（可能为 nil）
}

// ReplyNow 同步执行一次自动回复判定（用户端接口即时返回用）
func ReplyNow(tenantID, convID, customerID, channelID uint, question string) *ReplyResult {
	var conv model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&conv).Error; err != nil {
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
		addSystemMessage(tenantID, convID, "客户请求人工客服"+desc+"，正在为您转接，请稍候")
		gormDB().Model(&model.Conversation{}).Where("id = ?", convID).
			Updates(map[string]interface{}{"status": "pending", "agent_id": 0})
		notifyChannelAgents(tenantID, channelID, convID, "待接待提醒", "有客户请求人工客服，请尽快接入")
		broadcastConversation(tenantID, convID)
		return &ReplyResult{Transferred: true}
	}

	var reply string
	if matched {
		reply = rule.Reply
	} else {
		var ch model.Channel
		if gormDB().First(&ch, channelID).Error == nil && ch.RobotEnabled {
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
	if err := gormDB().Create(&m).Error; err != nil {
		return &ReplyResult{Transferred: false}
	}
	gormDB().Model(&model.Conversation{}).Where("id = ?", convID).
		Updates(map[string]interface{}{"last_message": reply, "last_message_at": time.Now()})
	vo := buildMessageVO(m)
	HubInstance().Publish(tenantID, nil, Event{Event: "message.new", Data: vo})
	return &ReplyResult{Transferred: false, Message: vo}
}

func matchKeywords(r model.AutoReply, lowerQ string) bool {
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
