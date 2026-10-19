// Package auth 鉴权 SDK：JWT 签发 / 校验 / 吊销，以及 bcrypt 密码哈希。
//
// 本包是可复用能力包：不依赖应用内部包，运行期需要的两项外部能力
// 都由调用方注入，避免把应用配置与缓存实现固化进 SDK：
//
//   - NewAuth(secret, expire) / (a *Auth) Configure(secret, expire)：JWT 密钥与有效期，
//     由 main 读取配置后注入一次；
//   - (a *Auth) SetRevoker(fn)：token 吊销实现（如基于 pkg/cache 的黑名单），由 main 启动时注入。
//     未注入时 RevokeToken 为空操作，其余能力不受影响。
//
// 未注入密钥（或密钥为空）时按「未配置」处理：GenerateToken 返回错误，
// ParseToken 一律判定 token 无效，避免用空密钥签发出任何人都能伪造的 token。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewAuth 与单例访问器 Default，
// 业务能力一律是 *Auth 的指针接收者方法；密钥、有效期、吊销实现都是实例字段。
package auth

import (
	"errors"
	"strings"
	"sync"
	"time"

	"kefu-server/pkg/cache"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 载荷。JTI 用于配合 Redis 黑名单实现"踢下线/退出登录"。
type Claims struct {
	UserID   uint   `json:"uid"`
	TenantID uint   `json:"tid"`
	Role     string `json:"role"`
	Account  string `json:"acc"`
	jwt.RegisteredClaims
}

// cachedClaims 缓存里的 token 载荷（避免每次请求都做签名运算）。
type cachedClaims struct {
	UserID   uint   `json:"uid"`
	TenantID uint   `json:"tid"`
	Role     string `json:"role"`
	Account  string `json:"acc"`
	Expire   int64  `json:"exp"`
}

// defaultExpire 未显式配置有效期时的兜底（与原实现一致：24 小时）。
const defaultExpire = 24 * time.Hour

// Auth 鉴权实例。密钥 / 有效期 / 吊销实现都是实例字段，不再有包级可变状态。
type Auth struct {
	mu     sync.RWMutex
	secret string
	expire time.Duration

	revokerMu sync.RWMutex
	revoker   func(token string)
}

// NewAuth 构造鉴权实例并注入 JWT 密钥与有效期。
// expire <= 0 时按 24 小时兜底；secret 为空时视为「未配置」。
func NewAuth(secret string, expire time.Duration) *Auth {
	secret = strings.TrimSpace(secret)
	if expire <= 0 {
		expire = defaultExpire
	}
	return &Auth{secret: secret, expire: expire}
}

// defaultAuth 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
// 初始为「未配置」状态，由 main（或测试）通过 Configure 注入真实密钥。
var defaultAuth = NewAuth("", 0)

// Default 返回包级默认实例。
func Default() *Auth { return defaultAuth }

// Configure 注入 JWT 密钥与有效期，由 main 在读取配置后调用一次。
// expire <= 0 时按 24 小时兜底；secret 为空时视为「未配置」。
func (a *Auth) Configure(secret string, expire time.Duration) {
	secret = strings.TrimSpace(secret)
	if expire <= 0 {
		expire = defaultExpire
	}
	a.mu.Lock()
	a.secret = secret
	a.expire = expire
	a.mu.Unlock()
}

// snapshot 读取当前 JWT 配置；secret 为空表示未配置。
func (a *Auth) snapshot() (string, time.Duration) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.secret, a.expire
}

// SetRevoker 注入 token 吊销实现（退出登录 / 强制下线）。
// 未注入时 RevokeToken 为空操作 —— 吊销依赖缓存，属于调用方的实现细节。
func (a *Auth) SetRevoker(fn func(token string)) {
	a.revokerMu.Lock()
	a.revoker = fn
	a.revokerMu.Unlock()
}

// GenerateToken 签发 token，并在缓存（若可用）中记录一份以便快速校验与失效控制。
func (a *Auth) GenerateToken(userID, tenantID uint, role, account string) (string, error) {
	secret, expire := a.snapshot()
	if secret == "" {
		return "", errors.New("鉴权未初始化：缺少 JWT 密钥")
	}
	claims := Claims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Account:  account,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   account,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	// 缓存 token → claims，避免每次请求都做签名运算；同时作为"可失效"依据
	c := cache.Default()
	c.Get().Set(c.TokenKey(token), cachedClaims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Account:  account,
		Expire:   time.Now().Add(expire).Unix(),
	}, expire)
	return token, nil
}

// ParseToken 解析 token。优先查缓存；缓存未命中时直接验签。
func (a *Auth) ParseToken(token string) (*Claims, error) {
	c := cache.Default()

	// 1) 黑名单：退出登录后立即失效（分布式生效）
	var blacklisted bool
	c.Get().Get(c.BlacklistKey(token), &blacklisted)
	if blacklisted {
		return nil, errors.New("token 已失效")
	}

	// 2) 缓存命中
	var cc cachedClaims
	if c.Get().Get(c.TokenKey(token), &cc) && cc.UserID > 0 {
		if cc.Expire > 0 && time.Now().Unix() > cc.Expire {
			return nil, errors.New("token 已过期")
		}
		return &Claims{UserID: cc.UserID, TenantID: cc.TenantID, Role: cc.Role, Account: cc.Account}, nil
	}

	// 3) 直接验签（未配置密钥时不可能验签成功）
	secret, _ := a.snapshot()
	if secret == "" {
		return nil, errors.New("token 无效")
	}
	claims := &Claims{}
	t, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("token 无效")
	}
	return claims, nil
}

// RevokeToken 吊销 token（退出登录 / 强制下线）。
// 具体实现由 SetRevoker 注入；未注入时为 no-op。
func (a *Auth) RevokeToken(token string) {
	a.revokerMu.RLock()
	fn := a.revoker
	a.revokerMu.RUnlock()
	if fn == nil {
		return
	}
	fn(token)
}
