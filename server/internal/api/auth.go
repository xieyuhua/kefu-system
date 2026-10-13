package api

import (
	"kefu-server/internal/auth"
	"kefu-server/internal/cache"
	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"

	"github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Account == "" {
		response.Param(c, "请输入账号和密码")
		return
	}
	var u model.User
	if err := db.Where("account = ?", req.Account).First(&u).Error; err != nil {
		response.Unauth(c, "账号或密码错误")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		response.Unauth(c, "账号或密码错误")
		return
	}
	if !u.Enabled {
		response.Forbid(c, "账号已被停用，请联系管理员")
		return
	}
	var tenant model.Tenant
	db.First(&tenant, u.TenantID)

	// 该账号可见渠道：admin 看租户全部，agent 看分配到的
	var channelIDs []uint
	if u.Role == "admin" {
		var chs []model.Channel
		db.Where("tenant_id = ?", u.TenantID).Find(&chs)
		for _, ch := range chs {
			channelIDs = append(channelIDs, ch.ID)
		}
	} else {
		var acs []model.AgentChannel
		db.Where("user_id = ?", u.ID).Find(&acs)
		for _, ac := range acs {
			channelIDs = append(channelIDs, ac.ChannelID)
		}
	}
	if channelIDs == nil {
		channelIDs = []uint{}
	}

	token, err := auth.GenerateToken(u.ID, u.TenantID, u.Role, u.Account)
	if err != nil {
		response.Server(c, "签发 token 失败")
		return
	}
	db.Model(&model.User{}).Where("id = ?", u.ID).Update("online", true)

	response.OK(c, dto.LoginResponse{
		Token: token,
		User: dto.CurrentUser{
			ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
			Account: u.Account, Avatar: u.Avatar, Online: true,
			MaxConcurrent: u.MaxConcurrent, Channels: channelIDs,
			Tenant: dto.TenantBrief{ID: tenant.ID, Name: tenant.Name, Logo: tenant.Logo},
		},
	})
}

func Me(c *gin.Context) {
	u := currentUser(c)
	if u == nil {
		return
	}
	var tenant model.Tenant
	db.First(&tenant, u.TenantID)

	var channelIDs []uint
	if u.Role == "admin" {
		var chs []model.Channel
		db.Where("tenant_id = ?", u.TenantID).Find(&chs)
		for _, ch := range chs {
			channelIDs = append(channelIDs, ch.ID)
		}
	} else {
		var acs []model.AgentChannel
		db.Where("user_id = ?", u.ID).Find(&acs)
		for _, ac := range acs {
			channelIDs = append(channelIDs, ac.ChannelID)
		}
	}
	if channelIDs == nil {
		channelIDs = []uint{}
	}
	response.OK(c, dto.CurrentUser{
		ID: u.ID, TenantID: u.TenantID, Role: u.Role, Name: u.Name,
		Account: u.Account, Avatar: u.Avatar, Online: u.Online,
		MaxConcurrent: u.MaxConcurrent, Channels: channelIDs,
		Tenant: dto.TenantBrief{ID: tenant.ID, Name: tenant.Name, Logo: tenant.Logo},
	})
}

func Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if len(token) > 7 {
		token = token[7:]
	}
	if token != "" {
		auth.RevokeToken(token)
	}
	if u := currentUser(c); u != nil {
		db.Model(&model.User{}).Where("id = ?", u.ID).Update("online", false)
		cache.Get().Del(cache.OnlineKey(u.TenantID, u.ID))
	}
	response.OK(c, true)
}
