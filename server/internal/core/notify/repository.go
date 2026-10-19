package notify

// 本文件是 core/notify 的数据访问层。
//
// 写表范围：devices（推送 token）/ notifications（站内提醒）。
// 这两张表原本落在 modules/agent 的 repository 里，但「设备注册 + 离线提醒补拉」
// 是各端都要复用的能力（后台端的推送自检、将来桌面端/APP 端的独立入口），
// 端与端之间不允许互相 import，因此下沉为共享内核。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// Repository 通知内核的数据访问层（无状态，可随时构造）。
type Repository struct{}

// NewRepository 创建数据访问层实例。
func NewRepository() *Repository { return &Repository{} }

// ---------------- 站内提醒（Notification） ----------------

// CreateNotification 写入一条站内提醒（客服离线时重连后可拉取）。
// 平台层的 realtime.Notify 也有一份等价实现（基础设施例外），两者写同一张表。
func (r *Repository) CreateNotification(n *model.Notification) error {
	return storage.DB().Create(n).Error
}

// ListNotificationsAfter 取用户在 sinceID 之后新增的系统提醒（最多 limit 条，升序）。
func (r *Repository) ListNotificationsAfter(tenantID, userID, sinceID uint, limit int) ([]*model.Notification, error) {
	var list []*model.Notification
	if err := storage.DB().Where("tenant_id = ? AND user_id = ? AND id > ?", tenantID, userID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SetNotificationsRead 把用户的提醒标记为已读；ids 为空时表示全部已读。
func (r *Repository) SetNotificationsRead(tenantID, userID uint, ids []uint) error {
	q := storage.DB().Model(&model.Notification{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("read", true).Error
}

// ---------------- 推送设备（Device：APP / PC 桌面端） ----------------

// CreateDevice 登记一台推送设备。
func (r *Repository) CreateDevice(d *model.Device) error {
	return storage.DB().Create(d).Error
}

// ListDevicesOfUser 列出某用户已登记的推送设备（按 id 倒序）。
func (r *Repository) ListDevicesOfUser(tenantID, userID uint) ([]*model.Device, error) {
	var list []*model.Device
	if err := storage.DB().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteDeviceOfUser 删除某用户自己的一台设备。
func (r *Repository) DeleteDeviceOfUser(tenantID, userID, id uint) error {
	return storage.DB().Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		Delete(&model.Device{}).Error
}
