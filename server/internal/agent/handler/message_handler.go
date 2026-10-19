package handler

import (
	"strings"

	"kefu-server/pkg/authctx"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// SearchConversationMessagesHandler 在会话内按关键词搜索历史消息
func (h *Handler) SearchConversationMessagesHandler(c *gin.Context) {
	convID := httpx.Default().UintParam(c, "id")
	keyword := c.Query("keyword")
	if strings.TrimSpace(keyword) == "" {
		response.Default().OK(c, []interface{}{})
		return
	}
	rows, err := h.svc.Conv.SearchMessages(authctx.TenantID(c), convID, keyword, httpx.Default().QueryInt(c, "limit", 50))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	out := make([]interface{}, 0, len(rows))
	for _, m := range rows {
		out = append(out, h.svc.Conv.BuildMessageVO(m))
	}
	response.Default().OK(c, out)
}

// ---------------- 消息 ----------------

func (h *Handler) ListMessagesHandler(c *gin.Context) {
	list, err := h.svc.Conv.ListMessages(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), httpx.Default().QueryUint(c, "before_id"), httpx.Default().QueryInt(c, "limit", 50))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) SendMessageHandler(c *gin.Context) {
	var req dto.SendMessageRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	mvo, cvo, err := h.svc.Conv.SendMessage(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), authctx.UserID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	realtime.PublishMessage(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), mvo, cvo)
	response.Default().OK(c, mvo)
}
