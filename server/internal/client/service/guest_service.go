package service

// Package service 用户端（访客）的端内编排：免登录的客户识别、会话定位、消息收发。
// 端内无自有数据表，因此没有 repository / model 层。
//
// 依赖方向：client/handler → client/service → common/service → common/repository → common/model → platform → pkg。
// 本包只做用户端自己的编排（解析接入参数、找人或建人、定位会话、拼装初始化响应），
// 可复用的业务能力一律在公共业务层，不向其它端暴露。
// 跨层写入说明：访客首次进入要新建客户（customers 表属客户内核），
// 这里调用客户内核导出的 CustomerService.CreateCustomer —— 该逻辑原就写在访客域内，
// 拆成两个 service 只会让「找到或创建客户」的原子流程变得难读。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨层依赖在构造时建好；
// 只有 ErrChannelNotFound 这种无状态哨兵值保留为包级变量。

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/common/model"
	commonsvc "kefu-server/internal/common/service"
	"kefu-server/pkg/setting"
)

// Service 用户端（访客）的端内编排层，持有四个公共业务依赖。
type Service struct {
	Conv      *commonsvc.ConversationService
	Cust      *commonsvc.CustomerService
	Channels  *commonsvc.ChannelService
	AutoReply *commonsvc.AutomationService
}

// NewService 创建端内业务层实例（自带跨内核依赖）。
func NewService() *Service {
	return &Service{
		Conv:      commonsvc.NewConversationService(),
		Cust:      commonsvc.NewCustomerService(),
		Channels:  commonsvc.NewChannelService(),
		AutoReply: commonsvc.NewAutomationService(),
	}
}

// ---------------- 用户端（访客 / Guest）业务编排 ----------------
// handler 只负责解析 query 参数与包装响应，
// 客户识别、会话定位、订单 / 商品卡片的查询全部在这里完成，不在 HTTP 层碰数据库。

// ErrChannelNotFound 用户端接入参数 channel 匹配不到任何渠道。
var ErrChannelNotFound = errors.New("渠道不存在")

// ResolveGuestChannel 解析用户端接入参数 key。
//
// 优先按 app_id 匹配 —— 这是渠道的业务唯一标识（同一类型可配多个渠道，只有它能区分）。
// 同时兼容历史写法：渠道类型（wechat / web / ...）与数字渠道 ID。
// 匹配不到时返回 ErrChannelNotFound，由 handler 决定提示文案。
func (s *Service) ResolveGuestChannel(tenantID uint, key string) (*model.Channel, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrChannelNotFound
	}
	list, err := s.Channels.ListChannelsByTenant(tenantID, 0, true)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	// ① app_id 精确匹配
	for i := range list {
		if list[i].AppID != "" && list[i].AppID == key {
			return list[i], nil
		}
	}
	// ② 兼容：类型或数字 ID
	for i := range list {
		if list[i].Type == key || strconv.FormatUint(uint64(list[i].ID), 10) == key {
			return list[i], nil
		}
	}
	return nil, ErrChannelNotFound
}

// EnsureGuestCustomer 定位客户，不存在则创建（用户端页面初始化的第一条入口）。
// uid 为该渠道下的外部用户标识：同一渠道下同一个 uid 视为同一个客户。
func (s *Service) EnsureGuestCustomer(tenantID uint, ch *model.Channel, uid, nickname, avatar, phone, ip string) (*model.Customer, error) {
	if cu, err := s.Cust.FindCustomerByChannelExternalID(tenantID, ch.ID, uid); err == nil {
		return cu, nil
	}
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		// 取 uid 末尾 4 个「字符」区分；不足 4 位时取全串，避免短 uid 切片越界 panic
		r := []rune(uid)
		if len(r) > 4 {
			r = r[len(r)-4:]
		}
		nickname = "游客" + string(r)
	}
	now := time.Now()
	cu := &model.Customer{
		TenantID: tenantID, ChannelID: ch.ID, ExternalID: uid,
		Nickname: nickname, Avatar: avatar, Phone: phone,
		IP:         ip,
		LastSeenAt: &now,
	}
	if err := s.Cust.CreateCustomer(cu); err != nil {
		return nil, err
	}
	s.RecordCustomerIP(cu.ID, cu.IP)
	return cu, nil
}

// RecordCustomerIP 记录访客最新 IP 与归属城市。
// IP 归属地的缓存与落库口径在公共业务层客户域，端内只做一次转调（handler 也走这里）。
func (s *Service) RecordCustomerIP(customerID uint, ip string) {
	s.Cust.RecordCustomerIP(customerID, ip)
}

// FindGuestCustomer 按「租户 + 渠道 + 外部用户标识」定位客户。
// 未命中说明用户还没走过 /api/guest/init，由 handler 给出初始化提示。
func (s *Service) FindGuestCustomer(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	return s.Cust.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
}

// FindGuestCustomerByExternalID 按「租户 + 外部用户标识」定位客户（不限渠道）。
// 用户端无 WebSocket 时的轮询降级接口只带 uid，用它定位。
func (s *Service) FindGuestCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	return s.Cust.FindCustomerByExternalID(tenantID, externalID)
}

// LatestConversationOfCustomer 取客户最近一个会话（轮询降级接口定位「当前该看哪个会话」）。
func (s *Service) LatestConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return s.Conv.FindLatestConversationByCustomer(tenantID, customerID)
}

// FindGuestConversation 定位（或创建）某渠道下某个外部用户的客户与会话。
// 用户端 WebSocket 与 guest/init 共用，保证两端拿到的是同一个会话。
func (s *Service) FindGuestConversation(tenantID, channelID uint, externalID string) (*model.Customer, uint, error) {
	cu, err := s.Cust.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
	if err != nil {
		return nil, 0, errors.New("会话不存在，请先调用 /api/guest/init")
	}
	// 复用最近一个会话（含 closed/archived，自动激活），保证聊天记录合并
	conv, err := s.Conv.FindOrCreateConversation(tenantID, cu.ID, channelID)
	if err != nil {
		return cu, 0, err
	}
	return cu, conv.ID, nil
}

// LookupOrderCard 按订单号取订单并转成聊天订单卡片；找不到返回 nil（响应里表现为 null）。
func (s *Service) LookupOrderCard(tenantID uint, orderNo string) map[string]interface{} {
	o, err := s.Cust.FindOrderByNo(tenantID, orderNo)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	_ = json.Unmarshal([]byte(s.Cust.BuildOrderCard(o)), &out)
	return out
}

// LookupProductCard 按对外商品 ID 取商品并转成聊天商品卡片；找不到返回 nil。
func (s *Service) LookupProductCard(tenantID uint, pid string) map[string]interface{} {
	p, err := s.Cust.FindProductByPID(tenantID, pid)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	_ = json.Unmarshal([]byte(s.Cust.BuildProductCard(p)), &out)
	return out
}

// DefaultTenantID 用户端未显式传 ?tenant= 时的兜底租户（便于演示）；取不到返回 0。
// 口径已下沉到公共业务层的客户域：上传服务的游客态分支也要用同一份 tenants 查询，
// 这里只保留端内入口，避免两处各写一遍「取第一个租户」。
func (s *Service) DefaultTenantID() uint {
	return s.Cust.DefaultTenantID()
}

// AssistantAvatar 智能助手（机器人）回复时展示的头像地址（后台可配置）。
// 未配置返回空串，由用户端回退到内置头像；真实客服账号的头像不经过这里。
func (s *Service) AssistantAvatar(tenantID uint) string {
	return setting.AssistantAvatar(tenantID)
}
