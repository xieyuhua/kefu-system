package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// 用户端（客户端）接口：所有参数通过 GET query 传递，便于运营直接拼链接投放。
//   /c?channel=web&uid=u_10001&nickname=王小明&order_no=SO...&pid=P10086

var errNoChannel = errors.New("渠道不存在")

// resolveChannel 解析用户端接入参数 channel。
//
// 优先按 app_id 匹配 —— 这是渠道的业务唯一标识（同一类型可配多个渠道，只有它能区分）。
// 同时兼容历史写法：渠道类型（wechat / web / ...）与数字渠道 ID。
func resolveChannel(c *gin.Context, tenantID uint) (*model.Channel, error) {
	key := strings.TrimSpace(c.Query("channel"))
	if key == "" {
		return nil, errNoChannel
	}
	var ch model.Channel
	// ① app_id 精确匹配
	if err := db.Where("tenant_id = ? AND app_id = ?", tenantID, key).First(&ch).Error; err == nil {
		return &ch, nil
	}
	// ② 兼容：类型或数字 ID
	if err := db.Where("tenant_id = ? AND (type = ? OR id = ?)", tenantID, key, key).First(&ch).Error; err == nil {
		return &ch, nil
	}
	return nil, errNoChannel
}

// GuestInit 用户端页面初始化：拿到渠道、客户、会话、欢迎语、上下文卡片
func GuestInit(c *gin.Context) {
	tenantID := guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	ch, err := resolveChannel(c, tenantID)
	if err != nil {
		response.Param(c, "渠道不存在或未启用：channel="+c.Query("channel"))
		return
	}
	uid := strings.TrimSpace(c.Query("uid"))
	if uid == "" {
		uid = "visitor_" + c.ClientIP() + "_" + ch.Type
	}

	// 找到或创建客户
	var cu model.Customer
	if err := gormDB().Where("tenant_id = ? AND channel_id = ? AND external_id = ?",
		tenantID, ch.ID, uid).First(&cu).Error; err != nil {
		nickname := strings.TrimSpace(c.Query("nickname"))
		if nickname == "" {
			nickname = "游客" + uid[len(uid)-4:]
		}
		now := time.Now()
		cu = model.Customer{
			TenantID: tenantID, ChannelID: ch.ID, ExternalID: uid,
			Nickname: nickname, Avatar: c.Query("avatar"), Phone: c.Query("phone"),
			IP:         service.ClientIP(c.Request.RemoteAddr, c.Request.Header),
			LastSeenAt: &now,
		}
		gormDB().Create(&cu)
		service.RecordCustomerIP(cu.ID, cu.IP)
	}

	// 定位或创建会话：同一客户始终复用最近一个会话（closed/archived 自动激活），
	// 保证同一 uid + 渠道的聊天记录自然合并
	conv, err := service.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 上下文：订单 / 商品
	var orderCard map[string]interface{}
	if no := strings.TrimSpace(c.Query("order_no")); no != "" {
		var o model.Order
		if gormDB().Where("tenant_id = ? AND order_no = ?", tenantID, no).First(&o).Error == nil {
			_ = json.Unmarshal([]byte(service.BuildOrderCard(o)), &orderCard)
		}
	}
	var productCard map[string]interface{}
	if pid := strings.TrimSpace(c.Query("pid")); pid != "" {
		var p model.Product
		if gormDB().Where("tenant_id = ? AND p_id = ?", tenantID, pid).First(&p).Error == nil {
			_ = json.Unmarshal([]byte(service.BuildProductCard(p)), &productCard)
		}
	}

	history, _ := service.ListMessages(tenantID, conv.ID, 0, 20)

	// 每次访问都刷新 IP 与归属城市（展示在客服工作台的会话列表）
	service.RecordCustomerIP(cu.ID, service.ClientIP(c.Request.RemoteAddr, c.Request.Header))

	response.OK(c, gin.H{
		"channel": gin.H{
			"id":             ch.ID,
			"type":           ch.Type,
			"name":           ch.Name,
			"hide_header":    ch.HideHeader,
			"hide_debug":     ch.HideDebug,
			"hide_more_menu": ch.HideMoreMenu,
		},
		"customer":     gin.H{"id": cu.ID, "external_id": cu.ExternalID, "nickname": cu.Nickname, "avatar": cu.Avatar},
		"conversation": gin.H{"id": conv.ID, "status": conv.Status},
		"greeting":     "您好，我是智能客服助手，请问有什么可以帮您？",
		"order":        orderCard,
		"product":      productCard,
		"history":      history,
	})
}

