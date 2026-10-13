package api

import (
	"kefu-server/internal/dto"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// ---------------- 客户 / 历史档案 ----------------

func ListCustomers(c *gin.Context) {
	paged, err := service.ListCustomers(tenantID(c), c.Query("keyword"), queryUint(c, "channel_id"),
		queryInt(c, "page", 1), queryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func GetCustomer(c *gin.Context) {
	detail, err := service.GetCustomerDetail(tenantID(c), uintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, detail)
}

// ---------------- 订单 / 商品 ----------------

func ListOrders(c *gin.Context) {
	paged, err := service.ListOrders(tenantID(c), queryUint(c, "user_id"), c.Query("keyword"),
		queryInt(c, "page", 1), queryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func ListProducts(c *gin.Context) {
	paged, err := service.ListProducts(tenantID(c), c.Query("keyword"),
		queryInt(c, "page", 1), queryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

// ---------------- 设备（APP / PC 桌面端推送） ----------------

func RegisterDevice(c *gin.Context) {
	var req dto.DeviceRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.RegisterDevice(tenantID(c), userID(c), req)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func ListDevices(c *gin.Context) {
	list, err := service.ListDevices(tenantID(c), userID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func DeleteDevice(c *gin.Context) {
	if err := service.DeleteDevice(tenantID(c), userID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 离线消息 / 离线提醒 ----------------

func OfflineMessages(c *gin.Context) {
	list, latest, hasMore, err := service.OfflineMessages(tenantID(c), userID(c), queryUint(c, "since_id"), queryInt(c, "limit", 200))
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

func OfflineNotifications(c *gin.Context) {
	list, latest, err := service.OfflineNotifications(tenantID(c), userID(c), queryUint(c, "since_id"))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"list":      list,
		"latest_id": latest,
	})
}

func ReadNotifications(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := service.MarkNotificationsRead(tenantID(c), userID(c), req.IDs); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 演示入口 ----------------

func DemoIncoming(c *gin.Context) {
	var req dto.DemoIncomingRequest
	if !bindJSON(c, &req) {
		return
	}
	cvo, _, err := service.DemoIncoming(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, cvo)
}

func DemoProducts(c *gin.Context) {
	paged, err := service.ListProducts(tenantID(c), c.Query("keyword"), 1, 50)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged.List)
}
