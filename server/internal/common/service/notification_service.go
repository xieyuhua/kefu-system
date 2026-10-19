package service

import (
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/util"
)

// ---------------- 离线提醒 ----------------

// OfflineNotifications 供 APP / PC 桌面端断线重连后增量补拉提醒。sinceID 为端侧已收到的最大提醒 ID。
func (s *NotifyService) OfflineNotifications(tenantID, userID uint, sinceID uint) ([]dto.NotificationVO, uint, error) {
	list, err := s.notifications.ListNotificationsAfter(tenantID, userID, sinceID, 200)
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
func (s *NotifyService) MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	return s.notifications.SetNotificationsRead(tenantID, userID, ids)
}
