package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 设备（APP / PC 桌面端推送） ----------------

func (h *Handler) RegisterDeviceHandler(c *gin.Context) {
	var req dto.DeviceRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.notify.RegisterDevice(authctx.TenantID(c), authctx.UserID(c), req)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) ListDevicesHandler(c *gin.Context) {
	list, err := h.notify.ListDevices(authctx.TenantID(c), authctx.UserID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) DeleteDeviceHandler(c *gin.Context) {
	if err := h.notify.DeleteDevice(authctx.TenantID(c), authctx.UserID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
