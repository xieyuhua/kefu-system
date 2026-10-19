package customer

// 本文件是 core/customer 的 HTTP 层：仅保留订单 / 商品两个「跨端端点」。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。
//
// 跨端端点（客服端 modules/agent 的订单 / 商品选择器与后台端 modules/admin 的
// 订单 / 商品管理列表共用一个路由与一套分页参数）：
//   - GET /api/orders   客服端按 ?user_id= 取某客户的订单；后台端不带 user_id 即全量分页
//   - GET /api/products 客服端取商品卡片候选；后台端做商品管理分页
//
// 两端语义一致、参数兼容，复制到任一端都会立刻出现两份分页实现，故留在客户内核。

import (
	"kefu-server/internal/platform/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 订单 / 商品（跨端端点） ----------------

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
