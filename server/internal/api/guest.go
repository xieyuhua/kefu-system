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

// resolveChannel 按渠道类型或 ID 解析渠道（GET 参数 channel）
func resolveChannel(c *gin.Context, tenantID uint) (*model.Channel, error) {
	key := strings.TrimSpace(c.Query("channel"))
	if key == "" {
		return nil, errors.New("缺少渠道参数 channel")
	}
	var ch model.Channel
	q := db.Where("tenant_id = ? AND (type = ? OR id = ?)", tenantID, key, key)
	if err := q.First(&ch).Error; err != nil {
		return nil, errNoChannel
	}
	return &ch, nil
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
			LastSeenAt: &now,
		}
		gormDB().Create(&cu)
	}

	// 最近未结束的会话优先复用，否则新建
	var conv model.Conversation
	if err := gormDB().Where("tenant_id = ? AND customer_id = ? AND status != 'closed'",
		tenantID, cu.ID).Order("id desc").First(&conv).Error; err != nil {
		now := time.Now()
		conv = model.Conversation{
			TenantID: tenantID, ChannelID: ch.ID, CustomerID: cu.ID,
			Status: "pending", LastMessageAt: now,
		}
		gormDB().Create(&conv)
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

	response.OK(c, gin.H{
		"channel":      gin.H{"id": ch.ID, "type": ch.Type, "name": ch.Name},
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
	var conv model.Conversation
	if err := gormDB().Where("tenant_id = ? AND customer_id = ? AND status != 'closed'",
		tenantID, cu.ID).Order("id desc").First(&conv).Error; err != nil {
		response.Param(c, "会话不存在")
		return
	}

	mvo, cvo, err := service.ReceiveMessage(tenantID, conv.ID, cu.ID, req.Type, req.Content)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	service.HubInstance().Publish(tenantID, nil, service.Event{Event: "message.new", Data: mvo})
	service.HubInstance().Publish(tenantID, nil, service.Event{Event: "conversation.updated", Data: cvo})

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
	list, err := service.ListMessages(tenantID, conv.ID, queryUint(c, "since_id"), 50)
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
