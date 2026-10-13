package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"

	"gorm.io/gorm"
)

const TimeLayout = "2006-01-02 15:04:05"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(TimeLayout)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(TimeLayout)
}

// ---------------- Channel ----------------

func ListChannels(tenantID uint, agentID uint, isAdmin bool) ([]dto.ChannelVO, error) {
	var list []model.Channel
	q := gormDB().Where("tenant_id = ?", tenantID)
	if !isAdmin {
		q = q.Where("id IN (SELECT channel_id FROM agent_channels WHERE user_id = ?)", agentID)
	}
	if err := q.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]dto.ChannelVO, 0, len(list))
	for _, c := range list {
		var agentCount int64
		gormDB().Model(&model.AgentChannel{}).Where("channel_id = ?", c.ID).Count(&agentCount)
		var pending int64
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND status = 'pending'", tenantID, c.ID).Count(&pending)
		out = append(out, dto.ChannelVO{
			ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
			Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
			CreatedAt: fmtTime(c.CreatedAt), AgentCount: agentCount, PendingCount: pending,
			AccessPath: AccessPath(c),
		})
	}
	return out, nil
}

// AccessPath 渠道的用户端接入路径（相对路径，前端拼上 origin 即为完整地址）
func AccessPath(c model.Channel) string {
	return "/c?channel=" + c.Type
}

func CreateChannel(tenantID uint, req dto.ChannelRequest) (*dto.ChannelVO, error) {
	if req.Name == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	// 渠道类型不限制枚举，可任意自定义（如 wechat / wecom / web / 小红书 / 快手 ...）
	if req.Type == "" {
		req.Type = "custom"
	}
	c := model.Channel{
		TenantID:     tenantID,
		Type:         req.Type,
		Name:         req.Name,
		AppID:        req.AppID,
		Enabled:      true,
		RobotEnabled: req.RobotEnabled != nil && *req.RobotEnabled,
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if err := gormDB().Create(&c).Error; err != nil {
		return nil, err
	}
	return &dto.ChannelVO{
		ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
		Enabled: c.Enabled, RobotEnabled: c.RobotEnabled, CreatedAt: fmtTime(c.CreatedAt),
	}, nil
}

func UpdateChannel(tenantID, id uint, req dto.ChannelRequest) error {
	var c model.Channel
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return errors.New("渠道不存在")
	}
	if req.Name != "" {
		c.Name = req.Name
	}
	if req.Type != "" {
		c.Type = req.Type
	}
	if req.AppID != "" {
		c.AppID = req.AppID
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if req.RobotEnabled != nil {
		c.RobotEnabled = *req.RobotEnabled
	}
	return gormDB().Save(&c).Error
}

func DeleteChannel(tenantID, id uint) error {
	gormDB().Where("channel_id = ?", id).Delete(&model.AgentChannel{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Channel{}).Error
}

// ---------------- Agent ----------------

func ListAgents(tenantID uint) ([]dto.AgentVO, error) {
	var users []model.User
	if err := gormDB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]dto.AgentVO, 0, len(users))
	for _, u := range users {
		vo := dto.AgentVO{
			ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
			Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
			Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
		}
		vo.Status = "offline"
		if u.Online {
			vo.Status = "online"
		}
		var acs []model.AgentChannel
		gormDB().Where("user_id = ?", u.ID).Find(&acs)
		for _, ac := range acs {
			var ch model.Channel
			if gormDB().First(&ch, ac.ChannelID).Error == nil {
				vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
			}
		}
		var active int64
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND agent_id = ? AND status = 'open'", tenantID, u.ID).Count(&active)
		vo.ActiveCount = active
		out = append(out, vo)
	}
	return out, nil
}

