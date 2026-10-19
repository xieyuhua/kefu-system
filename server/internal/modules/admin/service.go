package admin

// 本文件是后台端（modules/admin）的端内编排：数据统计概览。
// 其余后台能力（渠道、子账号、客户与标签、自动回复、设置）直接调 core/* 的 service，
// 端内只为「概览页」保留编排与统计 SQL —— 它是后台专属口径，没有第二个端使用。
//
// 依赖方向：modules/admin → core/* → platform → pkg。

import (
	"time"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// ---------------- 数据统计 ----------------
// 供后台概览：接待趋势（按天）、会话量分布、客服工作量、渠道分布。

type DailyPoint struct {
	Date       string `json:"date"`          // 2026-03-05
	Total      int64  `json:"total"`         // 当日会话数
	Handled    int64  `json:"handled"`       // 已接待（agent_id > 0）
	Closed     int64  `json:"closed"`        // 已结束
	Pending    int64  `json:"pending"`       // 未接待
	Messages   int64  `json:"messages"`      // 当日消息数
	CustomerMs int64  `json:"customer_msgs"` // 客户消息数
	AgentMs    int64  `json:"agent_msgs"`    // 客服消息数
}

type AgentStat struct {
	AgentID   uint   `json:"agent_id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Role      string `json:"role"`            // admin | agent
	Handled   int64  `json:"handled"`         // 接待会话数
	Closed    int64  `json:"closed"`          // 结束会话数
	Messages  int64  `json:"messages"`        // 发送消息数
	ActiveNow int64  `json:"active_now"`      // 当前进行中
	AvgFirst  int64  `json:"avg_first_reply"` // 平均首次响应秒
}

type ChannelStat struct {
	ChannelID uint   `json:"channel_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Total     int64  `json:"total"`
	Pending   int64  `json:"pending"`
	Open      int64  `json:"open"`
	Closed    int64  `json:"closed"`
}

type StatsOverview struct {
	Trend    []DailyPoint  `json:"trend"`
	Agents   []AgentStat   `json:"agents"`
	Channels []ChannelStat `json:"channels"`
	Summary  StatsSummary  `json:"summary"`
}

type StatsSummary struct {
	TotalConversations int64   `json:"total_conversations"`
	HandledRate        float64 `json:"handled_rate"` // 接待率 %
	AvgMessages        float64 `json:"avg_messages"` // 平均消息数/会话
	Customers          int64   `json:"customers"`
	NewCustomers       int64   `json:"new_customers"`
	Orders             int64   `json:"orders"`
	OnlineAgents       int64   `json:"online_agents"`
}

// StatsOverviewData 生成指定天数的数据概览
func StatsOverviewData(tenantID uint, days int) (*StatsOverview, error) {
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	out := &StatsOverview{
		Trend:    []DailyPoint{},
		Agents:   []AgentStat{},
		Channels: []ChannelStat{},
	}

	loc := time.Now().Location()
	today := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, -(days - 1))

	// 趋势
	for i := 0; i < days; i++ {
		from := start.AddDate(0, 0, i)
		to := from.AddDate(0, 0, 1)
		p := DailyPoint{Date: from.Format("2006-01-02")}

		// 每项都是一条独立查询：pending/open/closed 不能合并，避免条件互相污染。
		// 单项统计失败按 0 处理，概览页始终可渲染（与原实现一致的容错口径）。
		p.Total, _ = CountConversationsInRange(tenantID, from, to)
		p.Handled, _ = CountHandledConversationsInRange(tenantID, from, to)
		p.Closed, _ = CountConversationsInRangeByStatus(tenantID, from, to, "closed")
		p.Pending, _ = CountConversationsInRangeByStatus(tenantID, from, to, "pending")
		p.Messages, _ = CountMessagesInRange(tenantID, from, to)
		p.CustomerMs, _ = CountMessagesInRangeByDirection(tenantID, from, to, "in")
		p.AgentMs, _ = CountMessagesInRangeByDirection(tenantID, from, to, "out")

		out.Trend = append(out.Trend, p)
	}

	// 客服工作量：包含所有接待过会话的账号（admin 也算）
	// 注意：不能简单按 role='agent' 过滤 —— SAAS 管理员常常也亲自坐席
	users, _ := ListUsersForStats(tenantID)
	for _, u := range users {
		// 跳过完全没有参与接待的客服，表格更精炼
		handled, _ := CountAgentConversations(tenantID, u.ID)
		if handled == 0 && !u.Online {
			continue
		}
		s := AgentStat{AgentID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
		s.Handled = handled
		s.Closed, _ = CountAgentConversationsByStatus(tenantID, u.ID, "closed")
		s.ActiveNow, _ = CountAgentConversationsByStatus(tenantID, u.ID, "open")
		// 最近 N 天的消息数（与表头 "最近 N 天" 对齐）
		s.Messages, _ = CountAgentMessagesSince(tenantID, u.ID, start)
		out.Agents = append(out.Agents, s)
	}

	// 渠道分布：最近 N 天的会话（与趋势图、客服工作量时间口径一致）
	chs, _ := ListChannelsForStats(tenantID)
	for _, c := range chs {
		s := ChannelStat{ChannelID: c.ID, Name: c.Name, Type: c.Type}
		s.Total, _ = CountChannelConversationsSince(tenantID, c.ID, start)
		s.Pending, _ = CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "pending")
		s.Open, _ = CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "open")
		s.Closed, _ = CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "closed")
		out.Channels = append(out.Channels, s)
	}

	// 汇总
	out.Summary.TotalConversations, _ = CountTenantConversations(tenantID)
	handled, _ := CountTenantHandledConversations(tenantID)
	if out.Summary.TotalConversations > 0 {
		out.Summary.HandledRate = float64(handled) / float64(out.Summary.TotalConversations) * 100
		msgCount, _ := CountTenantMessages(tenantID)
		out.Summary.AvgMessages = float64(msgCount) / float64(out.Summary.TotalConversations)
	}
	out.Summary.Customers, _ = CountTenantCustomers(tenantID)
	out.Summary.NewCustomers, _ = CountNewCustomersSince(tenantID, start)
	out.Summary.Orders, _ = CountTenantOrders(tenantID)
	out.Summary.OnlineAgents, _ = CountOnlineAgents(tenantID)

	return out, nil
}

