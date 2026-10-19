package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

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
func ResolveGuestChannel(tenantID uint, key string) (*model.Channel, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrChannelNotFound
	}
	list, err := repository.ListChannelsByTenant(tenantID, 0, true)
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
func EnsureGuestCustomer(tenantID uint, ch *model.Channel, uid, nickname, avatar, phone, ip string) (*model.Customer, error) {
	if cu, err := repository.FindCustomerByChannelExternalID(tenantID, ch.ID, uid); err == nil {
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
	cu := model.Customer{
		TenantID: tenantID, ChannelID: ch.ID, ExternalID: uid,
		Nickname: nickname, Avatar: avatar, Phone: phone,
		IP:         ip,
		LastSeenAt: &now,
	}
	if err := repository.CreateCustomer(&cu); err != nil {
		return nil, err
	}
	RecordCustomerIP(cu.ID, cu.IP)
	return &cu, nil
}

// FindGuestCustomer 按「租户 + 渠道 + 外部用户标识」定位客户。
// 未命中说明用户还没走过 /api/guest/init，由 handler 给出初始化提示。
func FindGuestCustomer(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	return repository.FindCustomerByChannelExternalID(tenantID, channelID, externalID)
}

// FindGuestCustomerByExternalID 按「租户 + 外部用户标识」定位客户（不限渠道）。
// 用户端无 WebSocket 时的轮询降级接口只带 uid，用它定位。
func FindGuestCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	return repository.FindCustomerByExternalID(tenantID, externalID)
}

// LatestConversationOfCustomer 取客户最近一个会话（轮询降级接口定位「当前该看哪个会话」）。
func LatestConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return repository.FindLatestConversationByCustomer(tenantID, customerID)
}

// LookupOrderCard 按订单号取订单并转成聊天订单卡片；找不到返回 nil（响应里表现为 null）。
func LookupOrderCard(tenantID uint, orderNo string) map[string]interface{} {
	o, err := repository.FindOrderByNo(tenantID, orderNo)
	if err != nil {
		return nil
	}
	var card map[string]interface{}
	_ = json.Unmarshal([]byte(BuildOrderCard(o)), &card)
	return card
}

// LookupProductCard 按对外商品 ID 取商品并转成聊天商品卡片；找不到返回 nil。
func LookupProductCard(tenantID uint, pid string) map[string]interface{} {
	p, err := repository.FindProductByPID(tenantID, pid)
	if err != nil {
		return nil
	}
	var card map[string]interface{}
	_ = json.Unmarshal([]byte(BuildProductCard(p)), &card)
	return card
}

// DefaultTenantID 用户端未登录、未显式传 ?tenant= 时取第一个租户（便于演示）；取不到返回 0。
func DefaultTenantID() uint {
	id, err := repository.FirstTenantID()
	if err != nil {
		return 0
	}
	return id
}
