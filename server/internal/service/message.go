package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

// ---------------- Message ----------------

// ListMessagesSince 正向增量拉取：返回 id > afterID 的消息（按 id 升序）。
// 供用户端 / APP 离线同步使用 —— 与 ListMessages 的「向前翻页」语义相反，勿混用。
func ListMessagesSince(tenantID, convID uint, afterID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	list, err := repository.ListMessagesAfter(tenantID, convID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, buildMessageVO(m))
	}
	return out, nil
}

// ListMessages 分页向前翻页：返回 id < beforeID 的消息（按 id 升序）
func ListMessages(tenantID, convID uint, beforeID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 {
		limit = 50
	}
	list, err := repository.ListMessagesBefore(tenantID, convID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, buildMessageVO(m))
	}
	return out, nil
}

// BuildMessageVO 导出版本，供其他包（如消息搜索）复用
func BuildMessageVO(m *model.Message) dto.MessageVO { return buildMessageVO(m) }

func buildMessageVO(m *model.Message) dto.MessageVO {
	vo := dto.MessageVO{
		ID: m.ID, ConversationID: m.ConversationID, Type: m.Type,
		Direction: m.Direction, Content: m.Content,
		CreatedAt: fmtTime(m.CreatedAt),
	}
	switch m.SenderRole {
	case "user":
		u, err := repository.FindUserBriefByID(m.SenderID)
		if m.SenderID > 0 && err == nil {
			vo.Sender = dto.SenderRef{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: "agent"}
		} else {
			vo.Sender = dto.SenderRef{Name: "智能助手", Role: "robot"}
		}
	case "customer":
		if c, err := repository.FindCustomerByID(m.SenderID); err == nil {
			vo.Sender = dto.SenderRef{ID: c.ID, Name: c.Nickname, Avatar: c.Avatar, Role: "user"}
		}
	default:
		vo.Sender = dto.SenderRef{Role: "system"}
	}
	return vo
}

// SendMessage 客服发送消息
func SendMessage(tenantID, convID, senderID uint, req dto.SendMessageRequest) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, convID)
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
		if !isAdminUser(senderID) && !canServeChannel(senderID, c.ChannelID) {
			return nil, nil, errors.New("您未配置接待该渠道")
		}
		// 自动接管也受「最大同时接待数」限制
		if err := checkConcurrentLimit(tenantID, senderID); err != nil {
			return nil, nil, err
		}
		c.AgentID = senderID
		c.Status = "open"
	} else if c.AgentID != senderID && !isAdminUser(senderID) {
		// 会话已被他人接待，禁止越权发言
		return nil, nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       senderID,
		SenderRole:     "user",
		Type:           req.Type,
		Direction:      "out",
		Content:        req.Content,
		Read:           true,
	}
	if err := repository.CreateMessage(&m); err != nil {
		return nil, nil, err
	}
	c.LastMessage = messagePreview(&m)
	c.LastMessageAt = now
	c.MessageCount++
	if err := repository.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	mvo := buildMessageVO(&m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func messagePreview(m *model.Message) string {
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

func addSystemMessage(tenantID, convID uint, content string) *model.Message {
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderRole:     "system",
		Type:           "system",
		Direction:      "system",
		Content:        content,
		Read:           true,
	}
	// 落库失败不阻断主流程（与原逻辑一致）：系统提示语缺失不影响会话状态流转
	_ = repository.AddSystemMessage(&m)
	return &m
}

// ReceiveMessage 客户侧发消息（演示入口 / 渠道回调共用）
func ReceiveMessage(tenantID, convID, customerID uint, msgType, content string) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, convID)
	if err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if msgType == "" {
		msgType = "text"
	}
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       customerID,
		SenderRole:     "customer",
		Type:           msgType,
		Direction:      "in",
		Content:        content,
	}
	if err := repository.CreateMessage(&m); err != nil {
		return nil, nil, err
	}
	now := time.Now()
	c.LastMessage = messagePreview(&m)
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
	if err := repository.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	// 活跃时间刷新失败不阻断发消息（与原逻辑一致）
	_ = repository.UpdateCustomerColumns(customerID, map[string]interface{}{"last_seen_at": now})

	mvo := buildMessageVO(&m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func agentName(id uint) string {
	if u, err := repository.FindUserBriefByID(id); err == nil && u.Name != "" {
		return u.Name
	}
	return "客服"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
