package handler

// 本文件是公共 HTTP 层里的客户部分：仅保留订单 / 商品两个「跨端端点」。
// handler 层不持有数据库连接：参数绑定 → 调公共 service → 写响应。
// 所有处理器都是 *CustomerHandler 的方法（指针接收者），实例由 internal/common/route 在注册时创建一次。
//
// 跨端端点（客服端 agent 的订单 / 商品选择器与后台端 admin 的
// 订单 / 商品管理列表共用一个路由与一套分页参数）：
//   - GET /api/orders   客服端按 ?user_id= 取某客户的订单；后台端不带 user_id 即全量分页
//   - GET /api/products 客服端取商品卡片候选；后台端做商品管理分页
//
// 两端语义一致、参数兼容，复制到任一端都会立刻出现两份分页实现，故留在公共层。

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	"kefu-server/internal/common/service"
)

// CustomerHandler 公共客户服务的 HTTP 层。
type CustomerHandler struct {
	svc *service.CustomerService
}

// NewCustomerHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewCustomerHandler() *CustomerHandler {
	return &CustomerHandler{svc: service.NewCustomerService()}
}

// ---------------- 订单 / 商品（跨端端点） ----------------

func (h *CustomerHandler) ListOrdersHandler(c *gin.Context) {
	paged, err := h.svc.ListOrders(authctx.TenantID(c), httpx.Default().QueryUint(c, "user_id"), c.Query("keyword"),
		httpx.Default().QueryInt(c, "page", 1), httpx.Default().QueryInt(c, "page_size", 20))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, paged)
}

func (h *CustomerHandler) ListProductsHandler(c *gin.Context) {
	paged, err := h.svc.ListProducts(authctx.TenantID(c), c.Query("keyword"),
		httpx.Default().QueryInt(c, "page", 1), httpx.Default().QueryInt(c, "page_size", 20))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, paged)
}
