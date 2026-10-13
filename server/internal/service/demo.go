package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
)

// DemoIncoming 模拟客户从某个渠道发起咨询（一键体验）。
// 命中自动回复规则时机器人先答；命中转人工关键词则直接转为待人工接待。
func DemoIncoming(tenantID uint, req dto.DemoIncomingRequest) (*dto.ConversationVO, *dto.MessageVO, error) {
	var ch model.Channel
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, req.ChannelID).First(&ch).Error; err != nil {
		return nil, nil, errors.New("渠道不存在")
	}
	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = randomNickname()
	}
	externalID := fmt.Sprintf("demo_%s_%d", ch.Type, time.Now().UnixNano()%100000)

	cu := model.Customer{
		TenantID:   tenantID,
		ChannelID:  ch.ID,
		ExternalID: externalID,
		Nickname:   nickname,
		Avatar:     fmt.Sprintf("/demo/avatar-%d.svg", rand.Intn(6)+1),
		Phone:      randomPhone(),
	}
	now := time.Now()
	cu.LastSeenAt = &now
	if err := gormDB().Create(&cu).Error; err != nil {
		return nil, nil, err
	}

	conv := model.Conversation{
		TenantID:      tenantID,
		ChannelID:     ch.ID,
		CustomerID:    cu.ID,
		Status:        "pending",
		LastMessageAt: now,
	}
	if err := gormDB().Create(&conv).Error; err != nil {
		return nil, nil, err
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = "你好，我想咨询一下"
	}
	msgType := req.Type
	if msgType == "" {
		msgType = "text"
	}
	mvo, cvo, err := ReceiveMessage(tenantID, conv.ID, cu.ID, msgType, content)
	if err != nil {
		return nil, nil, err
	}

	HubInstance().Publish(tenantID, nil, Event{Event: "conversation.new", Data: *cvo})
	HubInstance().Publish(tenantID, nil, Event{Event: "message.new", Data: *mvo})

	// 自动回复 / 转人工判定
	go HandleIncoming(tenantID, conv.ID, cu.ID, ch.ID, content)
	return cvo, mvo, nil
}

// HandleIncoming 收到客户消息后的统一处理：
//  1. 命中"转人工"关键词 → 会话置为 pending 并通知该渠道在线客服
//  2. 命中自动回复规则   → 机器人回复
//  3. 渠道开启机器人且无规则命中 → 兜底回复
func HandleIncoming(tenantID, convID, customerID, channelID uint, question string) {
	time.Sleep(800 * time.Millisecond)

	var conv model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&conv).Error; err != nil {
		return
	}
	// 已被人工接入的会话不再自动回复
	if conv.AgentID > 0 && conv.Status == "open" {
		return
	}

	rule, matched := MatchRule(tenantID, channelID, question)

	// 1) 转人工
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
		return
	}

	// 2) 自动回复
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
		return
	}

	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderRole:     "user",
		SenderID:       0,
		Type:           "text",
		Direction:      "out",
		Content:        reply,
		Read:           true,
	}
	if err := gormDB().Create(&m).Error; err != nil {
		return
	}
	gormDB().Model(&model.Conversation{}).Where("id = ?", convID).
		Updates(map[string]interface{}{"last_message": reply, "last_message_at": time.Now()})
	HubInstance().Publish(tenantID, nil, Event{Event: "message.new", Data: buildMessageVO(m)})
}

// notifyChannelAgents 通知该渠道下所有在线客服（转人工场景）
func notifyChannelAgents(tenantID, channelID, convID uint, title, body string) {
	var acs []model.AgentChannel
	gormDB().Where("channel_id = ?", channelID).Find(&acs)
	for _, ac := range acs {
		var u model.User
		if gormDB().First(&u, ac.UserID).Error != nil {
			continue
		}
		Notify(tenantID, u.ID, "conversation.pending", title, body, convID)
	}
}

func broadcastConversation(tenantID, convID uint) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
		return
	}
	HubInstance().Publish(tenantID, nil, Event{Event: "conversation.updated", Data: buildConversationVO(c)})
}

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

func randomNickname() string {
	first := []string{"王", "李", "张", "刘", "陈", "杨", "赵", "周"}
	last := []string{"小明", "婷婷", "先生", "女士", "小雨", "浩然"}
	return first[rand.Intn(len(first))] + last[rand.Intn(len(last))]
}

func randomPhone() string {
	return fmt.Sprintf("13%d****%04d", rand.Intn(9)+1, rand.Intn(10000))
}

// BuildOrderCard 把订单转成聊天卡片 JSON（供前端发送订单卡片）
func BuildOrderCard(o model.Order) string {
	card := map[string]interface{}{
		"order_no":   o.OrderNo,
		"title":      o.Title,
		"status":     o.Status,
		"amount":     o.Amount,
		"quantity":   o.Quantity,
		"created_at": fmtTime(o.CreatedAt),
		"image":      o.Image,
		"sku":        o.SKU,
	}
	b, _ := json.Marshal(card)
	return string(b)
}

// BuildProductCard 商品卡片 JSON
func BuildProductCard(p model.Product) string {
	card := map[string]interface{}{
		"id":    p.PID,
		"title": p.Title,
		"price": p.Price,
		"image": p.Image,
		"stock": p.Stock,
		"sku":   p.SKU,
		"url":   p.URL,
	}
	b, _ := json.Marshal(card)
	return string(b)
}