// GuestSend 用户端发消息：走自动回复/转人工引擎
func GuestSend(c *gin.Context) {
	tenantID := guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	ch, err := resolveChannel(c, tenantID)
	if err != nil {
		response.Param(c, "渠道不存在")
		return
	}
	var req dto.SendMessageRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Content == "" {
		response.Param(c, "消息内容不能为空")
		return
	}

	uid := strings.TrimSpace(c.Query("uid"))
	var cu model.Customer
	if err := gormDB().Where("tenant_id = ? AND channel_id = ? AND external_id = ?",
		tenantID, ch.ID, uid).First(&cu).Error; err != nil {
		response.Param(c, "请先调用 /api/guest/init 初始化会话")
		return
	}
	conv, err := service.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	mvo, cvo, err := service.ReceiveMessage(tenantID, conv.ID, cu.ID, req.Type, req.Content)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	service.RecordCustomerIP(cu.ID, service.ClientIP(c.Request.RemoteAddr, c.Request.Header))
	service.PublishMessage(tenantID, conv.ID, mvo, cvo)

	// 同步判定自动回复 / 转人工（供接口直接返回）
	transferred := false
	reply := service.ReplyNow(tenantID, conv.ID, cu.ID, ch.ID, req.Content)
	if reply != nil {
		transferred = reply.Transferred
	}

	out := gin.H{"message": mvo, "transferred": transferred}
	if reply != nil && reply.Message != nil {
		out["reply"] = reply.Message
	}
	response.OK(c, out)
}

// GuestMessages 无 WebSocket 环境的降级轮询
func GuestMessages(c *gin.Context) {
	tenantID := guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	var cu model.Customer
	if err := gormDB().Where("tenant_id = ? AND external_id = ?", tenantID, c.Query("uid")).
		First(&cu).Error; err != nil {
		response.OK(c, []interface{}{})
		return
	}
	var conv model.Conversation
	if err := gormDB().Where("tenant_id = ? AND customer_id = ?", tenantID, cu.ID).
		Order("id desc").First(&conv).Error; err != nil {
		response.OK(c, []interface{}{})
		return
	}
	// 三种拉取模式，游标方向各不相同：
	//   before_id > 0  → 向前翻页，返回 id < before_id 的**更早**消息（上滑加载历史）
	//   since_id  > 0  → 正向增量，返回 id > since_id 的**更新**消息（轮询补拉）
	//   两个都不传      → 首屏，返回**最新** limit 条
	beforeID := queryUint(c, "before_id")
	sinceID := queryUint(c, "since_id")
	limit := queryInt(c, "limit", 20)

	var list []dto.MessageVO
	var err error
	switch {
	case beforeID > 0:
		list, err = service.ListMessages(tenantID, conv.ID, beforeID, limit)
	case sinceID > 0:
		list, err = service.ListMessagesSince(tenantID, conv.ID, sinceID, limit)
	default:
		// 首屏：取最新的 limit 条（beforeID 传 0 即不带 id < 条件）
		list, err = service.ListMessages(tenantID, conv.ID, 0, limit)
	}
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// guestTenantID 用户端未登录，租户由 ?tenant= 指定（缺省取第一个租户，便于演示）
func guestTenantID(c *gin.Context) uint {
	if t := queryUint(c, "tenant"); t > 0 {
		return t
	}
	var t model.Tenant
	if err := gormDB().Order("id asc").First(&t).Error; err != nil {
		return 0
	}
	return t.ID
}
