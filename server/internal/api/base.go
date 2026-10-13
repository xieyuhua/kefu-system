package api

import (
	"strconv"

	"kefu-server/internal/auth"
	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var db *gorm.DB

// gormDB 语义化别名，业务代码里统一用它，避免与 db 包名混淆
func gormDB() *gorm.DB { return db }

func SetDB(g *gorm.DB) { db = g }

// ClaimsFromContext 从 gin 上下文取登录信息（由 middleware 注入）
type ctxClaims struct {
	UserID   uint
	TenantID uint
	Role     string
	Account  string
}

func claims(c *gin.Context) *ctxClaims {
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

func currentUser(c *gin.Context) *model.User {
	cl := claims(c)
	if cl == nil {
		return nil
	}
	var u model.User
	if err := db.First(&u, cl.UserID).Error; err != nil {
		return nil
	}
	return &u
}

func isAdmin(c *gin.Context) bool {
	cl := claims(c)
	return cl != nil && cl.Role == "admin"
}

func userID(c *gin.Context) uint {
	if cl := claims(c); cl != nil {
		return cl.UserID
	}
	return 0
}

func tenantID(c *gin.Context) uint {
	if cl := claims(c); cl != nil {
		return cl.TenantID
	}
	return 0
}

func uintParam(c *gin.Context, key string) uint {
	v, _ := strconv.ParseUint(c.Param(key), 10, 64)
	return uint(v)
}

func queryInt(c *gin.Context, key string, def int) int {
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

func queryUint(c *gin.Context, key string) uint {
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

func bindJSON(c *gin.Context, out interface{}) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		response.Param(c, "参数格式错误")
		return false
	}
	return true
}

// ---------------- Channel ----------------

func ListChannels(c *gin.Context) {
	list, err := service.ListChannels(tenantID(c), userID(c), isAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateChannel(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.CreateChannel(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateChannel(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := service.UpdateChannel(tenantID(c), uintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteChannel(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := service.DeleteChannel(tenantID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- Agent ----------------

// ListAgents 客服列表。
//   - admin：看全部
//   - agent：带 ?scope=transfer 时可看「与自己接待渠道有交集」的在线客服（用于会话转接）
func ListAgents(c *gin.Context) {
	if !isAdmin(c) && c.Query("scope") != "transfer" {
		response.Forbid(c, "仅管理员可查看")
		return
	}
	list, err := service.ListAgents(tenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if !isAdmin(c) {
		// 非管理员只返回在线、且未停用的同事
		filtered := make([]dto.AgentVO, 0, len(list))
		for _, a := range list {
			if a.Online && a.Enabled && a.ID != userID(c) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}
	response.OK(c, list)
}

func CreateAgent(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.CreateAgent(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAgent(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := service.UpdateAgent(tenantID(c), uintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAgent(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := service.DeleteAgent(tenantID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func SetOnline(c *gin.Context) {
	var body struct {
		Online bool `json:"online"`
	}
	_ = c.ShouldBindJSON(&body)
	target := uintParam(c, "id")
	cl := claims(c)
	if cl == nil {
		return
	}
	if target != cl.UserID && cl.Role != "admin" {
		response.Forbid(c, "只能修改自己的在线状态")
		return
	}
	if err := service.SetOnline(tenantID(c), target, body.Online); err != nil {
		response.Server(c, err.Error())
		return
	}
	status := "offline"
	if body.Online {
		status = "online"
	}
	response.OK(c, map[string]interface{}{"online": body.Online, "status": status})
}
