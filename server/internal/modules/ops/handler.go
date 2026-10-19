package ops

// 本文件是 ops 模块的 HTTP 层
// （原 handler/misc.go 的设置 / 统计 / 设备自检部分 + handler/customer.go 的设备、离线同步、
// 演示入口部分）。handler 层不持有数据库连接：参数绑定 → 调 service（或 platform/setting）→ 写响应。

import (
	"kefu-server/internal/modules/customer"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/setting"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 系统配置（默认图片等） ----------------
// 实现完全落在 platform/setting（account 的登录响应也要用），本模块只做 HTTP 适配。

// ListSettingsHandler 读取全部配置项
func ListSettingsHandler(c *gin.Context) {
	list, err := setting.ListSettings(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// SaveSettingHandler 保存单个配置项
func SaveSettingHandler(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := setting.SaveSetting(authctx.TenantID(c), req.Key, req.Value); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 数据统计 ----------------

func StatsOverviewHandler(c *gin.Context) {
	days := httpx.QueryInt(c, "days", 7)
	data, err := StatsOverviewData(authctx.TenantID(c), days)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, data)
}

// ---------------- 设备（APP / PC 桌面端推送） ----------------

func RegisterDeviceHandler(c *gin.Context) {
	var req dto.DeviceRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := RegisterDevice(authctx.TenantID(c), authctx.UserID(c), req)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func ListDevicesHandler(c *gin.Context) {
	list, err := ListDevices(authctx.TenantID(c), authctx.UserID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func DeleteDeviceHandler(c *gin.Context) {
	if err := DeleteDevice(authctx.TenantID(c), authctx.UserID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// TestDevicePushHandler 手动触发一次推送，用于验证厂商通道是否接对。
// 只会推给「当前登录账号」已注册的设备，便于 APP 端自查。
func TestDevicePushHandler(c *gin.Context) {
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
	realtime.Notify(authctx.TenantID(c), authctx.UserID(c), "test", body.Title, body.Body, 0)
	response.OK(c, map[string]interface{}{"sent": true, "title": body.Title})
}

// ---------------- 离线消息 / 离线提醒 ----------------

func OfflineMessagesHandler(c *gin.Context) {
	list, latest, hasMore, err := OfflineMessages(authctx.TenantID(c), authctx.UserID(c), httpx.QueryUint(c, "since_id"), httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
		"has_more":  hasMore,
	})
}

func OfflineNotificationsHandler(c *gin.Context) {
	list, latest, err := OfflineNotifications(authctx.TenantID(c), authctx.UserID(c), httpx.QueryUint(c, "since_id"))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
	})
}

func ReadNotificationsHandler(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := MarkNotificationsRead(authctx.TenantID(c), authctx.UserID(c), req.IDs); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 演示入口 ----------------

func DemoIncomingHandler(c *gin.Context) {
	var req dto.DemoIncomingRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	cvo, _, err := DemoIncoming(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, cvo)
}

func DemoProductsHandler(c *gin.Context) {
	paged, err := customer.ListProducts(authctx.TenantID(c), c.Query("keyword"), 1, 50)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged.List)
}
