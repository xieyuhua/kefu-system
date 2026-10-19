// Package notify 通知内核：推送设备（devices）与站内提醒（notifications）。
// 模块内按 service / repository 分层，HTTP 层在各端模块
// （客服端 modules/agent 的设备 / 离线提醒端点、后台端 modules/admin 的推送自检）。
//
// 依赖方向：core/notify → platform → pkg（本内核不依赖其它内核，也不 import 任何 modules）。
// 提醒的「下发」不在这里：统一出口仍是 platform/realtime 的 Notify（WebSocket + 厂商推送）。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），依赖在构造时建好；
// 本文件没有需要保留的包级纯函数。
package notify

import (
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/util"
)

// Service 通知内核的业务编排层，持有本模块仓储。
type Service struct {
	repo *Repository
}

// NewService 创建业务层实例（自带数据访问层）。
func NewService() *Service {
	return &Service{repo: NewRepository()}
}

// ---------------- 推送设备（APP / PC 桌面端） ----------------

// RegisterDevice 登记一台推送设备，返回组装好的视图。
func (s *Service) RegisterDevice(tenantID, userID uint, req dto.DeviceRequest) (*dto.DeviceVO, error) {
	if req.Device == "" {
		req.Device = "app"
	}
	d := &model.Device{
		TenantID:  tenantID,
		UserID:    userID,
		Device:    req.Device,
		Platform:  req.Platform,
		PushToken: req.PushToken,
		Name:      req.Name,
	}
	if err := s.repo.CreateDevice(d); err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
	}, nil
}

// ListDevices 列出某用户已登记的推送设备（按 id 倒序）。
func (s *Service) ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	list, err := s.repo.ListDevicesOfUser(tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DeviceVO, 0, len(list))
	for _, d := range list {
		out = append(out, dto.DeviceVO{
			ID: d.ID, Device: d.Device, Platform: d.Platform,
			Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
		})
	}
	return out, nil
}

// DeleteDevice 删除某用户自己的一台设备。
func (s *Service) DeleteDevice(tenantID, userID, id uint) error {
	return s.repo.DeleteDeviceOfUser(tenantID, userID, id)
}

// ---------------- 离线提醒 ----------------

// OfflineNotifications 供 APP / PC 桌面端断线重连后增量补拉提醒。sinceID 为端侧已收到的最大提醒 ID。
func (s *Service) OfflineNotifications(tenantID, userID uint, sinceID uint) ([]dto.NotificationVO, uint, error) {
	list, err := s.repo.ListNotificationsAfter(tenantID, userID, sinceID, 200)
	if err != nil {
		return nil, sinceID, err
	}
	out := make([]dto.NotificationVO, 0, len(list))
	latest := sinceID
	for _, n := range list {
		out = append(out, dto.NotificationVO{
			ID: n.ID, Type: n.Type, Title: n.Title, Body: n.Body,
			ConversationID: n.ConversationID, Read: n.Read, CreatedAt: util.FmtTime(n.CreatedAt),
		})
		if n.ID > latest {
			latest = n.ID
		}
	}
	return out, latest, nil
}

// MarkNotificationsRead 把用户的提醒标记为已读；ids 为空时表示全部已读。
func (s *Service) MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	return s.repo.SetNotificationsRead(tenantID, userID, ids)
}

// ---------------- 供其它内核复用的门面 ----------------

// ListDevicesForPush 返回某客服已登记的推送设备（原 model 实体，供厂商推送下发使用）。
// platform/realtime 通过 SetDeviceLister 回调注入使用，避免平台层再写一份相同查询。
func (s *Service) ListDevicesForPush(tenantID, userID uint) ([]*model.Device, error) {
	return s.repo.ListDevicesOfUser(tenantID, userID)
}
