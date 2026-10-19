package service

// ---------------- 通知域（Notify） ----------------
// 推送设备（devices）与站内提醒（notifications）。
// 提醒的「下发」不在这里：统一出口仍是 platform/realtime 的 Notify（WebSocket + 厂商推送）。
//
// 形态约定：业务编排都是 NotifyService 的方法（指针接收者），构造时建好设备与提醒两个仓储；
// 本域没有需要保留的包级纯函数。
import (
	"kefu-server/internal/common/dto"
	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/util"
)

// NotifyService 通知内核的业务编排层，持有本模块仓储。
// 推送设备与站内提醒分属两个仓储（合并成一个包后按子域拆开）。
type NotifyService struct {
	devices       *repository.DeviceRepository
	notifications *repository.NotificationRepository
}

// NewNotifyService 创建业务层实例（自带数据访问层）。
func NewNotifyService() *NotifyService {
	return &NotifyService{
		devices:       repository.NewDeviceRepository(),
		notifications: repository.NewNotificationRepository(),
	}
}

// ---------------- 推送设备（APP / PC 桌面端） ----------------

// RegisterDevice 登记一台推送设备，返回组装好的视图。
//
// 幂等注册（应用层的 upsert，不走唯一索引）：
//   - push_token 非空：按 (tenant_id, user_id, push_token) 定位旧行，命中则更新
//     device / platform / name 并复用原 ID。APP 每次启动都会带同一个 token 调本接口，
//     以前每次都 Create，结果同一 token 重复推送、失效 token 无限堆积；
//   - push_token 为空（PC / Web 端常见）：按 (tenant_id, user_id, device, platform) 定位，
//     命中则只更新 name，push_token 保持原值（新建时即为空），避免同一台机器每次启动新增一行。
//
// 取舍：这里刻意**不加唯一索引**，而是把去重放在应用层。
//  1. push_token 为空时多台设备的值相等，唯一索引会把正常的第二次注册挡下来；
//  2. 线上库可能已经积累了重复行，AutoMigrate 建唯一索引会直接失败，服务起不来。
//
// 因此 devices 上只保留普通索引，重复行由本方法「先查后写」收敛。
func (s *NotifyService) RegisterDevice(tenantID, userID uint, req dto.DeviceRequest) (*dto.DeviceVO, error) {
	if req.Device == "" {
		req.Device = "app"
	}
	// 先按去重维度找已登记的设备；找不到（含读库失败）就新建，不阻断注册
	d := s.findRegisteredDevice(tenantID, userID, req)
	isNew := d == nil
	if isNew {
		d = &model.Device{TenantID: tenantID, UserID: userID}
	}
	// 可变字段统一覆盖；token 为空时不覆盖原 token，避免把已登记的有效 token 清掉
	d.Device = req.Device
	d.Platform = req.Platform
	d.Name = req.Name
	if req.PushToken != "" {
		d.PushToken = req.PushToken
	}
	if isNew {
		if err := s.devices.CreateDevice(d); err != nil {
			return nil, err
		}
	} else if err := s.devices.SaveDevice(d); err != nil {
		return nil, err
	}
	return &dto.DeviceVO{
		ID: d.ID, Device: d.Device, Platform: d.Platform,
		Name: d.Name, PushToken: d.PushToken, CreatedAt: util.FmtTime(d.CreatedAt),
	}, nil
}

// findRegisteredDevice 按去重维度查已登记的设备：push_token 非空按 token 查，
// 为空则退化为「设备 + 平台」维度。查不到（含读库失败）返回 nil，由调用方新建。
func (s *NotifyService) findRegisteredDevice(tenantID, userID uint, req dto.DeviceRequest) *model.Device {
	if req.PushToken != "" {
		if d, err := s.devices.FindDeviceByToken(tenantID, userID, req.PushToken); err == nil {
			return d
		}
		return nil
	}
	if d, err := s.devices.FindDeviceByKind(tenantID, userID, req.Device, req.Platform); err == nil {
		return d
	}
	return nil
}

// ListDevices 列出某用户已登记的推送设备（按 id 倒序）。
func (s *NotifyService) ListDevices(tenantID, userID uint) ([]dto.DeviceVO, error) {
	list, err := s.devices.ListDevicesOfUser(tenantID, userID)
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
func (s *NotifyService) DeleteDevice(tenantID, userID, id uint) error {
	return s.devices.DeleteDeviceOfUser(tenantID, userID, id)
}

// ---------------- 供其它内核复用的门面 ----------------

// ListDevicesForPush 返回某客服已登记的推送设备（原 model 实体，供厂商推送下发使用）。
// platform/realtime 通过 SetDeviceLister 回调注入使用，避免平台层再写一份相同查询。
func (s *NotifyService) ListDevicesForPush(tenantID, userID uint) ([]*model.Device, error) {
	return s.devices.ListDevicesOfUser(tenantID, userID)
}
