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
	"kefu-server/internal/repository"
)

// DemoIncoming 模拟客户从某个渠道发起咨询（一键体验）。
// 命中自动回复规则时机器人先答；命中转人工关键词则直接转为待人工接待。
func DemoIncoming(tenantID uint, req dto.DemoIncomingRequest) (*dto.ConversationVO, *dto.MessageVO, error) {
	ch, err := repository.FindChannelByID(tenantID, req.ChannelID)
	if err != nil {
		return nil, nil, errors.New("渠道不存在")
	}
	nickname := strings.TrimSpace(req.Nickname)

	now := time.Now()
	uid := strings.TrimSpace(req.UID)

	// ① 指定了 uid：复用该渠道下已存在的客户与会话（可对同一用户反复发消息）
	if uid != "" {
		externalID := "demo_" + ch.Type + "_" + uid
		if cu, err := repository.FindCustomerByChannelExternalID(tenantID, ch.ID, externalID); err == nil {
			// 昵称有填则更新
			if nickname != "" {
				_ = repository.UpdateCustomerColumns(cu.ID, map[string]interface{}{"nickname": nickname})
			}
			_ = repository.UpdateCustomerColumns(cu.ID, map[string]interface{}{"last_seen_at": now})

			// 复用未结束的会话；全部结束则重开一个
			if conv, err := repository.FindActiveConversationOfCustomer(tenantID, cu.ID); err == nil {
				return sendDemoMessage(tenantID, conv, cu, req)
			}
			conv := &model.Conversation{
				TenantID: tenantID, ChannelID: ch.ID, CustomerID: cu.ID,
				Status: "pending", LastMessageAt: now,
			}
			if err := repository.CreateConversation(conv); err != nil {
				return nil, nil, err
			}
			return sendDemoMessage(tenantID, conv, cu, req)
		}
	}

	// ② 未指定 uid（或该 uid 首次出现）：新建客户 + 新会话
	if nickname == "" {
		nickname = randomNickname()
	}
	externalID := fmt.Sprintf("demo_%s_%d", ch.Type, time.Now().UnixNano()%100000)
	if uid != "" {
		externalID = "demo_" + ch.Type + "_" + uid
	}

	cu := &model.Customer{
		TenantID:   tenantID,
		ChannelID:  ch.ID,
		ExternalID: externalID,
		Nickname:   nickname,
		Avatar:     fmt.Sprintf("/demo/avatar-%d.svg", rand.Intn(6)+1),
		Phone:      randomPhone(),
	}
	cu.LastSeenAt = &now
	if err := repository.CreateCustomer(cu); err != nil {
		return nil, nil, err
	}

	conv := &model.Conversation{
		TenantID:      tenantID,
		ChannelID:     ch.ID,
		CustomerID:    cu.ID,
		Status:        "pending",
		LastMessageAt: now,
	}
	if err := repository.CreateConversation(conv); err != nil {
		return nil, nil, err
	}
	return sendDemoMessage(tenantID, conv, cu, req)
}

// sendDemoMessage 往指定会话追加一条客户消息并广播
func sendDemoMessage(
	tenantID uint,
	conv *model.Conversation,
	cu *model.Customer,
	req dto.DemoIncomingRequest,
) (*dto.ConversationVO, *dto.MessageVO, error) {
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
	HubInstance().PublishToGuest(tenantID, conv.ID, Event{Event: "message.new", Data: *mvo})

	// 自动回复 / 转人工判定
	go HandleIncoming(tenantID, conv.ID, cu.ID, conv.ChannelID, content)
	return cvo, mvo, nil
}

// HandleIncoming 收到客户消息后的统一处理：
//  1. 命中"转人工"关键词 → 会话置为 pending 并通知该渠道在线客服
//  2. 命中自动回复规则   → 机器人回复
//  3. 渠道开启机器人且无规则命中 → 兜底回复
//
// 判定逻辑与用户端接口的同步判定（ReplyNow）完全一致，这里只保留演示入口的延迟下发。
func HandleIncoming(tenantID, convID, customerID, channelID uint, question string) {
	time.Sleep(800 * time.Millisecond)
	ReplyNow(tenantID, convID, customerID, channelID, question)
}

// notifyChannelAgents 通知该渠道下所有在线客服（转人工场景）
func notifyChannelAgents(tenantID, channelID, convID uint, title, body string) {
	acs, err := repository.ListAgentChannelsByChannel(channelID)
	if err != nil {
		return
	}
	for _, ac := range acs {
		u, err := repository.FindUserBriefByID(ac.UserID)
		if err != nil {
			continue
		}
		Notify(tenantID, u.ID, "conversation.pending", title, body, convID)
	}
}

func broadcastConversation(tenantID, convID uint) {
	c, err := repository.FindConversationByID(tenantID, convID)
	if err != nil {
		return
	}
	vo := buildConversationVO(*c)
	HubInstance().Publish(tenantID, nil, Event{Event: "conversation.updated", Data: vo})
	HubInstance().PublishToGuest(tenantID, convID, Event{Event: "conversation.updated", Data: vo})
}

// FindGuestConversation 定位（或创建）某渠道下某个外部用户的客户与会话。
// 用户端 WebSocket 与 guest/init 共用，保证两端拿到的是同一个会话。
func FindGuestConversation(tenantID, channelID uint, externalID string) (model.Customer, uint, error) {
	cu, err := repository.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
	if err != nil {
		return model.Customer{}, 0, errors.New("会话不存在，请先调用 /api/guest/init")
	}
	// 复用最近一个会话（含 closed/archived，自动激活），保证聊天记录合并
	conv, err := FindOrCreateConversation(tenantID, cu.ID, channelID)
	if err != nil {
		return *cu, 0, err
	}
	return *cu, conv.ID, nil
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
