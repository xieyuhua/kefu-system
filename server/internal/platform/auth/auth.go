package auth

import (
	"errors"
	"time"

	"kefu-server/internal/platform/cache"
	"kefu-server/internal/platform/config"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims JWT 载荷。JTI 用于配合 Redis 黑名单实现"踢下线/退出登录"。
type Claims struct {
	UserID   uint   `json:"uid"`
	TenantID uint   `json:"tid"`
	Role     string `json:"role"`
	Account  string `json:"acc"`
	jwt.RegisteredClaims
}

func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}

// GenerateToken 签发 token，并在 Redis（若可用）中缓存一份以便快速校验与失效控制。
func GenerateToken(userID, tenantID uint, role, account string) (string, error) {
	expire := time.Duration(config.C.JWT.Expire) * time.Hour
	if config.C.JWT.Expire <= 0 {
		expire = 24 * time.Hour
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
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.C.JWT.Secret))
	if err != nil {
		return "", err
	}
	// 缓存 token → claims，避免每次请求都做签名运算；同时作为"可失效"依据
	cache.Get().Set(cache.TokenKey(token), cachedClaims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Account:  account,
		Expire:   time.Now().Add(expire).Unix(),
	}, expire)
	return token, nil
}

type cachedClaims struct {
	UserID   uint   `json:"uid"`
	TenantID uint   `json:"tid"`
	Role     string `json:"role"`
	Account  string `json:"acc"`
	Expire   int64  `json:"exp"`
}

// ParseToken 解析 token。优先查缓存；Redis 未启用时直接验签。
func ParseToken(token string) (*Claims, error) {
	// 1) 黑名单：退出登录后立即失效（分布式生效）
	var blacklisted bool
	cache.Get().Get(cache.BlacklistKey(token), &blacklisted)
	if blacklisted {
		return nil, errors.New("token 已失效")
	}

	// 2) 缓存命中
	var cc cachedClaims
	if cache.Get().Get(cache.TokenKey(token), &cc) && cc.UserID > 0 {
		if cc.Expire > 0 && time.Now().Unix() > cc.Expire {
			return nil, errors.New("token 已过期")
		}
		return &Claims{UserID: cc.UserID, TenantID: cc.TenantID, Role: cc.Role, Account: cc.Account}, nil
	}

	// 3) 直接验签
	claims := &Claims{}
	t, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(config.C.JWT.Secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("token 无效")
	}
	return claims, nil
}

// RevokeToken 加入黑名单并删除缓存（退出登录 / 强制下线）
func RevokeToken(token string) {
	cache.Get().Del(cache.TokenKey(token))
	cache.Get().Set(cache.BlacklistKey(token), true, 7*24*time.Hour)
}
