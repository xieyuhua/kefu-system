package agent

// 本文件是客服端（modules/agent）的数据访问层：devices（推送 token）与 notifications（离线提醒）
// 两张表只服务客服端的「重连补拉 + 推送 token」，没有第二个端使用，因此归属本端；
// 跨表只读口径（可见会话范围）也在这里，与 core/conversation 的可见范围判定保持一致。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// Repository 客服端自有表的数据访问层（无状态，可随时构造）。
type Repository struct{}

// NewRepository 创建数据访问层实例。
func NewRepository() *Repository { return &Repository{} }

// ---------------- 站内提醒（Notification） ----------------

// CreateNotification 写入一条站内提醒（客服离线时重连后可拉取）。
// 平台层的 realtime.Notify 也有一份等价实现（基础设施例外），两者写同一张表。
func (r *Repository) CreateNotification(n *model.Notification) error {
	return storage.DB().Create(n).Error
}

// ---------------- 客服设备（Device：APP / PC 桌面端） ----------------

// CreateDevice 登记一台推送设备。
func (r *Repository) CreateDevice(d *model.Device) error {
	return storage.DB().Create(d).Error
}

// ListDevicesOfUser 列出某客服已登记的推送设备（按 id 倒序）。
func (r *Repository) ListDevicesOfUser(tenantID, userID uint) ([]*model.Device, error) {
	var list []*model.Device
	if err := storage.DB().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteDeviceOfUser 删除某客服自己的一台设备。
func (r *Repository) DeleteDeviceOfUser(tenantID, userID, id uint) error {
	return storage.DB().Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		Delete(&model.Device{}).Error
}

// ---------------- 离线同步（消息 / 提醒） ----------------

// ListVisibleConversationIDs 列出客服可见范围内的会话 ID：
// 自己接待的 + 自己渠道下尚未分配的（与原离线同步口径一致）。
func (r *Repository) ListVisibleConversationIDs(tenantID, userID uint) ([]uint, error) {
	var chIDs []uint
	if err := storage.DB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).
		Pluck("channel_id", &chIDs).Error; err != nil {
		return nil, err
	}
	var convIDs []uint
	q := storage.DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if len(chIDs) > 0 {
		q = q.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
	} else {
		q = q.Where("agent_id = ?", userID)
	}
	if err := q.Pluck("id", &convIDs).Error; err != nil {
		return nil, err
	}
	return convIDs, nil
}

// ListMessagesOfConversations 取一批会话中 id > afterID 的消息，按 id 升序，最多 limit 条。
// 调用方按「多取一条」判断是否还有更多（has_more）。
func (r *Repository) ListMessagesOfConversations(tenantID uint, convIDs []uint, afterID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.DB().Where("tenant_id = ? AND conversation_id IN ? AND id > ?", tenantID, convIDs, afterID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListNotificationsAfter 取客服在 sinceID 之后新增的系统提醒（最多 limit 条，升序）。
func (r *Repository) ListNotificationsAfter(tenantID, userID, sinceID uint, limit int) ([]*model.Notification, error) {
	var list []*model.Notification
	if err := storage.DB().Where("tenant_id = ? AND user_id = ? AND id > ?", tenantID, userID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SetNotificationsRead 把客服的提醒标记为已读；ids 为空时表示全部已读。
func (r *Repository) SetNotificationsRead(tenantID, userID uint, ids []uint) error {
	q := storage.DB().Model(&model.Notification{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("read", true).Error
}
