package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 客户资料与标签 ----------------

func (h *Handler) GetCustomerHandler(c *gin.Context) {
	detail, err := h.svc.Cust.GetCustomerDetail(authctx.TenantID(c), httpx.Default().UintParam(c, "id"))
	if err != nil {
		response.Default().NotFound(c, err.Error())
		return
	}
	response.Default().OK(c, detail)
}

// SetCustomerTagsHandler 给客户打/改标签（接待会话中即可操作，其他客服可见）
func (h *Handler) SetCustomerTagsHandler(c *gin.Context) {
	var req struct {
		TagIDs []uint `json:"tag_ids"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if req.TagIDs == nil {
		req.TagIDs = []uint{}
	}
	tags, err := h.svc.Cust.SetCustomerTags(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), authctx.UserID(c), req.TagIDs)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, tags)
}
