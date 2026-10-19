package admin

import (
	"kefu-server/internal/core/automation"
	"kefu-server/internal/platform/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 自动回复规则（关键词 → 机器人回复 / 转人工） ----------------

func (h *Handler) ListAutoRepliesHandler(c *gin.Context) {
	list, err := h.svc.auto.ListAutoReplies(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.auto.CreateAutoReply(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.auto.UpdateAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.auto.DeleteAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
