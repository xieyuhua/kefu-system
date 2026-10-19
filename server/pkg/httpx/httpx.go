// Package httpx 提供与业务无关的 gin 辅助：路径/查询参数解析与 JSON 绑定。
//
// 本包属于通用层（pkg），不得依赖 internal 下的任何包；
// 依赖登录态的鉴权辅助见 internal/common/authctx。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewHelper 与单例访问器 Default，
// 业务能力一律是 *Helper 的指针接收者方法；调用点写 httpx.Default().BindJSON(c, &req)。
package httpx

import (
	"strconv"

	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// Helper gin 辅助集。无状态，所有能力都是它的方法。
type Helper struct{}

// NewHelper 创建辅助集实例。
func NewHelper() *Helper { return &Helper{} }

// defaultHelper 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultHelper = NewHelper()

// Default 返回包级默认实例。
func Default() *Helper { return defaultHelper }

// UintParam 解析路径参数为 uint；解析失败返回 0
func (h *Helper) UintParam(c *gin.Context, key string) uint {
	v, _ := strconv.ParseUint(c.Param(key), 10, 64)
	return uint(v)
}

// QueryInt 解析整型查询参数；缺省或非法时返回 def
func (h *Helper) QueryInt(c *gin.Context, key string, def int) int {
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
func (h *Helper) QueryUint(c *gin.Context, key string) uint {
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
func (h *Helper) BindJSON(c *gin.Context, out interface{}) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		response.Default().Param(c, "参数格式错误")
		return false
	}
	return true
}