func CreateAgent(tenantID uint, req dto.AgentRequest) (*dto.AgentVO, error) {
	if req.Account == "" || req.Password == "" {
		return nil, errors.New("账号与密码不能为空")
	}
	var exist model.User
	if gormDB().Where("account = ?", req.Account).First(&exist).Error == nil {
		return nil, errors.New("账号已存在")
	}
	hash, err := hashPwd(req.Password)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role != "admin" {
		role = "agent"
	}
	maxC := req.MaxConcurrent
	if maxC <= 0 {
		maxC = 5
	}
	u := model.User{
		TenantID:      tenantID,
		Account:       req.Account,
		PasswordHash:  hash,
		Name:          orDefault(req.Name, req.Account),
		Role:          role,
		Avatar:        "/demo/avatar-2.svg",
		MaxConcurrent: maxC,
		Enabled:       true,
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := gormDB().Create(&u).Error; err != nil {
		return nil, err
	}
	setAgentChannels(tenantID, u.ID, req.ChannelIDs)
	return getAgentVO(tenantID, u.ID)
}

func UpdateAgent(tenantID, id uint, req dto.AgentRequest) error {
	var u model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&u).Error; err != nil {
		return errors.New("账号不存在")
	}
	if req.Name != "" {
		u.Name = req.Name
	}
	if req.Role != "" {
		u.Role = req.Role
	}
	if req.MaxConcurrent > 0 {
		u.MaxConcurrent = req.MaxConcurrent
	}
	if req.Password != "" {
		h, err := hashPwd(req.Password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := gormDB().Save(&u).Error; err != nil {
		return err
	}
	if req.ChannelIDs != nil {
		setAgentChannels(tenantID, id, req.ChannelIDs)
	}
	return nil
}

func DeleteAgent(tenantID, id uint) error {
	gormDB().Where("user_id = ?", id).Delete(&model.AgentChannel{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.User{}).Error
}

func SetOnline(tenantID, id uint, online bool) error {
	return gormDB().Model(&model.User{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Update("online", online).Error
}

func setAgentChannels(tenantID, userID uint, ids []uint) {
	gormDB().Where("user_id = ?", userID).Delete(&model.AgentChannel{})
	var valid []model.Channel
	gormDB().Where("tenant_id = ?", tenantID).Find(&valid)
	allowed := map[uint]bool{}
	for _, c := range valid {
		allowed[c.ID] = true
	}
	for _, id := range ids {
		if !allowed[id] {
			continue
		}
		gormDB().Create(&model.AgentChannel{UserID: userID, ChannelID: id})
	}
}

func getAgentVO(tenantID, id uint) (*dto.AgentVO, error) {
	var u model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&u).Error; err != nil {
		return nil, err
	}
	vo := &dto.AgentVO{
		ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
		Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
		Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
	}
	vo.Status = "offline"
	if u.Online {
		vo.Status = "online"
	}
	var acs []model.AgentChannel
	gormDB().Where("user_id = ?", id).Find(&acs)
	for _, ac := range acs {
		var ch model.Channel
		if gormDB().First(&ch, ac.ChannelID).Error == nil {
			vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
		}
	}
	return vo, nil
}

// ---------------- Conversation ----------------

type ConvQuery struct {
	ChannelID uint
	Status    string
	Scope     string
	Keyword   string
	Page      int
	PageSize  int
}

func ListConversations(tenantID, userID uint, isAdmin bool, q ConvQuery) (*response.Paged, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	dbq := gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if q.ChannelID > 0 {
		dbq = dbq.Where("channel_id = ?", q.ChannelID)
	}
	if q.Status != "" && q.Status != "all" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	if !isAdmin || q.Scope == "mine" {
		// 自己接待的会话 或 「未分配且属于自己可接待渠道」的会话
		var chIDs []uint
		gormDB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
		if len(chIDs) > 0 {
			dbq = dbq.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
		} else {
			dbq = dbq.Where("agent_id = ?", userID)
		}
	}
	if q.Keyword != "" {
		kw := "%" + strings.TrimSpace(q.Keyword) + "%"
		dbq = dbq.Where(
			"last_message LIKE ? OR customer_id IN (SELECT id FROM customers WHERE nickname LIKE ?)",
			kw, kw)
	}

	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Conversation
	if err := dbq.Order("last_message_at desc").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		vos = append(vos, buildConversationVO(c))
	}
	return &response.Paged{List: vos, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func GetConversation(tenantID, id uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	vo := buildConversationVO(c)
	return &vo, nil
}

func buildConversationVO(c model.Conversation) dto.ConversationVO {
	vo := dto.ConversationVO{
		ID: c.ID, ChannelID: c.ChannelID, Status: c.Status,
		Unread: c.Unread, LastMessage: c.LastMessage,
		LastMessageAt: fmtTime(c.LastMessageAt), CreatedAt: fmtTime(c.CreatedAt),
		ClosedAt: fmtTimePtr(c.ClosedAt), MessageCount: c.MessageCount, Summary: c.Summary,
	}
	var ch model.Channel
	if gormDB().First(&ch, c.ChannelID).Error == nil {
		vo.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}
	var cu model.Customer
	if gormDB().First(&cu, c.CustomerID).Error == nil {
		var oc int64
		gormDB().Model(&model.Order{}).Where("customer_id = ?", cu.ID).Count(&oc)
		vo.User = &dto.CustomerRef{
			ID: cu.ID, ExternalID: cu.ExternalID, Nickname: cu.Nickname,
			Avatar: cu.Avatar, Phone: cu.Phone, OrderCount: oc, ChannelID: cu.ChannelID,
			Tags:   GetCustomerTags(c.TenantID, cu.ID),
			Remark: cu.Remark,
		}
		vo.OrderCount = oc
	}
	if c.AgentID > 0 {
		var u model.User
		if gormDB().First(&u, c.AgentID).Error == nil {
			vo.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role, Online: u.Online}
		}
	}
	return vo
}

func ClaimConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.Status == "closed" {
		return nil, errors.New("会话已结束")
	}
	if c.AgentID != 0 && c.AgentID != agentID {
		return nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	c.AgentID = agentID
	c.Status = "open"
	c.LastMessageAt = now
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, agentName(agentID)+" 已接入会话")
	vo := buildConversationVO(c)
	return &vo, nil
}

func TransferConversation(tenantID, id, fromAgentID, toAgentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.AgentID != fromAgentID {
		return nil, errors.New("只能转接自己接待的会话")
	}
	var target model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, toAgentID).First(&target).Error; err != nil {
		return nil, errors.New("目标客服不存在")
	}
	c.AgentID = toAgentID
	c.Status = "open"
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, fmt.Sprintf("会话已转接给 %s", target.Name))
	Notify(tenantID, toAgentID, "conversation.transferred", "会话转接", "有一条会话被转接给你", c.ID)
	vo := buildConversationVO(c)
	return &vo, nil
}

func CloseConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, "会话已结束，感谢您的咨询")
	vo := buildConversationVO(c)
	return &vo, nil
}

