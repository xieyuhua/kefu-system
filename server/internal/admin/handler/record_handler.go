package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// SearchConversationRecordsHandler 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=

// ---------------- 对话记录（全局检索 / 只读回放） ----------------

func (h *Handler) SearchConversationRecordsHandler(c *gin.Context) {
	q := commonsvc.ConversationRecordsQuery{
		Keyword:   c.Query("keyword"),
		ChannelID: httpx.QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		AgentID:   httpx.QueryUint(c, "agent_id"),
		Start:     c.Query("start"),
		End:       c.Query("end"),
		Page:      httpx.QueryInt(c, "page", 1),
		PageSize:  httpx.QueryInt(c, "page_size", 20),
	}
	list, total, err := h.svc.Conv.SearchConversations(authctx.TenantID(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if list == nil {
		list = []commonsvc.ConversationRecord{}
	}
	response.OKPaged(c, list, total, q.Page, q.PageSize)
}

// GetConversationMessagesHandler 管理后台查看某个会话的完整消息（只读）
func (h *Handler) GetConversationMessagesHandler(c *gin.Context) {
	list, err := h.svc.Conv.ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), 0, httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}
