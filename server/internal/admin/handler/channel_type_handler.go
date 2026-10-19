package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func (h *Handler) ListChannelTypesHandler(c *gin.Context) {
	list, err := h.svc.Channels.ListChannelTypes(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateChannelTypeHandler(c *gin.Context) {
	var req commonsvc.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Channels.CreateChannelType(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateChannelTypeHandler(c *gin.Context) {
	var req commonsvc.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Channels.UpdateChannelType(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteChannelTypeHandler(c *gin.Context) {
	if err := h.svc.Channels.DeleteChannelType(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}
