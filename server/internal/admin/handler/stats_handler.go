package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 数据统计 ----------------

func (h *Handler) StatsOverviewHandler(c *gin.Context) {
	days := httpx.Default().QueryInt(c, "days", 7)
	data, err := h.svc.StatsOverviewData(authctx.TenantID(c), days)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, data)
}
