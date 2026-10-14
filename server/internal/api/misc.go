package api

import (
	"fmt"
	"net/http"
	"net/url"

	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// ---------------- 自动回复规则 ----------------

func ListAutoReplies(c *gin.Context) {
	list, err := service.ListAutoReplies(tenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateAutoReply(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req service.AutoReplyRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.CreateAutoReply(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAutoReply(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req service.AutoReplyRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := service.UpdateAutoReply(tenantID(c), uintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAutoReply(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := service.DeleteAutoReply(tenantID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 渠道二维码 / 接入地址 ----------------

// ChannelQR 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func ChannelQRPNG(c *gin.Context) {
	vo, err := service.ChannelQR(tenantID(c), uintParam(c, "id"),
		c.Query("base"), queryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(vo.FileName)))
	c.Data(http.StatusOK, "image/png", vo.PNG)
}

// ChannelQRInfo 返回 base64 + 地址，便于页面内联展示与复制
func ChannelQRInfo(c *gin.Context) {
	vo, err := service.ChannelQR(tenantID(c), uintParam(c, "id"),
		c.Query("base"), queryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func ListChannelAgents(c *gin.Context) {
	list, err := service.ListChannelAgents(tenantID(c), uintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

func SetChannelAgents(c *gin.Context) {
	if !isAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.UserIDs == nil {
		req.UserIDs = []uint{}
	}
	list, err := service.SetChannelAgents(tenantID(c), uintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func ListChannelTypes(c *gin.Context) {
	list, err := service.ListChannelTypes(tenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateChannelType(c *gin.Context) {
	var req service.ChannelTypeRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.CreateChannelType(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateChannelType(c *gin.Context) {
	var req service.ChannelTypeRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := service.UpdateChannelType(tenantID(c), uintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteChannelType(c *gin.Context) {
	if err := service.DeleteChannelType(tenantID(c), uintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 设备推送自检 ----------------

// TestDevicePush 手动触发一次推送，用于验证厂商通道是否接对。
// 只会推给「当前登录账号」已注册的设备，便于 APP 端自查。
func TestDevicePush(c *gin.Context) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Title == "" {
		body.Title = "推送自检"
	}
	if body.Body == "" {
		body.Body = "这是一条测试推送，收到说明厂商通道已接通"
	}
	service.Notify(tenantID(c), userID(c), "test", body.Title, body.Body, 0)
	response.OK(c, map[string]interface{}{"sent": true, "title": body.Title})
}

// ---------------- 数据统计 ----------------

func StatsOverview(c *gin.Context) {
	days := queryInt(c, "days", 7)
	data, err := service.StatsOverviewData(tenantID(c), days)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, data)
}
