package service

import (
	"kefu-server/internal/auth"
	"kefu-server/internal/db"
	"kefu-server/internal/model"

	"gorm.io/gorm"
)

// gormDB 返回全局数据库连接。包内统一用此函数避免与 db 包名冲突。
func gormDB() *gorm.DB { return db.DB }

func hashPwd(p string) (string, error) { return auth.HashPassword(p) }

// Notify 写一条系统提醒（离线时 APP/PC 端重连后可拉取），并通过推送网关尝试下发。
func Notify(tenantID, userID uint, typ, title, body string, convID uint) {
	n := model.Notification{
		TenantID:       tenantID,
		UserID:         userID,
		Type:           typ,
		Title:          title,
		Body:           body,
		ConversationID: convID,
	}
	gormDB().Create(&n)

	// 实时推送（Web/PC/APP 在线时立即收到；离线则由端侧重连后拉取离线列表）
	HubInstance().Publish(tenantID, []uint{userID}, Event{
		Event: "notification.new",
		Data: map[string]interface{}{
			"id":              n.ID,
			"type":            n.Type,
			"title":           n.Title,
			"body":            n.Body,
			"conversation_id": n.ConversationID,
			"created_at":      fmtTime(n.CreatedAt),
		},
	})

	// APP 厂商推送（预留）：此处按设备 token 调用推送网关，
	// 实际接入 APNs/极光/个推时在 pushGateway 里实现即可。
	pushGateway.Send(tenantID, userID, n)
}
