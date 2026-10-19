package httpx

import (
	"strconv"

	"kefu-server/internal/platform/auth"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// 说明：本包只依赖 platform 内的基础设施包，不反向依赖 handler/service/repository。
// 需要业务侧的账号加载能力时，由 service 包在 init 里通过 SetUserLoader 注入。

// ctxClaims 从 gin 上下文取登录信息（由 middleware 注入）
type ctxClaims struct {
	UserID   uint
	TenantID uint
	Role     string
	Account  string
}

// ClaimsFromContext 从 gin 上下文取登录信息（由 middleware 注入）
func ClaimsFromContext(c *gin.Context) *ctxClaims {
	v, ok := c.Get("claims")
	if !ok {
		return nil
	}
	cl, ok := v.(*auth.Claims)
	if !ok || cl == nil {
		return nil
	}
	return &ctxClaims{UserID: cl.UserID, TenantID: cl.TenantID, Role: cl.Role, Account: cl.Account}
}

// userLoader 按「租户 + 账号 ID」载入账号，由 service 包启动时注入。
var userLoader func(tenantID, userID uint) (*model.User, error)

// SetUserLoader 注册账号加载实现（由 service 包 init 调用），避免 httpx 依赖业务包。
func SetUserLoader(fn func(tenantID, userID uint) (*model.User, error)) { userLoader = fn }

// CurrentUser 取当前登录账号的完整信息；未登录或账号不存在时返回 nil
func CurrentUser(c *gin.Context) *model.User {
	cl := ClaimsFromContext(c)
	if cl == nil || userLoader == nil {
		return nil
	}
	u, err := userLoader(cl.TenantID, cl.UserID)
	if err != nil || u == nil {
		return nil
	}
	return u
}

// IsAdmin 当前登录账号是否管理员
func IsAdmin(c *gin.Context) bool {
	cl := ClaimsFromContext(c)
	return cl != nil && cl.Role == "admin"
}

// UserID 当前登录账号 ID；未登录返回 0
func UserID(c *gin.Context) uint {
	if cl := ClaimsFromContext(c); cl != nil {
		return cl.UserID
	}
	return 0
}

// TenantID 当前登录账号所属租户 ID；未登录返回 0
func TenantID(c *gin.Context) uint {
	if cl := ClaimsFromContext(c); cl != nil {
		return cl.TenantID
	}
	return 0
}

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
