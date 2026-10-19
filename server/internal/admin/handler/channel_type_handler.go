package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func (h *Handler) ListChannelTypesHandler(c *gin.Context) {
	list, err := h.svc.Channels.ListChannelTypes(authctx.TenantID(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateChannelTypeHandler(c *gin.Context) {
	var req commonsvc.ChannelTypeRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Channels.CreateChannelType(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateChannelTypeHandler(c *gin.Context) {
	var req commonsvc.ChannelTypeRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.Channels.UpdateChannelType(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteChannelTypeHandler(c *gin.Context) {
	if err := h.svc.Channels.DeleteChannelType(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
