package guest

// 本文件是 guest 模块的 HTTP 层（原 handler/guest.go + handler/ws.go 的访客通道）。
// handler 只做参数解析与响应包装：客户识别、会话定位、订单 / 商品卡片查询都在 service 层。
//
// 用户端（客户端）接口：所有参数通过 GET query 传递，便于运营直接拼链接投放。
//   /c?channel=web&uid=u_10001&nickname=王小明&order_no=SO...&pid=P10086

import (
	"errors"
	"net/http"
	"strings"

	"kefu-server/internal/modules/automation"
	"kefu-server/internal/modules/chat"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/httpx"
	"kefu-server/internal/platform/ipinfo"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/response"

	"github.com/gin-gonic/gin"
)

var errNoChannel = errors.New("渠道不存在")

// resolveChannel 解析用户端接入参数 channel（用户端页面与 WebSocket 共用）。
func resolveChannel(c *gin.Context, tenantID uint) (*model.Channel, error) {
	ch, err := ResolveGuestChannel(tenantID, c.Query("channel"))
	if err != nil {
		return nil, errNoChannel
	}
	return ch, nil
}

// TenantID 用户端未登录，租户由 ?tenant= 指定（缺省取第一个租户，便于演示）。
// 导出给 upload 模块复用（上传接口在游客态下也要落 tenant_id）。
func TenantID(c *gin.Context) uint {
	if t := httpx.QueryUint(c, "tenant"); t > 0 {
		return t
	}
	return DefaultTenantID()
}

// GuestInitHandler 用户端页面初始化：拿到渠道、客户、会话、欢迎语、上下文卡片
func GuestInitHandler(c *gin.Context) {
	tenantID := TenantID(c)
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
	ip := ipinfo.ClientIP(c.Request.RemoteAddr, c.Request.Header)
	cu, err := EnsureGuestCustomer(tenantID, ch, uid,
		c.Query("nickname"), c.Query("avatar"), c.Query("phone"), ip)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 定位或创建会话：同一客户始终复用最近一个会话（closed/archived 自动激活），
	// 保证同一 uid + 渠道的聊天记录自然合并
	conv, err := chat.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 上下文卡片：订单 / 商品
	orderCard := LookupOrderCard(tenantID, strings.TrimSpace(c.Query("order_no")))
	productCard := LookupProductCard(tenantID, strings.TrimSpace(c.Query("pid")))

	history, _ := chat.ListMessages(tenantID, conv.ID, 0, 20)

	// 每次访问都刷新 IP 与归属城市（展示在客服工作台的会话列表）
	ipinfo.RecordCustomerIP(cu.ID, ip)

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

// GuestSendHandler 用户端发消息：走自动回复/转人工引擎
func GuestSendHandler(c *gin.Context) {
	tenantID := TenantID(c)
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
	if !httpx.BindJSON(c, &req) {
		return
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Content == "" {
		response.Param(c, "消息内容不能为空")
		return
	}

	cu, err := FindGuestCustomer(tenantID, ch.ID, strings.TrimSpace(c.Query("uid")))
	if err != nil {
		response.Param(c, "请先调用 /api/guest/init 初始化会话")
		return
	}
	conv, err := chat.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	mvo, cvo, err := chat.ReceiveMessage(tenantID, conv.ID, cu.ID, req.Type, req.Content)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	ipinfo.RecordCustomerIP(cu.ID, ipinfo.ClientIP(c.Request.RemoteAddr, c.Request.Header))
	realtime.PublishMessage(tenantID, conv.ID, mvo, cvo)

	// 同步判定自动回复 / 转人工（供接口直接返回）
	transferred := false
	reply := automation.ReplyNow(tenantID, conv.ID, cu.ID, ch.ID, req.Content)
	if reply != nil {
		transferred = reply.Transferred
	}

	out := gin.H{"message": mvo, "transferred": transferred}
	if reply != nil && reply.Message != nil {
		out["reply"] = reply.Message
	}
	response.OK(c, out)
}

// GuestMessagesHandler 无 WebSocket 环境的降级轮询
func GuestMessagesHandler(c *gin.Context) {
	tenantID := TenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	cu, err := FindGuestCustomerByExternalID(tenantID, c.Query("uid"))
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	conv, err := LatestConversationOfCustomer(tenantID, cu.ID)
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	// 三种拉取模式，游标方向各不相同：
	//   before_id > 0  → 向前翻页，返回 id < before_id 的**更早**消息（上滑加载历史）
	//   since_id  > 0  → 正向增量，返回 id > since_id 的**更新**消息（轮询补拉）
	//   两个都不传      → 首屏，返回**最新** limit 条
	beforeID := httpx.QueryUint(c, "before_id")
	sinceID := httpx.QueryUint(c, "since_id")
	limit := httpx.QueryInt(c, "limit", 20)

	var list []dto.MessageVO
	switch {
	case beforeID > 0:
		list, err = chat.ListMessages(tenantID, conv.ID, beforeID, limit)
	case sinceID > 0:
		list, err = chat.ListMessagesSince(tenantID, conv.ID, sinceID, limit)
	default:
		// 首屏：取最新的 limit 条（beforeID 传 0 即不带 id < 条件）
		list, err = chat.ListMessages(tenantID, conv.ID, 0, limit)
	}
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// GuestWSHandler 用户端（访客）实时通道，无需登录，凭 channel + uid 定位会话。
// 连接后立即回放「离线期间累积的消息」（since_id 之后），保证不在线也不丢消息。
//
//	GET /ws/guest?channel=web&uid=u_10001&since_id=0&tenant=1
func GuestWSHandler(c *gin.Context) {
	tenantID := TenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少租户参数 tenant"})
		return
	}
	ch, err := resolveChannel(c, tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "渠道不存在"})
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少参数 uid"})
		return
	}

	// 定位客户与会话（复用 GuestInit 的查找逻辑）
	cu, convID, err := FindGuestConversation(tenantID, ch.ID, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
		return
	}

	conn, err := chat.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := realtime.NewGuestClient(conn, tenantID, convID, "guest")
	hub := realtime.HubInstance()
	hub.AddGuest(client)

	// ① 连接确认
	client.SendEvent("connected", map[string]interface{}{
		"conversation_id": convID,
		"customer_id":     cu.ID,
		"nickname":        cu.Nickname,
		"avatar":          cu.Avatar,
		"channel":         map[string]interface{}{"id": ch.ID, "type": ch.Type, "name": ch.Name},
	})

	// ② 离线消息回放：把 since_id 之后的消息一次性推给该端
	if msgs, err := chat.ListMessagesSince(tenantID, convID, httpx.QueryUint(c, "since_id"), 200); err == nil && len(msgs) > 0 {
		for _, m := range msgs {
			client.SendEvent("message.new", m)
		}
	}

	go client.WritePump()
	go client.ReadPump()
}
