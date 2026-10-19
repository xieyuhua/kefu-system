package agent

// 本文件是客服端（modules/agent）的 HTTP 层：会话与消息、客服端 WebSocket、
// 客户资料与标签、离线同步与设备、演示面板。
// handler 层不持有数据库连接：参数绑定 → 调本端 service 或 core/* 的 service → 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 route 在 Setup 里创建一次。
//
// 依赖方向：modules/agent → core/* → platform → pkg。
// 本包不 import modules/client、modules/admin，也不被 core/* import。
//
// 会话/消息、客户资料、渠道准入这些可复用能力全在 core/*，端内只保留
// 「客服端专属的编排」（见 service.go：设备推送注册、离线补拉、演示入口）。

import (
	"net/http"
	"strings"

	"kefu-server/internal/core/conversation"
	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 客服端的 HTTP 层，持有本端 service（service 内已建好各内核依赖）。
type Handler struct {
	svc *Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler { return &Handler{svc: NewService()} }

// ---------------- 会话 ----------------

func (h *Handler) ListConversationsHandler(c *gin.Context) {
	q := conversation.ConvQuery{
		ChannelID:       httpx.QueryUint(c, "channel_id"),
		Status:          c.Query("status"),
		Scope:           c.Query("scope"),
		Keyword:         c.Query("keyword"),
		Page:            httpx.QueryInt(c, "page", 1),
		PageSize:        httpx.QueryInt(c, "page_size", 20),
		IncludeArchived: c.Query("include_archived") == "1" || c.Query("include_archived") == "true",
	}
	paged, err := h.svc.conv.ListConversations(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func (h *Handler) GetConversationHandler(c *gin.Context) {
	vo, err := h.svc.conv.GetConversation(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) ClaimConversationHandler(c *gin.Context) {
	vo, err := h.svc.conv.ClaimConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func (h *Handler) TransferConversationHandler(c *gin.Context) {
	var req struct {
		AgentID uint `json:"agent_id"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.conv.TransferConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req.AgentID)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func (h *Handler) CloseConversationHandler(c *gin.Context) {
	vo, err := h.svc.conv.CloseConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func (h *Handler) MarkReadHandler(c *gin.Context) {
	if err := h.svc.conv.MarkRead(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) ConversationStatsHandler(c *gin.Context) {
	s, err := h.svc.conv.ConversationStats(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, s)
}

// ArchiveConversationHandler 归档会话（客服清理工作台列表；客户再咨询会自动恢复）
func (h *Handler) ArchiveConversationHandler(c *gin.Context) {
	if err := h.svc.conv.ArchiveConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c)); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ArchiveConversationsBulkHandler 一键清空工作台列表（批量归档）
// POST /api/conversations/archive-bulk  body/query 同列表接口的过滤条件
// 只处理「当前筛选条件下可见」的会话，保证所见即所清。
func (h *Handler) ArchiveConversationsBulkHandler(c *gin.Context) {
	q := conversation.ConvQuery{
		ChannelID: httpx.QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		Scope:     c.Query("scope"),
		Keyword:   c.Query("keyword"),
		Page:      httpx.QueryInt(c, "page", 1),
		PageSize:  httpx.QueryInt(c, "page_size", 20),
	}
	n, err := h.svc.conv.ArchiveConversationsBulk(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, gin.H{"archived": n})
}

// UnarchiveConversationHandler 取消归档
func (h *Handler) UnarchiveConversationHandler(c *gin.Context) {
	if err := h.svc.conv.UnarchiveConversation(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// DeleteConversationHandler 彻底删除会话与消息（不可恢复）
func (h *Handler) DeleteConversationHandler(c *gin.Context) {
	if err := h.svc.conv.DeleteConversation(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// SearchConversationMessagesHandler 在会话内按关键词搜索历史消息
func (h *Handler) SearchConversationMessagesHandler(c *gin.Context) {
	convID := httpx.UintParam(c, "id")
	keyword := c.Query("keyword")
	if strings.TrimSpace(keyword) == "" {
		response.OK(c, []interface{}{})
		return
	}
	rows, err := h.svc.conv.SearchMessages(authctx.TenantID(c), convID, keyword, httpx.QueryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	out := make([]interface{}, 0, len(rows))
	for _, m := range rows {
		out = append(out, h.svc.conv.BuildMessageVO(m))
	}
	response.OK(c, out)
}

// ---------------- 消息 ----------------

func (h *Handler) ListMessagesHandler(c *gin.Context) {
	list, err := h.svc.conv.ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), httpx.QueryUint(c, "before_id"), httpx.QueryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) SendMessageHandler(c *gin.Context) {
	var req dto.SendMessageRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	mvo, cvo, err := h.svc.conv.SendMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.PublishMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), mvo, cvo)
	response.OK(c, mvo)
}

// ---------------- 客服端 WebSocket ----------------

// WSHandler 客服端实时通道：Web 工作台 / PC 桌面端 / APP 共用。
// 连接时通过 ?token= 鉴权，?device=web|desktop|app 标识端类型。
func (h *Handler) WSHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Sec-WebSocket-Protocol")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	claims, err := auth.ParseToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "登录已失效"})
		return
	}

	device := c.Query("device")
	switch device {
	case "desktop", "app", "guest":
	default:
		device = "web"
	}

	conn, err := conversation.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := realtime.NewClient(conn, claims.UserID, claims.TenantID, device)
	realtime.HubInstance().Add(client)

	go client.WritePump()
	go client.ReadPump()
}

// ---------------- 客户资料与标签 ----------------

func (h *Handler) GetCustomerHandler(c *gin.Context) {
	detail, err := h.svc.cust.GetCustomerDetail(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, detail)
}

// SetCustomerTagsHandler 给客户打/改标签（接待会话中即可操作，其他客服可见）
func (h *Handler) SetCustomerTagsHandler(c *gin.Context) {
	var req struct {
		TagIDs []uint `json:"tag_ids"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if req.TagIDs == nil {
		req.TagIDs = []uint{}
	}
	tags, err := h.svc.cust.SetCustomerTags(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req.TagIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, tags)
}

// ---------------- 设备（APP / PC 桌面端推送） ----------------

func (h *Handler) RegisterDeviceHandler(c *gin.Context) {
	var req dto.DeviceRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.RegisterDevice(authctx.TenantID(c), authctx.UserID(c), req)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) ListDevicesHandler(c *gin.Context) {
	list, err := h.svc.ListDevices(authctx.TenantID(c), authctx.UserID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) DeleteDeviceHandler(c *gin.Context) {
	if err := h.svc.DeleteDevice(authctx.TenantID(c), authctx.UserID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 离线消息 / 离线提醒 ----------------

func (h *Handler) OfflineMessagesHandler(c *gin.Context) {
	list, latest, hasMore, err := h.svc.OfflineMessages(authctx.TenantID(c), authctx.UserID(c), httpx.QueryUint(c, "since_id"), httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
		"has_more":  hasMore,
	})
}

func (h *Handler) OfflineNotificationsHandler(c *gin.Context) {
	list, latest, err := h.svc.OfflineNotifications(authctx.TenantID(c), authctx.UserID(c), httpx.QueryUint(c, "since_id"))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
	})
}

func (h *Handler) ReadNotificationsHandler(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.MarkNotificationsRead(authctx.TenantID(c), authctx.UserID(c), req.IDs); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 演示入口 ----------------

func (h *Handler) DemoIncomingHandler(c *gin.Context) {
	var req dto.DemoIncomingRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	cvo, _, err := h.svc.DemoIncoming(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, cvo)
}

func (h *Handler) DemoProductsHandler(c *gin.Context) {
	paged, err := h.svc.cust.ListProducts(authctx.TenantID(c), c.Query("keyword"), 1, 50)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged.List)
}
