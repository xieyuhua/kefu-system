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

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Result{Code: CodeOK, Message: "ok", Data: data})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Result{Code: code, Message: msg, Data: nil})
}

func Param(c *gin.Context, msg string)    { Fail(c, CodeParam, msg) }
func Unauth(c *gin.Context, msg string)   { Fail(c, CodeUnauth, msg) }
func Forbid(c *gin.Context, msg string)   { Fail(c, CodeForbid, msg) }
func NotFound(c *gin.Context, msg string) { Fail(c, CodeNotFound, msg) }
func Server(c *gin.Context, msg string)   { Fail(c, CodeServer, msg) }

// Paged 分页响应
type Paged struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

func OKPaged(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	OK(c, Paged{List: list, Total: total, Page: page, PageSize: pageSize})
}
