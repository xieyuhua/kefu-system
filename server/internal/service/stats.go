package service

import (
	"time"

	"kefu-server/internal/model"
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

// StatsOverview 生成指定天数的数据概览
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

		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to).Count(&p.Total)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND agent_id > 0", tenantID, from, to).Count(&p.Handled)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status = 'closed'", tenantID, from, to).Count(&p.Closed)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status = 'pending'", tenantID, from, to).Count(&p.Pending)
		gormDB().Model(&model.Message{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to).Count(&p.Messages)
		gormDB().Model(&model.Message{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND direction = 'in'", tenantID, from, to).Count(&p.CustomerMs)
		gormDB().Model(&model.Message{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND direction = 'out'", tenantID, from, to).Count(&p.AgentMs)

		out.Trend = append(out.Trend, p)
	}

	// 客服工作量：包含所有接待过会话的账号（admin 也算）
	// 注意：不能简单按 role='agent' 过滤 —— SAAS 管理员常常也亲自坐席
	var users []model.User
	gormDB().Where("tenant_id = ?", tenantID).Order("role desc, id asc").Find(&users)
	for _, u := range users {
		// 跳过完全没有参与接待的客服，表格更精炼
		var handled int64
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND agent_id = ?", tenantID, u.ID).Count(&handled)
		if handled == 0 && !u.Online {
			continue
		}
		s := AgentStat{AgentID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role}
		s.Handled = handled
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND agent_id = ? AND status = 'closed'", tenantID, u.ID).Count(&s.Closed)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND agent_id = ? AND status = 'open'", tenantID, u.ID).Count(&s.ActiveNow)
		// 最近 N 天的消息数（与表头 "最近 N 天" 对齐）
		gormDB().Model(&model.Message{}).
			Where("tenant_id = ? AND sender_id = ? AND sender_role = 'user' AND created_at >= ?",
				tenantID, u.ID, start).Count(&s.Messages)
		out.Agents = append(out.Agents, s)
	}

	// 渠道分布：最近 N 天的会话（与趋势图、客服工作量时间口径一致）
	var chs []model.Channel
	gormDB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&chs)
	for _, c := range chs {
		s := ChannelStat{ChannelID: c.ID, Name: c.Name, Type: c.Type}
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND created_at >= ?", tenantID, c.ID, start).Count(&s.Total)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND status = 'pending' AND created_at >= ?", tenantID, c.ID, start).Count(&s.Pending)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND status = 'open' AND created_at >= ?", tenantID, c.ID, start).Count(&s.Open)
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND status = 'closed' AND created_at >= ?", tenantID, c.ID, start).Count(&s.Closed)
		out.Channels = append(out.Channels, s)
	}

	// 汇总
	gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID).Count(&out.Summary.TotalConversations)
	var handled int64
	gormDB().Model(&model.Conversation{}).Where("tenant_id = ? AND agent_id > 0", tenantID).Count(&handled)
	if out.Summary.TotalConversations > 0 {
		out.Summary.HandledRate = float64(handled) / float64(out.Summary.TotalConversations) * 100
		var msgCount int64
		gormDB().Model(&model.Message{}).Where("tenant_id = ?", tenantID).Count(&msgCount)
		out.Summary.AvgMessages = float64(msgCount) / float64(out.Summary.TotalConversations)
	}
	gormDB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID).Count(&out.Summary.Customers)
	gormDB().Model(&model.Customer{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, start).Count(&out.Summary.NewCustomers)
	gormDB().Model(&model.Order{}).Where("tenant_id = ?", tenantID).Count(&out.Summary.Orders)
	gormDB().Model(&model.User{}).
		Where("tenant_id = ? AND online = ?", tenantID, true).Count(&out.Summary.OnlineAgents)

	return out, nil
}
