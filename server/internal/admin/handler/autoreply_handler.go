package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 自动回复规则（关键词 → 机器人回复 / 转人工） ----------------

func (h *Handler) ListAutoRepliesHandler(c *gin.Context) {
	list, err := h.svc.AutoReply.ListAutoReplies(authctx.TenantID(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.AutoReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.AutoReply.CreateAutoReply(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.AutoReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.AutoReply.UpdateAutoReply(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.AutoReply.DeleteAutoReply(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
