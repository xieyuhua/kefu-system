package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 演示入口 ----------------

func (h *Handler) DemoIncomingHandler(c *gin.Context) {
	// 模拟新咨询仅用于演示/自测，只允许管理员调用（子账号工作台不显示该入口）
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可模拟新咨询")
		return
	}
	var req dto.DemoIncomingRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	cvo, _, err := h.svc.DemoIncoming(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, cvo)
}

func (h *Handler) DemoProductsHandler(c *gin.Context) {
	paged, err := h.svc.Cust.ListProducts(authctx.TenantID(c), c.Query("keyword"), 1, 50)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, paged.List)
}
