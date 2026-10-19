package stats

// 本文件是共享统计内核（core/stats）的统计口径组装层：概览页的返回结构与计算方式。
// 结构与算法从 modules/admin 的 service 迁入，返回结构、字段计算方式、时间窗口（左闭右开）
// 全部保持原样；本内核只负责「把 Repository 的计数拼成概览结构」，不做 HTTP、不做鉴权。
//
// 依赖方向：core/stats → platform → pkg（本内核不 import 任何 modules，也不调用其它 core 包）。

import "time"

// Service 统计内核的业务编排层，持有本模块仓储。
type Service struct {
	repo *Repository
}

// NewService 创建业务层实例（自带数据访问层）。
func NewService() *Service {
	return &Service{repo: NewRepository()}
}

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
func (s *Service) StatsOverviewData(tenantID uint, days int) (*StatsOverview, error) {
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
		p.Total, _ = s.repo.CountConversationsInRange(tenantID, from, to)
		p.Handled, _ = s.repo.CountHandledConversationsInRange(tenantID, from, to)
		p.Closed, _ = s.repo.CountConversationsInRangeByStatus(tenantID, from, to, "closed")
		p.Pending, _ = s.repo.CountConversationsInRangeByStatus(tenantID, from, to, "pending")
		p.Messages, _ = s.repo.CountMessagesInRange(tenantID, from, to)
		p.CustomerMs, _ = s.repo.CountMessagesInRangeByDirection(tenantID, from, to, "in")
		p.AgentMs, _ = s.repo.CountMessagesInRangeByDirection(tenantID, from, to, "out")

		out.Trend = append(out.Trend, p)
	}

	// 客服工作量：包含所有接待过会话的账号（admin 也算）
	// 注意：不能简单按 role='agent' 过滤 —— SAAS 管理员常常也亲自坐席
	users, _ := s.repo.ListUsersForStats(tenantID)
	for _, u := range users {
		// 跳过完全没有参与接待的客服，表格更精炼
		handled, _ := s.repo.CountAgentConversations(tenantID, u.ID)
		if handled == 0 && !u.Online {
			continue
		}
		st := AgentStat{AgentID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
		st.Handled = handled
		st.Closed, _ = s.repo.CountAgentConversationsByStatus(tenantID, u.ID, "closed")
		st.ActiveNow, _ = s.repo.CountAgentConversationsByStatus(tenantID, u.ID, "open")
		// 最近 N 天的消息数（与表头 "最近 N 天" 对齐）
		st.Messages, _ = s.repo.CountAgentMessagesSince(tenantID, u.ID, start)
		out.Agents = append(out.Agents, st)
	}

	// 渠道分布：最近 N 天的会话（与趋势图、客服工作量时间口径一致）
	chs, _ := s.repo.ListChannelsForStats(tenantID)
	for _, c := range chs {
		st := ChannelStat{ChannelID: c.ID, Name: c.Name, Type: c.Type}
		st.Total, _ = s.repo.CountChannelConversationsSince(tenantID, c.ID, start)
		st.Pending, _ = s.repo.CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "pending")
		st.Open, _ = s.repo.CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "open")
		st.Closed, _ = s.repo.CountChannelConversationsSinceByStatus(tenantID, c.ID, start, "closed")
		out.Channels = append(out.Channels, st)
	}

	// 汇总
	out.Summary.TotalConversations, _ = s.repo.CountTenantConversations(tenantID)
	handled, _ := s.repo.CountTenantHandledConversations(tenantID)
	if out.Summary.TotalConversations > 0 {
		out.Summary.HandledRate = float64(handled) / float64(out.Summary.TotalConversations) * 100
		msgCount, _ := s.repo.CountTenantMessages(tenantID)
		out.Summary.AvgMessages = float64(msgCount) / float64(out.Summary.TotalConversations)
	}
	out.Summary.Customers, _ = s.repo.CountTenantCustomers(tenantID)
	out.Summary.NewCustomers, _ = s.repo.CountNewCustomersSince(tenantID, start)
	out.Summary.Orders, _ = s.repo.CountTenantOrders(tenantID)
	out.Summary.OnlineAgents, _ = s.repo.CountOnlineAgents(tenantID)

	return out, nil
}
