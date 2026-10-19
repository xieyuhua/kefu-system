package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 离线消息 / 离线提醒 ----------------

func (h *Handler) OfflineMessagesHandler(c *gin.Context) {
	list, latest, hasMore, err := h.svc.OfflineMessages(authctx.TenantID(c), authctx.UserID(c), httpx.Default().QueryUint(c, "since_id"), httpx.Default().QueryInt(c, "limit", 200))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
		"has_more":  hasMore,
	})
}

func (h *Handler) OfflineNotificationsHandler(c *gin.Context) {
	list, latest, err := h.notify.OfflineNotifications(authctx.TenantID(c), authctx.UserID(c), httpx.Default().QueryUint(c, "since_id"))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
	})
}

func (h *Handler) ReadNotificationsHandler(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.notify.MarkNotificationsRead(authctx.TenantID(c), authctx.UserID(c), req.IDs); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
