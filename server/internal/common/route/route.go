package route

// Package route 跨端公共路由：登录 / 登出 / 当前用户 / 改资料 / 改密码、
// 附件上传（登录态与游客态）、订单与商品。
//
// 这些端点的处理器只依赖公共业务层（账号 / 客户 / 上传），三端共用同一套路由与参数，
// 因此不归属任何一端。各端的路由见 internal/{admin,agent,client}/route。

import (
	"kefu-server/internal/common/handler"
	"kefu-server/internal/common/middleware"

	"github.com/gin-gonic/gin"
)

// Register 注册三端共用的 /api 路由。
//   - g：/api 分组（无鉴权）；
//   - authed：挂在 /api 下、已套 middleware.JWTAuth() 的子分组。
func Register(g *gin.RouterGroup, authed *gin.RouterGroup) {
	accH := handler.NewAuthHandler()      // 账号：登录 / 个人资料（跨端端点）
	custH := handler.NewCustomerHandler() // 客户：订单 / 商品（跨端端点）
	upH := handler.NewUploadHandler()     // 附件上传（跨端端点）

	// 认证
	g.POST("/auth/login", accH.LoginHandler)
	g.POST("/auth/logout", middleware.JWTAuth(), accH.LogoutHandler)
	g.GET("/auth/me", middleware.JWTAuth(), accH.MeHandler)
	g.PUT("/auth/profile", middleware.JWTAuth(), accH.UpdateProfileHandler)
	g.PUT("/auth/password", middleware.JWTAuth(), accH.ChangePasswordHandler)

	// 用户端上传附件（图片/视频/文件）
	g.POST("/guest/upload", upH.UploadHandler)

	// 上传附件（图片 / 视频 / 文件）
	authed.POST("/upload", upH.UploadHandler)

	// 订单 / 商品
	authed.GET("/orders", custH.ListOrdersHandler)
	authed.GET("/products", custH.ListProductsHandler)
}
