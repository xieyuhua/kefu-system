package admin

import (
	"kefu-server/internal/core/customer"
	"kefu-server/internal/platform/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 客户 / 历史档案 ----------------

func (h *Handler) ListCustomersHandler(c *gin.Context) {
	paged, err := h.svc.cust.ListCustomers(authctx.TenantID(c), c.Query("keyword"), httpx.QueryUint(c, "channel_id"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

// ---------------- 客户标签 ----------------

func (h *Handler) ListTagsHandler(c *gin.Context) {
	list, err := h.svc.cust.ListTags(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateTagHandler(c *gin.Context) {
	var req customer.TagRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.cust.CreateTag(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) DeleteTagHandler(c *gin.Context) {
	if err := h.svc.cust.DeleteTag(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) UpdateCustomerRemarkHandler(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.cust.UpdateCustomerRemark(authctx.TenantID(c), httpx.UintParam(c, "id"), req.Remark); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
