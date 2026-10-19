package customer

// 本文件是 customer 模块的 HTTP 层
// （原 handler/customer.go 的客户 / 订单 / 商品部分 + handler/conversation.go 的客户标签部分）。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。

import (
	"kefu-server/internal/platform/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 客户 / 历史档案 ----------------

func ListCustomersHandler(c *gin.Context) {
	paged, err := ListCustomers(authctx.TenantID(c), c.Query("keyword"), httpx.QueryUint(c, "channel_id"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func GetCustomerHandler(c *gin.Context) {
	detail, err := GetCustomerDetail(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, detail)
}

// ---------------- 订单 / 商品 ----------------

func ListOrdersHandler(c *gin.Context) {
	paged, err := ListOrders(authctx.TenantID(c), httpx.QueryUint(c, "user_id"), c.Query("keyword"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func ListProductsHandler(c *gin.Context) {
	paged, err := ListProducts(authctx.TenantID(c), c.Query("keyword"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

// ---------------- 客户标签 ----------------

func ListTagsHandler(c *gin.Context) {
	list, err := ListTags(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateTagHandler(c *gin.Context) {
	var req TagRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := CreateTag(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func DeleteTagHandler(c *gin.Context) {
	if err := DeleteTag(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// SetCustomerTagsHandler 给客户打/改标签（接待会话中即可操作，其他客服可见）
func SetCustomerTagsHandler(c *gin.Context) {
	var req struct {
		TagIDs []uint `json:"tag_ids"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if req.TagIDs == nil {
		req.TagIDs = []uint{}
	}
	tags, err := SetCustomerTags(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req.TagIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, tags)
}

func UpdateCustomerRemarkHandler(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := UpdateCustomerRemark(authctx.TenantID(c), httpx.UintParam(c, "id"), req.Remark); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
