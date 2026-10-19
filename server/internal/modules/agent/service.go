package agent

// 本文件是客服端（modules/agent）的端内编排：设备推送注册、离线消息 / 提醒补拉、演示面板。
// 会话与消息的读写、客户与会话的定位、自动回复判定都在 core/*，这里只做「客服端专属」的组合。
//
// 依赖方向：modules/agent → core/* → platform → pkg。
//
// 数据访问说明：devices / notifications 两张表只服务客服端的「重连补拉 + 推送 token」，
// 没有第二个端使用，因此读写就近放在本端 repository.go（端内分 handler / service / repository 三层）；
// 跨表只读口径（可见会话范围）同样在那里，与 core/conversation 的可见范围判定保持一致。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨内核依赖在构造时建好；
// 只有无状态的纯工具（randomNickname / randomPhone）保留为包级函数。

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"kefu-server/internal/core/automation"
	"kefu-server/internal/core/channel"
	"kefu-server/internal/core/conversation"
	"kefu-server/internal/core/customer"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/util"
)

// Service 客服端的端内编排层，持有本端仓储与四个跨内核依赖。
type Service struct {
	repo *Repository
	conv *conversation.Service
	cust *customer.Service
	ch   *channel.Service
	auto *automation.Service
}

// NewService 创建端内业务层实例（自带数据访问层与跨内核依赖）。
func NewService() *Service {
	return &Service{
		repo: NewRepository(),
		conv: conversation.NewService(),
		cust: customer.NewService(),
		ch:   channel.NewService(),
		auto: automation.NewService(),
	}
}

// ---------------- Device（APP / PC 桌面端） ----------------

func (s *Service) RegisterDevice(tenantID, userID uint, req dto.DeviceRequest) (*dto.DeviceVO, error) {
	if req.Device == "" {
		req.Device = "app"
	}
	d := &model.Device{
		TenantID:  tenantID,
		UserID:    userID,
		Device:    req.Device,
		Platform:  req.Platform,
		PushToken: req.PushToken,
		Name:      req.Name,
	}
	if err := s.repo.CreateDevice(d); err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
	}, nil
}

func (s *Service) ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	list, err := s.repo.ListDevicesOfUser(tenantID, userID)
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

func (s *Service) DeleteDevice(tenantID, userID, id uint) error {
	return s.repo.DeleteDeviceOfUser(tenantID, userID, id)
}

// ---------------- 离线消息 / 离线提醒 ----------------

// OfflineMessages 供 APP / PC 桌面端断线重连后增量同步。sinceID 为端侧已收到的最大消息 ID。
func (s *Service) OfflineMessages(tenantID, userID uint, sinceID uint, limit int) ([]dto.MessageVO, uint, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	// 只同步该客服可见范围内的会话消息：自己接待的 + 自己渠道下未分配的
	convIDs, err := s.repo.ListVisibleConversationIDs(tenantID, userID)
	if err != nil {
		return nil, sinceID, false, err
	}
	if len(convIDs) == 0 {
		return []dto.MessageVO{}, sinceID, false, nil
	}
	// 多取一条用于判断是否还有更多
	list, err := s.repo.ListMessagesOfConversations(tenantID, convIDs, sinceID, limit+1)
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
		out = append(out, s.conv.BuildMessageVO(m))
		if m.ID > latest {
			latest = m.ID
		}
	}
	return out, latest, hasMore, nil
}

func (s *Service) OfflineNotifications(tenantID, userID uint, sinceID uint) ([]dto.NotificationVO, uint, error) {
	list, err := s.repo.ListNotificationsAfter(tenantID, userID, sinceID, 200)
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

func (s *Service) MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	return s.repo.SetNotificationsRead(tenantID, userID, ids)
}

// ---------------- 演示入口 ----------------

