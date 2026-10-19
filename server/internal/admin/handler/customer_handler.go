package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 客户 / 历史档案 ----------------

func (h *Handler) ListCustomersHandler(c *gin.Context) {
	paged, err := h.svc.Cust.ListCustomers(authctx.TenantID(c), c.Query("keyword"), httpx.Default().QueryUint(c, "channel_id"),
		httpx.Default().QueryInt(c, "page", 1), httpx.Default().QueryInt(c, "page_size", 20))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, paged)
}

// ---------------- 客户标签 ----------------

func (h *Handler) ListTagsHandler(c *gin.Context) {
	list, err := h.svc.Cust.ListTags(authctx.TenantID(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateTagHandler(c *gin.Context) {
	var req commonsvc.TagRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Cust.CreateTag(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) DeleteTagHandler(c *gin.Context) {
	if err := h.svc.Cust.DeleteTag(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) UpdateCustomerRemarkHandler(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.Cust.UpdateCustomerRemark(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req.Remark); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
