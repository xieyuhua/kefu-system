// Package httpx 提供与业务无关的 gin 辅助：路径/查询参数解析与 JSON 绑定。
//
// 本包属于通用层（pkg），不得依赖 internal 下的任何包；
// 依赖登录态的鉴权辅助见 internal/platform/authctx。
package httpx

import (
	"strconv"

	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// UintParam 解析路径参数为 uint；解析失败返回 0
func UintParam(c *gin.Context, key string) uint {
	v, _ := strconv.ParseUint(c.Param(key), 10, 64)
	return uint(v)
}

// QueryInt 解析整型查询参数；缺省或非法时返回 def
func QueryInt(c *gin.Context, key string, def int) int {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// QueryUint 解析无符号整型查询参数；缺省或非法时返回 0
func QueryUint(c *gin.Context, key string) uint {
	s := c.Query(key)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return uint(v)
}

// BindJSON 绑定请求体；失败时直接写回「参数格式错误」并返回 false
func BindJSON(c *gin.Context, out interface{}) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		response.Param(c, "参数格式错误")
		return false
	}
	return true
}
