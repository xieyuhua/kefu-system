package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"kefu-server/internal/common/dto"
	"kefu-server/internal/common/model"
	"kefu-server/internal/common/realtime"
	"kefu-server/pkg/util"
)

// ---------------- 消息搜索 ----------------

// SearchMessages 在指定会话内按内容关键词搜索消息，返回升序结果
func (s *ConversationService) SearchMessages(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, errors.New("请输入搜索关键词")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// repository 内部按 id 倒序取最近 limit 条后反转为升序，与消息流顺序一致
	return s.messages.SearchMessagesByContent(tenantID, convID, kw, limit)
}

// ---------------- Message ----------------

// ListMessagesSince 正向增量拉取：返回 id > afterID 的消息（按 id 升序）。
// 供用户端 / APP 离线同步使用 —— 与 ListMessages 的「向前翻页」语义相反，勿混用。
func (s *ConversationService) ListMessagesSince(tenantID, convID uint, afterID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	list, err := s.messages.ListMessagesAfter(tenantID, convID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, s.buildMessageVO(m))
	}
	return out, nil
}

// ListMessages 分页向前翻页：返回 id < beforeID 的消息（按 id 升序）
func (s *ConversationService) ListMessages(tenantID, convID uint, beforeID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 {
		limit = 50
	}
	list, err := s.messages.ListMessagesBefore(tenantID, convID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, s.buildMessageVO(m))
	}
	return out, nil
}

// BuildMessageVO 导出方法，供其他模块（如消息搜索、离线同步）复用
func (s *ConversationService) BuildMessageVO(m *model.Message) dto.MessageVO {
	return s.buildMessageVO(m)
}

func (s *ConversationService) buildMessageVO(m *model.Message) dto.MessageVO {
	vo := dto.MessageVO{
		ID: m.ID, ConversationID: m.ConversationID, Type: m.Type,
		Direction: m.Direction, Content: m.Content,
		CreatedAt: util.Default().FmtTime(m.CreatedAt),
	}
	switch m.SenderRole {
	case "user":
		u, err := s.account.FindUserBriefByID(m.SenderID)
		if m.SenderID > 0 && err == nil {
			vo.Sender = dto.SenderRef{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: "agent"}
		} else {
			vo.Sender = dto.SenderRef{Name: "智能助手", Role: "robot"}
		}
	case "customer":
		if c, err := s.customer.FindCustomerByID(m.SenderID); err == nil {
			vo.Sender = dto.SenderRef{ID: c.ID, Name: c.Nickname, Avatar: c.Avatar, Role: "user"}
		}
	default:
		vo.Sender = dto.SenderRef{Role: "system"}
	}
	return vo
}

// SendMessage 客服发送消息
func (s *ConversationService) SendMessage(tenantID, convID, senderID uint, req dto.SendMessageRequest) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Content == "" {
		return nil, nil, errors.New("消息内容不能为空")
	}
	// 卡片与媒体消息的 content 必须是合法 JSON（{url,kind,name,size}）
	if req.Type == "order" || req.Type == "product" || req.Type == "image" ||
		req.Type == "video" || req.Type == "file" {
		var probe map[string]interface{}
		if err := json.Unmarshal([]byte(req.Content), &probe); err != nil {
			return nil, nil, errors.New("消息内容格式不正确（需为合法 JSON）")
		}
	}
	if c.AgentID == 0 {
		// 未分配时由发送者接管；必须先确认他有权接待该渠道，
		// 否则未绑定的客服也能通过直接发消息「抢」走别人的待接待会话。
		if !s.isAdminUser(senderID) && !s.canServeChannel(senderID, c.ChannelID) {
			return nil, nil, errors.New("您未配置接待该渠道")
		}
		// 自动接管也受「最大同时接待数」限制
		if err := s.checkConcurrentLimit(tenantID, senderID); err != nil {
			return nil, nil, err
		}
		c.AgentID = senderID
		c.Status = "open"
	} else if c.AgentID != senderID && !s.isAdminUser(senderID) {
		// 会话已被他人接待，禁止越权发言
		return nil, nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       senderID,
		SenderRole:     "user",
		Type:           req.Type,
		Direction:      "out",
		Content:        req.Content,
		Read:           true,
	}
	if err := s.messages.CreateMessage(m); err != nil {
		return nil, nil, err
	}
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	mvo := s.buildMessageVO(m)
	cvo := s.buildConversationVO(c)
	return &mvo, &cvo, nil
}

func (s *ConversationService) addSystemMessage(tenantID, convID uint, content string) *model.Message {
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderRole:     "system",
		Type:           "system",
		Direction:      "system",
		Content:        content,
		Read:           true,
	}
	// 落库失败不阻断主流程（与原逻辑一致）：系统提示语缺失不影响会话状态流转
	_ = s.messages.AddSystemMessage(m)
	return m
}

// AppendSystemMessage 导出方法：给其它模块（automation 的自动回复转人工）落系统提示语。
// 内部实现仍是 addSystemMessage，保持「落库失败不阻断」的原语义。
func (s *ConversationService) AppendSystemMessage(tenantID, convID uint, content string) *model.Message {
	return s.addSystemMessage(tenantID, convID, content)
}

// ReceiveMessage 客户侧发消息（演示入口 / 渠道回调共用）
func (s *ConversationService) ReceiveMessage(tenantID, convID, customerID uint, msgType, content string) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if msgType == "" {
		msgType = "text"
	}
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       customerID,
		SenderRole:     "customer",
		Type:           msgType,
		Direction:      "in",
		Content:        content,
	}
	if err := s.messages.CreateMessage(m); err != nil {
		return nil, nil, err
	}
	now := time.Now()
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	c.Unread++
	if c.Status == "closed" {
		c.Status = "pending"
		c.ClosedAt = nil
	}
	// 客户重新咨询时，若会话曾被客服归档 → 自动恢复，重新出现在工作台待接待列表
	if c.Archived {
		c.Archived = false
		c.ArchivedAt = nil
		if c.Status == "closed" {
			c.Status = "pending"
			c.ClosedAt = nil
		}
		// 归档释放过客服归属，这里保持 agent_id = 0 进入待接待池
		if c.AgentID == 0 {
			c.Status = "pending"
		}
	}
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	// 活跃时间刷新失败不阻断发消息（与原逻辑一致）
	_ = s.customer.UpdateCustomerColumns(customerID, map[string]interface{}{"last_seen_at": now})

	mvo := s.buildMessageVO(m)
	cvo := s.buildConversationVO(c)
	return &mvo, &cvo, nil
}

// ---------------- 供其它内核复用的通知辅助 ----------------

// NotifyChannelAgents 通知该渠道下所有在线客服（转人工场景）。
// common/service 的 ReplyNow 需要它，而内核层不允许 automation 反向依赖端模块，
// 故留在会话内核（本包已具备渠道 / 账号读取能力）。
func (s *ConversationService) NotifyChannelAgents(tenantID, channelID, convID uint, title, body string) {
	acs, err := s.channel.ListAgentChannelsByChannel(channelID)
	if err != nil {
		return
	}
	for _, ac := range acs {
		u, err := s.account.FindUserBriefByID(ac.UserID)
		if err != nil {
			continue
		}
		realtime.Notify(tenantID, u.ID, "conversation.pending", title, body, convID)
	}
}

// BroadcastConversation 把会话最新状态推给客服端与用户端（原 service/demo.go 的 broadcastConversation）。
func (s *ConversationService) BroadcastConversation(tenantID, convID uint) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return
	}
	vo := s.buildConversationVO(c)
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.updated", Data: vo})
	realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{Event: "conversation.updated", Data: vo})
}
