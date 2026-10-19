package admin

import (
	"kefu-server/internal/platform/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 数据统计 ----------------

func (h *Handler) StatsOverviewHandler(c *gin.Context) {
	days := httpx.QueryInt(c, "days", 7)
	data, err := h.svc.StatsOverviewData(authctx.TenantID(c), days)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, data)
}