// ---------------- 数据统计（Stats）数据访问 ----------------
//
// 统计口径约定（改动前先对齐产品口径，这些条件一个字都不能动）：
//   - 「已接待」= agent_id > 0；「已结束」= status = 'closed'；「未接待」= status = 'pending'
//   - 时间窗口统一写 created_at >= from AND created_at < to（左闭右开）
//   - 每一项计数都是一条独立 SQL，不复用条件：多个计数拼在同一条链上会互相污染条件，
//     因此不要为了「省一次查询」把 pending/open/closed 合并成一条 SQL。

// countRows 执行计数并按 (int64, error) 返回，省掉每处 var/err 样板。
func countRows(q *gorm.DB) (int64, error) {
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountConversationsInRange 统计租户在 [from, to) 内创建的会话总数。
func CountConversationsInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to))
}

// CountHandledConversationsInRange 统计 [from, to) 内已接待（agent_id > 0）的会话数。
func CountHandledConversationsInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND agent_id > 0", tenantID, from, to))
}

// CountConversationsInRangeByStatus 统计 [from, to) 内指定状态（pending/open/closed）的会话数。
func CountConversationsInRangeByStatus(tenantID uint, from, to time.Time, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status = ?", tenantID, from, to, status))
}

// CountMessagesInRange 统计 [from, to) 内的消息总数。
func CountMessagesInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to))
}

// CountMessagesInRangeByDirection 统计 [from, to) 内指定方向（in=客户 / out=客服）的消息数。
func CountMessagesInRangeByDirection(tenantID uint, from, to time.Time, direction string) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND direction = ?", tenantID, from, to, direction))
}

// ListUsersForStats 列出参与客服工作量统计的账号：admin 与 agent 都算（管理员常亲自坐席，
// 不能只按 role='agent' 过滤），按 role 降序、id 升序返回。
func ListUsersForStats(tenantID uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ?", tenantID).
		Order("role desc, id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CountAgentConversations 统计某账号接待过的会话总数（不限状态、不限时间）。
func CountAgentConversations(tenantID, agentID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID))
}

// CountAgentConversationsByStatus 统计某账号在指定状态下的会话数。
func CountAgentConversationsByStatus(tenantID, agentID uint, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, status))
}

// CountAgentMessagesSince 统计某账号自 since 起的消息数（沿用原有 sender_role='user' 口径，
// 与表头「最近 N 天」对齐）。
func CountAgentMessagesSince(tenantID, agentID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND sender_id = ? AND sender_role = 'user' AND created_at >= ?",
			tenantID, agentID, since))
}

// ListChannelsForStats 列出参与渠道分布统计的渠道，按 id 升序返回。
func ListChannelsForStats(tenantID uint) ([]*model.Channel, error) {
	var list []*model.Channel
	if err := storage.DB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountChannelConversationsSince 统计某渠道自 since 起创建的会话总数。
func CountChannelConversationsSince(tenantID, channelID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND created_at >= ?", tenantID, channelID, since))
}

// CountChannelConversationsSinceByStatus 统计某渠道自 since 起指定状态的会话数。
func CountChannelConversationsSinceByStatus(tenantID, channelID uint, since time.Time, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND status = ? AND created_at >= ?", tenantID, channelID, status, since))
}

// CountTenantConversations 统计租户全部会话数（汇总卡片用，不限时间）。
func CountTenantConversations(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID))
}

// CountTenantHandledConversations 统计租户全部已接待会话数（agent_id > 0）。
func CountTenantHandledConversations(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).Where("tenant_id = ? AND agent_id > 0", tenantID))
}

// CountTenantMessages 统计租户全部消息数（用于计算平均消息数/会话）。
func CountTenantMessages(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).Where("tenant_id = ?", tenantID))
}

// CountTenantCustomers 统计租户客户总数。
func CountTenantCustomers(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID))
}

// CountNewCustomersSince 统计自 since 起新增的客户数。
func CountNewCustomersSince(tenantID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Customer{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, since))
}

// CountTenantOrders 统计租户订单总数。
func CountTenantOrders(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Order{}).Where("tenant_id = ?", tenantID))
}

// CountOnlineAgents 统计当前在线客服数（online = true）。
func CountOnlineAgents(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.User{}).
		Where("tenant_id = ? AND online = ?", tenantID, true))
}
