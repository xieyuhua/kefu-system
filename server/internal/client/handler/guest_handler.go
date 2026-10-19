package handler

// Package handler 用户端（client，访客）的 HTTP 层。
//
// handler 只做参数解析与响应包装：客户识别、会话定位、订单 / 商品卡片查询都在 service 层。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 internal/client/route 在注册时创建一次。
//
// 用户端接口：所有参数通过 GET query 传递，便于运营直接拼链接投放。
//
//	/kefu/?channel=web&uid=u_10001&nickname=王小明&order_no=SO...&pid=P10086
//
// 依赖方向：client/handler → client/service → common/service → common/repository → common/model
// → platform → pkg。本包不 import admin / agent 两端的任何包。

import (
	"errors"
	"net/http"
	"strings"

	"kefu-server/internal/client/service"
	"kefu-server/internal/common/dto"
	"kefu-server/internal/common/ipinfo"
	"kefu-server/internal/common/model"
	"kefu-server/internal/common/realtime"
	commonsvc "kefu-server/internal/common/service"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"
	"kefu-server/pkg/util"

	"github.com/gin-gonic/gin"
)

var errNoChannel = errors.New("渠道不存在")

// Handler 用户端的 HTTP 层，持有本模块 service。
type Handler struct {
	svc *service.Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler { return &Handler{svc: service.NewService()} }

// resolveChannel 解析用户端接入参数 channel（用户端页面与 WebSocket 共用）。
func (h *Handler) resolveChannel(c *gin.Context, tenantID uint) (*model.Channel, error) {
	ch, err := h.svc.ResolveGuestChannel(tenantID, c.Query("channel"))
	if err != nil {
		return nil, errNoChannel
	}
	return ch, nil
}

// guestTenantID 用户端未登录，租户由 ?tenant= 指定（缺省取第一个租户，便于演示）。
// 端内私有：其它端与内核的游客态租户口径见 common/service 的 DefaultTenantID。
func (h *Handler) guestTenantID(c *gin.Context) uint {
	if t := httpx.QueryUint(c, "tenant"); t > 0 {
		return t
	}
	return h.svc.DefaultTenantID()
}

// GuestInitHandler 用户端页面初始化：拿到渠道、客户、会话、欢迎语、上下文卡片
func (h *Handler) GuestInitHandler(c *gin.Context) {
	tenantID := h.guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	ch, err := h.resolveChannel(c, tenantID)
	if err != nil {
		response.Param(c, "渠道不存在或未启用：channel="+c.Query("channel"))
		return
	}
	uid := strings.TrimSpace(c.Query("uid"))
	if uid == "" {
		uid = "visitor_" + c.ClientIP() + "_" + ch.Type
	}

	// 找到或创建客户
	ip := util.ClientIP(c.Request.RemoteAddr, c.Request.Header)
	cu, err := h.svc.EnsureGuestCustomer(tenantID, ch, uid,
		c.Query("nickname"), c.Query("avatar"), c.Query("phone"), ip)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 定位或创建会话：同一客户始终复用最近一个会话（closed/archived 自动激活），
	// 保证同一 uid + 渠道的聊天记录自然合并
	conv, err := h.svc.Conv.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	// 上下文卡片：订单 / 商品
	orderCard := h.svc.LookupOrderCard(tenantID, strings.TrimSpace(c.Query("order_no")))
	productCard := h.svc.LookupProductCard(tenantID, strings.TrimSpace(c.Query("pid")))

	history, _ := h.svc.Conv.ListMessages(tenantID, conv.ID, 0, 20)

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
func (h *Handler) GuestSendHandler(c *gin.Context) {
	tenantID := h.guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	ch, err := h.resolveChannel(c, tenantID)
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

	cu, err := h.svc.FindGuestCustomer(tenantID, ch.ID, strings.TrimSpace(c.Query("uid")))
	if err != nil {
		response.Param(c, "请先调用 /api/guest/init 初始化会话")
		return
	}
	conv, err := h.svc.Conv.FindOrCreateConversation(tenantID, cu.ID, ch.ID)
	if err != nil {
		response.Server(c, err.Error())
		return
	}

	mvo, cvo, err := h.svc.Conv.ReceiveMessage(tenantID, conv.ID, cu.ID, req.Type, req.Content)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	ipinfo.RecordCustomerIP(cu.ID, util.ClientIP(c.Request.RemoteAddr, c.Request.Header))
	realtime.PublishMessage(tenantID, conv.ID, mvo, cvo)

	// 同步判定自动回复 / 转人工（供接口直接返回）
	transferred := false
	reply := h.svc.AutoReply.ReplyNow(tenantID, conv.ID, cu.ID, ch.ID, req.Content)
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
func (h *Handler) GuestMessagesHandler(c *gin.Context) {
	tenantID := h.guestTenantID(c)
	if tenantID == 0 {
		response.Param(c, "缺少租户参数 tenant")
		return
	}
	cu, err := h.svc.FindGuestCustomerByExternalID(tenantID, c.Query("uid"))
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	conv, err := h.svc.LatestConversationOfCustomer(tenantID, cu.ID)
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
		list, err = h.svc.Conv.ListMessages(tenantID, conv.ID, beforeID, limit)
	case sinceID > 0:
		list, err = h.svc.Conv.ListMessagesSince(tenantID, conv.ID, sinceID, limit)
	default:
		// 首屏：取最新的 limit 条（beforeID 传 0 即不带 id < 条件）
		list, err = h.svc.Conv.ListMessages(tenantID, conv.ID, 0, limit)
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
func (h *Handler) GuestWSHandler(c *gin.Context) {
	tenantID := h.guestTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少租户参数 tenant"})
		return
	}
	ch, err := h.resolveChannel(c, tenantID)
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
	cu, convID, err := h.svc.FindGuestConversation(tenantID, ch.ID, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error()})
		return
	}

	conn, err := commonsvc.Upgrader.Upgrade(c.Writer, c.Request, nil)
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
	if msgs, err := h.svc.Conv.ListMessagesSince(tenantID, convID, httpx.QueryUint(c, "since_id"), 200); err == nil && len(msgs) > 0 {
		for _, m := range msgs {
			client.SendEvent("message.new", m)
		}
	}

	go client.WritePump()
	go client.ReadPump()
}
