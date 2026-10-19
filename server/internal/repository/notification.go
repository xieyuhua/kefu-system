package repository

import "kefu-server/internal/model"

// ---------------- 站内提醒（Notification） ----------------
// 查询与已读标记见 customer.go（ListNotificationsAfter / MarkNotificationsRead），
// 此处只保留「写入」这一最小职责，供 service.Notify 使用。

// CreateNotification 写入一条站内提醒（客服离线时重连后可拉取）。
func CreateNotification(n *model.Notification) error {
	return DB().Create(n).Error
}
