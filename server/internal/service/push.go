package service

import (
	"log"

	"kefu-server/internal/db"
	"kefu-server/internal/model"
)

// pushGateway 厂商推送网关（预留实现）。
// 当前是"记录型"占位实现：把待推送内容打到日志，并标记设备。
// 接入真实推送时，只需把 Send 里的实现替换为：
//   - APNs:  github.com/sideshow/apns2
//   - 极光/个推/华为/小米：各自官方 SDK
//
// 推送目标 token 从 model.Device 读取（APP 启动后调用 POST /api/devices 注册）。
type PushGateway struct{}

var pushGateway = &PushGateway{}

func (p *PushGateway) Send(tenantID, userID uint, n model.Notification) {
	var devices []model.Device
	if err := db.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&devices).Error; err != nil {
		return
	}
	if len(devices) == 0 {
		// 没有注册设备 → 说明用户完全离线，提醒已落库，等待重连后由
		// GET /api/offline/notifications 拉取。
		return
	}
	for _, d := range devices {
		if d.PushToken == "" {
			continue
		}
		// TODO(接入厂商推送): 调用 APNs / 极光 / 个推 的 HTTP API 下发
		log.Printf("[push] -> device=%s platform=%s token=%s title=%s body=%s",
			d.Device, d.Platform, mask(d.PushToken), n.Title, n.Body)
	}
}

func mask(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}
