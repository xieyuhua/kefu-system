package middleware

import (
	"strings"

	"kefu-server/pkg/auth"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// JWTAuth 校验 Authorization: Bearer <token>
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			// 兼容 ?token= 形式（WebSocket / APP 便捷调用）
			header = c.Query("token")
			if header != "" {
				header = "Bearer " + header
			}
		}
		if header == "" {
			response.Default().Unauth(c, "未登录")
			c.Abort()
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		token = strings.TrimSpace(token)
		claims, err := auth.Default().ParseToken(token)
		if err != nil {
			response.Default().Unauth(c, "登录已失效，请重新登录")
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

// CORS 允许前后端分离开发跨域
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS,PATCH")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Requested-With")
		c.Header("Access-Control-Expose-Headers", "Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
