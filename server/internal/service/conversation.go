package service

import (
	"errors"
	"fmt"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
	"kefu-server/internal/response"
)

// ---------------- Conversation ----------------

type ConvQuery struct {
	ChannelID       uint
	Status          string
	Scope           string
	Keyword         string
	Page            int
	PageSize        int
	IncludeArchived bool // 是否包含已归档会话
}

func ListConversations(tenantID, userID uint, isAdmin bool, q ConvQuery) (*response.Paged, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	// 可见范围（scope=mine / 管理员全量 / 普通客服看自己接待的 + 自己渠道待接待的）
	// 与默认排除已归档的过滤都在 repository 内完成，保证与统计口径一致。
	list, total, err := repository.ListConversations(tenantID, userID, isAdmin, repository.ConversationFilter{
		ChannelID:       q.ChannelID,
		Status:          q.Status,
		Scope:           q.Scope,
		Keyword:         q.Keyword,
		IncludeArchived: q.IncludeArchived,
		Page:            q.Page,
		PageSize:        q.PageSize,
	})
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		vos = append(vos, buildConversationVO(c))
	}
	return &response.Paged{List: vos, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func GetConversation(tenantID, id uint) (*dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	vo := buildConversationVO(c)
	return &vo, nil
}

func buildConversationVO(c *model.Conversation) dto.ConversationVO {
	vo := dto.ConversationVO{
		ID: c.ID, ChannelID: c.ChannelID, Status: c.Status,
		Unread: c.Unread, LastMessage: c.LastMessage,
		LastMessageAt: fmtTime(c.LastMessageAt), CreatedAt: fmtTime(c.CreatedAt),
		ClosedAt: fmtTimePtr(c.ClosedAt), MessageCount: c.MessageCount, Summary: c.Summary,
		Archived: c.Archived, ArchivedAt: fmtTimePtr(c.ArchivedAt),
	}
	// 渠道 / 客户 / 客服的展示信息查不到时留空，不阻断 VO 组装（与原逻辑一致）
	if ch, err := repository.FindChannelByID(c.TenantID, c.ChannelID); err == nil {
		vo.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}
	if cu, err := repository.FindCustomerByID(c.CustomerID); err == nil {
		oc, _ := repository.CountOrdersOfCustomer(cu.ID)
		vo.User = &dto.CustomerRef{
			ID: cu.ID, ExternalID: cu.ExternalID, Nickname: cu.Nickname,
			Avatar: cu.Avatar, Phone: cu.Phone, OrderCount: oc, ChannelID: cu.ChannelID,
			Tags:   GetCustomerTags(c.TenantID, cu.ID),
			Remark: cu.Remark,
			IP:     cu.IP,
			City:   cityOrLookup(cu),
			// 客户在线状态：按最近活跃时间推断（2 分钟内活跃即在线）
			Online:     IsCustomerOnline(cu),
			LastSeenAt: fmtTimePtr(cu.LastSeenAt),
		}
		vo.OrderCount = oc
	}
	if c.AgentID > 0 {
		if u, err := repository.FindUserBriefByID(c.AgentID); err == nil {
			vo.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role, Online: u.Online}
		}
	}
	return vo
}

// isAdminUser 判断用户是否为管理员（管理员享有接管/查看全部会话的特权）
func isAdminUser(userID uint) bool {
	if userID == 0 {
		return false
	}
	u, err := repository.FindUserBriefByID(userID)
	if err != nil {
		return false
	}
	return u.Role == "admin"
}

// canServeChannel 判断客服是否配置了接待该渠道。
func canServeChannel(userID, channelID uint) bool {
	ok, err := repository.HasAgentChannel(userID, channelID)
	return err == nil && ok
}

// countOpenConversations 统计某客服当前进行中的会话数（status=open）。
func countOpenConversations(tenantID, agentID uint) int64 {
	n, _ := repository.CountOpenConversations(tenantID, agentID)
	return n
}

// checkConcurrentLimit 校验客服是否已达「最大同时接待数」上限。
// 返回 nil 表示可继续接待；返回 error 表示已满。
// 规则：MaxConcurrent <= 0 视为不限；查不到用户时放行（避免误伤）。
func checkConcurrentLimit(tenantID, agentID uint) error {
	u, err := repository.FindUserBriefByID(agentID)
	if err != nil {
		return nil
	}
	if u.MaxConcurrent <= 0 {
		return nil
	}
	if active := countOpenConversations(tenantID, agentID); active >= int64(u.MaxConcurrent) {
		return fmt.Errorf("已达最大同时接待数（%d），请先处理现有会话", u.MaxConcurrent)
	}
	return nil
}

func ClaimConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.Status == "closed" {
		return nil, errors.New("会话已结束")
	}
	// 渠道绑定校验：该渠道只接待账号限制了名单时，未绑定的客服不能接。
	// 管理员不受限制（可接待任意渠道）。
	if !isAdminUser(agentID) && !canServeChannel(agentID, c.ChannelID) {
		return nil, errors.New("您未配置接待该渠道")
	}
	// 互斥接待：已被其他客服领取的会话不能再领。
	// 管理员例外——可以接管任何会话（含其他客服正在接待的）。
	if c.AgentID != 0 && c.AgentID != agentID && !isAdminUser(agentID) {
		return nil, errors.New("会话已被其他客服接待")
	}
	// 并发上限校验：仅当本次领取会让该客服的 open 会话数 +1 时才校验。
	// 自己已在 open 接待的会话重复领取属幂等操作，不拦截。
	if !(c.AgentID == agentID && c.Status == "open") {
		if err := checkConcurrentLimit(tenantID, agentID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	c.AgentID = agentID
	c.Status = "open"
	c.LastMessageAt = now
	if err := repository.SaveConversation(c); err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, agentName(agentID)+" 已接入会话")
	vo := buildConversationVO(c)
	// 通知用户端：已有人工客服接入
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

// lastSystemMessage 取会话最后一条 system 消息（VO）
func lastSystemMessage(tenantID, convID uint) *dto.MessageVO {
	m, err := repository.FindLastSystemMessage(tenantID, convID)
	if err != nil {
		return nil
	}
	vo := buildMessageVO(m)
	return &vo
}

func TransferConversation(tenantID, id, fromAgentID, toAgentID uint) (*dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.AgentID != fromAgentID {
		return nil, errors.New("只能转接自己接待的会话")
	}
	target, err := repository.FindUserByID(tenantID, toAgentID)
	if err != nil {
		return nil, errors.New("目标客服不存在")
	}
	// 转接目标必须配置了接待该渠道，否则转过去他根本看不到这个会话
	if !isAdminUser(toAgentID) && !canServeChannel(toAgentID, c.ChannelID) {
		return nil, errors.New("该客服未配置接待此渠道，无法转接")
	}
	// 目标客服并发上限校验（转给自己则无变化，跳过）
	if toAgentID != fromAgentID {
		if err := checkConcurrentLimit(tenantID, toAgentID); err != nil {
			return nil, errors.New("转接失败：" + err.Error())
		}
	}
	c.AgentID = toAgentID
	c.Status = "open"
	if err := repository.SaveConversation(c); err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, fmt.Sprintf("会话已转接给 %s", target.Name))
	Notify(tenantID, toAgentID, "conversation.transferred", "会话转接", "有一条会话被转接给你", c.ID)
	vo := buildConversationVO(c)
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	return &vo, nil
}

func CloseConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	c, err := repository.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if err := repository.SaveConversation(c); err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, "会话已结束，感谢您的咨询")
	vo := buildConversationVO(c)
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

func MarkRead(tenantID, id uint) error {
	// 会话未读清零 + 消息置读；失败不阻断（与原逻辑一致，下次打开会话会再次置读）
	_ = repository.ClearConversationUnread(tenantID, id)
	_ = repository.MarkMessagesRead(tenantID, id)
	return nil
}

func ConversationStats(tenantID, userID uint, isAdmin bool) (*dto.StatsVO, error) {
	s := &dto.StatsVO{}

	// 统计口径必须与 ListConversations 完全一致：同一套可见范围 + 默认排除已归档会话。
	// 否则会出现「侧边栏显示未接待 1，点进去却是空的」——归档会话被算进了统计，
	// 但列表接口过滤掉了 archived = true 的记录。
	// 三项计数各自新建查询（repository 内部每次独立构建条件），避免 GORM 条件互相污染。
	s.Pending, _ = repository.CountVisibleConversations(tenantID, userID, isAdmin, "pending")
	s.Open, _ = repository.CountVisibleConversations(tenantID, userID, isAdmin, "open")
	s.Closed, _ = repository.CountVisibleConversations(tenantID, userID, isAdmin, "closed")
	// 未读消息总数：把各会话的 unread 求和（会话 unread 字段 = 该会话未读消息条数）
	s.Unread, _ = repository.SumVisibleConversationUnread(tenantID, userID, isAdmin)
	return s, nil
}
