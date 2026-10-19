// Package ops 运营支撑模块：数据统计、系统设置端点（HTTP 适配 platform/setting）、
// 设备推送注册、离线消息/提醒同步、演示入口。模块内按 handler / service / repository 分层。
//
// 依赖方向：ops → chat（会话/消息 VO）、customer（客户与商品）、account（客服）、automation（自动回复判定）。
// 跨模块写入说明：演示入口要「新建客户 + 刷新客户昵称 / 活跃时间」，按归属表这段流程属于 ops，
// 因此直接调用 customer 导出的仓储函数（CreateCustomer / UpdateCustomerColumns），
// 会话则由 chat 导出的 service 函数创建（chat.CreateConversation）。
package ops

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"kefu-server/internal/modules/automation"
	"kefu-server/internal/modules/chat"
	"kefu-server/internal/modules/customer"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/util"
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

// ---------------- Device（APP / PC 桌面端） ----------------

func RegisterDevice(tenantID, userID uint, req dto.DeviceRequest) (*dto.DeviceVO, error) {
	if req.Device == "" {
		req.Device = "app"
	}
	d := model.Device{
		TenantID:  tenantID,
		UserID:    userID,
		Device:    req.Device,
		Platform:  req.Platform,
		PushToken: req.PushToken,
		Name:      req.Name,
	}
	if err := CreateDevice(&d); err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
	}, nil
}

func ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	list, err := ListDevicesOfUser(tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DeviceVO, 0, len(list))
	for _, d := range list {
		out = append(out, dto.DeviceVO{
			ID: d.ID, Device: d.Device, Platform: d.Platform,
			Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
		})
	}
	return out, nil
}

func DeleteDevice(tenantID, userID, id uint) error {
	return DeleteDeviceOfUser(tenantID, userID, id)
}

// ---------------- 离线消息 / 离线提醒 ----------------

// OfflineMessages 供 APP / PC 桌面端断线重连后增量同步。sinceID 为端侧已收到的最大消息 ID。
func OfflineMessages(tenantID, userID uint, sinceID uint, limit int) ([]dto.MessageVO, uint, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	// 只同步该客服可见范围内的会话消息：自己接待的 + 自己渠道下未分配的
	convIDs, err := ListVisibleConversationIDs(tenantID, userID)
	if err != nil {
		return nil, sinceID, false, err
	}
	if len(convIDs) == 0 {
		return []dto.MessageVO{}, sinceID, false, nil
	}
	// 多取一条用于判断是否还有更多
	list, err := ListMessagesOfConversations(tenantID, convIDs, sinceID, limit+1)
	if err != nil {
		return nil, sinceID, false, err
	}
	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	out := make([]dto.MessageVO, 0, len(list))
	latest := sinceID
	for _, m := range list {
		out = append(out, chat.BuildMessageVO(m))
		if m.ID > latest {
			latest = m.ID
		}
	}
	return out, latest, hasMore, nil
}

func OfflineNotifications(tenantID, userID uint, sinceID uint) ([]dto.NotificationVO, uint, error) {
	list, err := ListNotificationsAfter(tenantID, userID, sinceID, 200)
	if err != nil {
		return nil, sinceID, err
	}
	out := make([]dto.NotificationVO, 0, len(list))
	latest := sinceID
	for _, n := range list {
		out = append(out, dto.NotificationVO{
			ID: n.ID, Type: n.Type, Title: n.Title, Body: n.Body,
			ConversationID: n.ConversationID, Read: n.Read, CreatedAt: util.FmtTime(n.CreatedAt),
		})
		if n.ID > latest {
			latest = n.ID
		}
	}
	return out, latest, nil
}

func MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	return SetNotificationsRead(tenantID, userID, ids)
}

// ---------------- 演示入口 ----------------

// DemoIncoming 模拟客户从某个渠道发起咨询（一键体验）。
// 命中自动回复规则时机器人先答；命中转人工关键词则直接转为待人工接待。
func DemoIncoming(tenantID uint, req dto.DemoIncomingRequest) (*dto.ConversationVO, *dto.MessageVO, error) {
	ch, err := findChannelByID(tenantID, req.ChannelID)
	if err != nil {
		return nil, nil, errors.New("渠道不存在")
	}
	nickname := strings.TrimSpace(req.Nickname)

	now := time.Now()
	uid := strings.TrimSpace(req.UID)

	// ① 指定了 uid：复用该渠道下已存在的客户与会话（可对同一用户反复发消息）
	if uid != "" {
		externalID := "demo_" + ch.Type + "_" + uid
		if cu, err := customer.FindCustomerByChannelExternalID(tenantID, ch.ID, externalID); err == nil {
			// 昵称有填则更新
			if nickname != "" {
				_ = customer.UpdateCustomerColumns(cu.ID, map[string]interface{}{"nickname": nickname})
			}
			_ = customer.UpdateCustomerColumns(cu.ID, map[string]interface{}{"last_seen_at": now})

			// 复用未结束的会话；全部结束则重开一个
			if conv, err := customer.FindActiveConversationOfCustomer(tenantID, cu.ID); err == nil {
				return sendDemoMessage(tenantID, conv, cu, req)
			}
			conv := &model.Conversation{
				TenantID: tenantID, ChannelID: ch.ID, CustomerID: cu.ID,
				Status: "pending", LastMessageAt: now,
			}
			if err := chat.CreateConversation(conv); err != nil {
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
	if err := customer.CreateCustomer(cu); err != nil {
		return nil, nil, err
	}

	conv := &model.Conversation{
		TenantID:      tenantID,
		ChannelID:     ch.ID,
		CustomerID:    cu.ID,
		Status:        "pending",
		LastMessageAt: now,
	}
	if err := chat.CreateConversation(conv); err != nil {
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
	mvo, cvo, err := chat.ReceiveMessage(tenantID, conv.ID, cu.ID, msgType, content)
	if err != nil {
		return nil, nil, err
	}

	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.new", Data: *cvo})
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "message.new", Data: *mvo})
	realtime.HubInstance().PublishToGuest(tenantID, conv.ID, realtime.Event{Event: "message.new", Data: *mvo})

	// 自动回复 / 转人工判定
	go HandleIncoming(tenantID, conv.ID, cu.ID, conv.ChannelID, content)
	return cvo, mvo, nil
}

// HandleIncoming 收到客户消息后的统一处理：
//  1. 命中"转人工"关键词 → 会话置为 pending 并通知该渠道在线客服
//  2. 命中自动回复规则   → 机器人回复
//  3. 渠道开启机器人且无规则命中 → 兜底回复
//
// 判定逻辑与用户端接口的同步判定（automation.ReplyNow）完全一致，这里只保留演示入口的延迟下发。
func HandleIncoming(tenantID, convID, customerID, channelID uint, question string) {
	time.Sleep(800 * time.Millisecond)
	automation.ReplyNow(tenantID, convID, customerID, channelID, question)
}

func randomNickname() string {
	first := []string{"王", "李", "张", "刘", "陈", "杨", "赵", "周"}
	last := []string{"小明", "婷婷", "先生", "女士", "小雨", "浩然"}
	return first[rand.Intn(len(first))] + last[rand.Intn(len(last))]
}

func randomPhone() string {
	return fmt.Sprintf("13%d****%04d", rand.Intn(9)+1, rand.Intn(10000))
}
