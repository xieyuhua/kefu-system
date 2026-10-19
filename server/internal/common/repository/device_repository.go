package repository

// 本文件是 common/repository 的数据访问层。
//
// 写表范围：devices（推送 token）/ notifications（站内提醒）。
// 这两张表原本落在 agent 的 repository 里，但「设备注册 + 离线提醒补拉」
// 是各端都要复用的能力（后台端的推送自检、将来桌面端/APP 端的独立入口），
// 端与端之间不允许互相 import，因此下沉为共享内核。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。
import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// Repository 通知内核的数据访问层（无状态，可随时构造）。
type DeviceRepository struct{}

// NewRepository 创建数据访问层实例。
func NewDeviceRepository() *DeviceRepository { return &DeviceRepository{} }

// ---------------- 推送设备（Device：APP / PC 桌面端） ----------------

// CreateDevice 登记一台推送设备。
func (r *DeviceRepository) CreateDevice(d *model.Device) error {
	return storage.DB().Create(d).Error
}

// FindDeviceByToken 按 (tenant_id, user_id, push_token) 查一台已登记的设备。
//
// 未命中返回 gorm.ErrRecordNotFound（与其它 Find* 一致）。
// 排序取 id 最小的那条：线上若已存在历史重复行，也总是复用最早的一行，不让重复继续放大。
func (r *DeviceRepository) FindDeviceByToken(tenantID, userID uint, token string) (*model.Device, error) {
	d := &model.Device{}
	if err := storage.DB().
		Where("tenant_id = ? AND user_id = ? AND push_token = ?", tenantID, userID, token).
		Order("id asc").First(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

// FindDeviceByKind 按 (tenant_id, user_id, device, platform) 查一台已登记的设备。
// 用于 push_token 为空的端（PC / Web）：同一台机器同一平台只保留一行。
// 未命中同样返回 gorm.ErrRecordNotFound。
func (r *DeviceRepository) FindDeviceByKind(tenantID, userID uint, device, platform string) (*model.Device, error) {
	d := &model.Device{}
	if err := storage.DB().
		Where("tenant_id = ? AND user_id = ? AND device = ? AND platform = ?", tenantID, userID, device, platform).
		Order("id asc").First(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

// SaveDevice 按主键整体更新一台设备（应用层 upsert 的更新分支）。
func (r *DeviceRepository) SaveDevice(d *model.Device) error {
	return storage.DB().Save(d).Error
}

// ListDevicesOfUser 列出某用户已登记的推送设备（按 id 倒序）。
func (r *DeviceRepository) ListDevicesOfUser(tenantID, userID uint) ([]*model.Device, error) {
	var list []*model.Device
	if err := storage.DB().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteDeviceOfUser 删除某用户自己的一台设备。
func (r *DeviceRepository) DeleteDeviceOfUser(tenantID, userID, id uint) error {
	return storage.DB().Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		Delete(&model.Device{}).Error
}
