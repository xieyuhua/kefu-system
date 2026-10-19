// Package response 统一响应封装：HTTP 状态恒为 200，业务错误看 code。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewWriter 与单例访问器 Default，
// 业务能力一律是 *Writer 的指针接收者方法；调用点写 response.Default().OK(c, data)。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Result 统一响应结构。HTTP 状态码恒为 200，业务错误看 code。
type Result struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

const (
	CodeOK       = 0
	CodeParam    = 400
	CodeUnauth   = 401
	CodeForbid   = 403
	CodeNotFound = 404
	CodeServer   = 500
)

// Writer 响应写出器。无状态，所有响应都是它的方法。
type Writer struct{}

// NewWriter 创建响应写出器实例。
func NewWriter() *Writer { return &Writer{} }

// defaultWriter 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultWriter = NewWriter()

// Default 返回包级默认实例。
func Default() *Writer { return defaultWriter }

// OK 成功响应
func (w *Writer) OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Result{Code: CodeOK, Message: "ok", Data: data})
}

// Fail 业务失败响应（HTTP 状态仍为 200）
func (w *Writer) Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Result{Code: code, Message: msg, Data: nil})
}

// Param 参数错误
func (w *Writer) Param(c *gin.Context, msg string) { w.Fail(c, CodeParam, msg) }

// Unauth 未登录 / token 失效
func (w *Writer) Unauth(c *gin.Context, msg string) { w.Fail(c, CodeUnauth, msg) }

// Forbid 无权限
func (w *Writer) Forbid(c *gin.Context, msg string) { w.Fail(c, CodeForbid, msg) }

// NotFound 资源不存在
func (w *Writer) NotFound(c *gin.Context, msg string) { w.Fail(c, CodeNotFound, msg) }

// Server 服务端错误
func (w *Writer) Server(c *gin.Context, msg string) { w.Fail(c, CodeServer, msg) }

// Paged 分页响应
type Paged struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// OKPaged 分页成功响应
func (w *Writer) OKPaged(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	w.OK(c, Paged{List: list, Total: total, Page: page, PageSize: pageSize})
}