func MarkRead(tenantID, id uint) error {
	gormDB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{"unread": 0})
	gormDB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, id).
		Update("read", true)
	return nil
}

func ConversationStats(tenantID, userID uint, isAdmin bool) (*dto.StatsVO, error) {
	s := &dto.StatsVO{}
	base := gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if !isAdmin {
		var chIDs []uint
		gormDB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
		if len(chIDs) > 0 {
			base = base.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
		} else {
			base = base.Where("agent_id = ?", userID)
		}
	}
	base.Where("status = 'pending'").Count(&s.Pending)
	base.Where("status = 'open'").Count(&s.Open)
	base.Where("status = 'closed'").Count(&s.Closed)
	base.Where("unread > 0").Count(&s.Unread)
	return s, nil
}

// ---------------- Message ----------------

func ListMessages(tenantID, convID uint, beforeID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 {
		limit = 50
	}
	q := gormDB().Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []model.Message
	if err := q.Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	// 反转为升序返回
	out := make([]dto.MessageVO, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, buildMessageVO(list[i]))
	}
	return out, nil
}

func buildMessageVO(m model.Message) dto.MessageVO {
	vo := dto.MessageVO{
		ID: m.ID, ConversationID: m.ConversationID, Type: m.Type,
		Direction: m.Direction, Content: m.Content,
		CreatedAt: fmtTime(m.CreatedAt),
	}
	switch m.SenderRole {
	case "user":
		var u model.User
		if m.SenderID > 0 && gormDB().First(&u, m.SenderID).Error == nil {
			vo.Sender = dto.SenderRef{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: "agent"}
		} else {
			vo.Sender = dto.SenderRef{Name: "智能助手", Role: "robot"}
		}
	case "customer":
		var c model.Customer
		if gormDB().First(&c, m.SenderID).Error == nil {
			vo.Sender = dto.SenderRef{ID: c.ID, Name: c.Nickname, Avatar: c.Avatar, Role: "user"}
		}
	default:
		vo.Sender = dto.SenderRef{Role: "system"}
	}
	return vo
}

// SendMessage 客服发送消息
func SendMessage(tenantID, convID, senderID uint, req dto.SendMessageRequest) (*dto.MessageVO, *dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Content == "" {
		return nil, nil, errors.New("消息内容不能为空")
	}
	if req.Type == "order" || req.Type == "product" {
		var probe map[string]interface{}
		if err := json.Unmarshal([]byte(req.Content), &probe); err != nil {
			return nil, nil, errors.New("卡片内容必须是合法 JSON")
		}
	}
	if c.AgentID == 0 {
		c.AgentID = senderID
		c.Status = "open"
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
	if err := gormDB().Create(&m).Error; err != nil {
		return nil, nil, err
	}
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, nil, err
	}
	mvo := buildMessageVO(m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func messagePreview(m model.Message) string {
	switch m.Type {
	case "image":
		return "[图片]"
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
	gormDB().Create(&m)
	gormDB().Model(&model.Conversation{}).Where("id = ?", convID).
		Update("message_count", gorm.Expr("message_count + 1"))
	return &m
}

// ReceiveMessage 客户侧发消息（演示入口 / 渠道回调共用）
func ReceiveMessage(tenantID, convID, customerID uint, msgType, content string) (*dto.MessageVO, *dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
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
	if err := gormDB().Create(&m).Error; err != nil {
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
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, nil, err
	}
	gormDB().Model(&model.Customer{}).Where("id = ?", customerID).Update("last_seen_at", now)

	mvo := buildMessageVO(m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func agentName(id uint) string {
	var u model.User
	if gormDB().First(&u, id).Error == nil && u.Name != "" {
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
