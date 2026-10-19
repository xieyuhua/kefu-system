package handler

import (
	"errors"
	"strings"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// 用户端（客户端）接口：所有参数通过 GET query 传递，便于运营直接拼链接投放。
//   /c?channel=web&uid=u_10001&nickname=王小明&order_no=SO...&pid=P10086
//
// 本文件只做参数解析与响应包装：客户识别、会话定位、订单 / 商品卡片查询都在 service 层。

var errNoChannel = errors.New("渠道不存在")

// resolveChannel 解析用户端接入参数 channel（用户端页面与 WebSocket 共用）。
func resolveChannel(c *gin.Context, tenantID uint) (*model.Channel, error) {
	ch, err := service.ResolveGuestChannel(tenantID, c.Query("channel"))
	if err != nil {
		return nil, errNoChannel
	}
	return ch, nil
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
	ip := service.ClientIP(c.Request.RemoteAddr, c.Request.Header)
	cu, err := service.EnsureGuestCustomer(tenantID, ch, uid,
		c.Query("nickname"), c.Query("avatar"), c.Query("phone"), ip)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 定位或创建会话：同一客户始终复用最近一个会话（closed/archived 自动激活），
	// 保证同一 uid + 渠道的聊天记录自然合并
	conv, err := service.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 上下文卡片：订单 / 商品
	orderCard := service.LookupOrderCard(tenantID, strings.TrimSpace(c.Query("order_no")))
	productCard := service.LookupProductCard(tenantID, strings.TrimSpace(c.Query("pid")))

	history, _ := service.ListMessages(tenantID, conv.ID, 0, 20)

	// 每次访问都刷新 IP 与归属城市（展示在客服工作台的会话列表）
	service.RecordCustomerIP(cu.ID, ip)

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

	cu, err := service.FindGuestCustomer(tenantID, ch.ID, strings.TrimSpace(c.Query("uid")))
	if err != nil {
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
	cu, err := service.FindGuestCustomerByExternalID(tenantID, c.Query("uid"))
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	conv, err := service.LatestConversationOfCustomer(tenantID, cu.ID)
	if err != nil {
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
	return service.DefaultTenantID()
}
