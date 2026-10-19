package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 演示入口 ----------------

func (h *Handler) DemoIncomingHandler(c *gin.Context) {
	var req dto.DemoIncomingRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	cvo, _, err := h.svc.DemoIncoming(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, cvo)
}

func (h *Handler) DemoProductsHandler(c *gin.Context) {
	paged, err := h.svc.Cust.ListProducts(authctx.TenantID(c), c.Query("keyword"), 1, 50)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged.List)
}
