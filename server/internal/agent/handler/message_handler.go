package handler

import (
	"strings"

	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// SearchConversationMessagesHandler 在会话内按关键词搜索历史消息
func (h *Handler) SearchConversationMessagesHandler(c *gin.Context) {
	convID := httpx.UintParam(c, "id")
	keyword := c.Query("keyword")
	if strings.TrimSpace(keyword) == "" {
		response.OK(c, []interface{}{})
		return
	}
	rows, err := h.svc.Conv.SearchMessages(authctx.TenantID(c), convID, keyword, httpx.QueryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	out := make([]interface{}, 0, len(rows))
	for _, m := range rows {
		out = append(out, h.svc.Conv.BuildMessageVO(m))
	}
	response.OK(c, out)
}

// ---------------- 消息 ----------------

func (h *Handler) ListMessagesHandler(c *gin.Context) {
	list, err := h.svc.Conv.ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), httpx.QueryUint(c, "before_id"), httpx.QueryInt(c, "limit", 50))
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
	mvo, cvo, err := h.svc.Conv.SendMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.PublishMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), mvo, cvo)
	response.OK(c, mvo)
}
