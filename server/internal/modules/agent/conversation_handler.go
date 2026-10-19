package agent

import (
	"kefu-server/internal/core/conversation"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

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