// DemoIncoming 模拟客户从某个渠道发起咨询（一键体验）。
// 命中自动回复规则时机器人先答；命中转人工关键词则直接转为待人工接待。
func (s *Service) DemoIncoming(tenantID uint, req dto.DemoIncomingRequest) (*dto.ConversationVO, *dto.MessageVO, error) {
	ch, err := s.ch.FindChannelByID(tenantID, req.ChannelID)
	if err != nil {
		return nil, nil, errors.New("渠道不存在")
	}
	nickname := strings.TrimSpace(req.Nickname)

	now := time.Now()
	uid := strings.TrimSpace(req.UID)

	// ① 指定了 uid：复用该渠道下已存在的客户与会话（可对同一用户反复发消息）
	if uid != "" {
		externalID := "demo_" + ch.Type + "_" + uid
		if cu, err := s.cust.FindCustomerByChannelExternalID(tenantID, ch.ID, externalID); err == nil {
			// 昵称有填则更新
			if nickname != "" {
				_ = s.cust.UpdateCustomerColumns(cu.ID, map[string]interface{}{"nickname": nickname})
			}
			_ = s.cust.UpdateCustomerColumns(cu.ID, map[string]interface{}{"last_seen_at": now})

			// 复用未结束的会话；全部结束则重开一个
			if conv, err := s.cust.FindActiveConversationOfCustomer(tenantID, cu.ID); err == nil {
				return s.sendDemoMessage(tenantID, conv, cu, req)
			}
			conv := &model.Conversation{
				TenantID: tenantID, ChannelID: ch.ID, CustomerID: cu.ID,
				Status: "pending", LastMessageAt: now,
			}
			if err := s.conv.CreateConversation(conv); err != nil {
				return nil, nil, err
			}
			return s.sendDemoMessage(tenantID, conv, cu, req)
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
	if err := s.cust.CreateCustomer(cu); err != nil {
		return nil, nil, err
	}

	conv := &model.Conversation{
		TenantID:      tenantID,
		ChannelID:     ch.ID,
		CustomerID:    cu.ID,
		Status:        "pending",
		LastMessageAt: now,
	}
	if err := s.conv.CreateConversation(conv); err != nil {
		return nil, nil, err
	}
	return s.sendDemoMessage(tenantID, conv, cu, req)
}

// sendDemoMessage 往指定会话追加一条客户消息并广播
func (s *Service) sendDemoMessage(
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
	mvo, cvo, err := s.conv.ReceiveMessage(tenantID, conv.ID, cu.ID, msgType, content)
	if err != nil {
		return nil, nil, err
	}

	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.new", Data: *cvo})
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "message.new", Data: *mvo})
	realtime.HubInstance().PublishToGuest(tenantID, conv.ID, realtime.Event{Event: "message.new", Data: *mvo})

	// 自动回复 / 转人工判定
	go s.HandleIncoming(tenantID, conv.ID, cu.ID, conv.ChannelID, content)
	return cvo, mvo, nil
}

// HandleIncoming 收到客户消息后的统一处理：
//  1. 命中"转人工"关键词 → 会话置为 pending 并通知该渠道在线客服
//  2. 命中自动回复规则   → 机器人回复
//  3. 渠道开启机器人且无规则命中 → 兜底回复
//
// 判定逻辑与用户端接口的同步判定（automation.ReplyNow）完全一致，这里只保留演示入口的延迟下发。
func (s *Service) HandleIncoming(tenantID, convID, customerID, channelID uint, question string) {
	time.Sleep(800 * time.Millisecond)
	s.auto.ReplyNow(tenantID, convID, customerID, channelID, question)
}

func randomNickname() string {
	first := []string{"王", "李", "张", "刘", "陈", "杨", "赵", "周"}
	last := []string{"小明", "婷婷", "先生", "女士", "小雨", "浩然"}
	return first[rand.Intn(len(first))] + last[rand.Intn(len(last))]
}

func randomPhone() string {
	return fmt.Sprintf("13%d****%04d", rand.Intn(9)+1, rand.Intn(10000))
}
