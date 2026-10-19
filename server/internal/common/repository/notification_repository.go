package repository

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// NotificationRepository 站内提醒（notifications）的数据访问层。
type NotificationRepository struct{}

// NewNotificationRepository 创建数据访问层实例。
func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{}
}

// ---------------- 站内提醒（Notification） ----------------

// CreateNotification 写入一条站内提醒（客服离线时重连后可拉取）。
// 平台层的 realtime.Notify 也有一份等价实现（基础设施例外），两者写同一张表。
func (r *NotificationRepository) CreateNotification(n *model.Notification) error {
	return storage.DB().Create(n).Error
}

// DefaultNotificationLimit 增量拉取提醒的默认条数上限，与唯一调用方 OfflineNotifications 保持一致。
const DefaultNotificationLimit = 200

// ListNotificationsAfter 取用户在 sinceID 之后新增的系统提醒（最多 limit 条，升序）。
func (r *NotificationRepository) ListNotificationsAfter(tenantID, userID, sinceID uint, limit int) ([]*model.Notification, error) {
	// gorm 对 limit=0 会生成 LIMIT 0（永远查不到数据）、对负数等于不写 LIMIT（全表拉取），
	// 两者都不是调用方想要的结果，这里统一按默认上限兜底。
	if limit <= 0 {
		limit = DefaultNotificationLimit
	}
	var list []*model.Notification
	if err := storage.DB().Where("tenant_id = ? AND user_id = ? AND id > ?", tenantID, userID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SetNotificationsRead 把用户的提醒标记为已读；ids 为空时表示全部已读。
func (r *NotificationRepository) SetNotificationsRead(tenantID, userID uint, ids []uint) error {
	q := storage.DB().Model(&model.Notification{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("read", true).Error
}
