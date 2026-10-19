package handler

import (
	"kefu-server/internal/auth"
	"kefu-server/internal/cache"
	"kefu-server/internal/dto"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// Login 登录：鉴权与用户信息组装都在 service 层，这里只负责参数绑定与响应映射。
func Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Param(c, "请输入账号和密码")
		return
	}
	res, aerr := service.Login(req)
	if aerr != nil {
		switch aerr.Status {
		case "param":
			response.Param(c, aerr.Msg)
		case "unauth":
			response.Unauth(c, aerr.Msg)
		case "forbid":
			response.Forbid(c, aerr.Msg)
		default:
			response.Server(c, aerr.Msg)
		}
		return
	}
	response.OK(c, res)
}

// Me 当前登录用户信息（前端刷新后恢复登录态用）。
func Me(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		return
	}
	cur, err := service.BuildCurrentUser(u)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, cur)
}

// Logout 退出登录：吊销 token 并置离线。
func Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if len(token) > 7 {
		token = token[7:]
	}
	if token != "" {
		auth.RevokeToken(token)
	}
	if u := currentUser(c); u != nil {
		_ = service.SetOnline(u.TenantID, u.ID, false)
		cache.Get().Del(cache.OnlineKey(u.TenantID, u.ID))
	}
	response.OK(c, true)
}
