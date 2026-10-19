package realtime

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 基础设施层的数据落库例外 ----------------
// platform 不能反向依赖业务模块，但实时通道的几处副作用必须落库，因此在这里
// 直接使用 platform/storage 的连接做最小读写（与原先调用 repository.* 的 SQL 等价）：
//   - 提醒写入（Notify）
//   - 账号在线状态翻转（WS 连接建立 / 断开）
//   - 设备列表读取（厂商推送下发前）
// 这些表的「业务写入」仍由对应模块的 repository 负责，本文件只处理实时通道自身需要的那几条。

// createNotification 写入一条站内提醒（客服离线时重连后可拉取）。
func createNotification(n *model.Notification) error {
	return storage.DB().Create(n).Error
}

// updateUserOnlineByID 只按主键更新在线状态（WS 层只持有 userID 的场景）。
func updateUserOnlineByID(id uint, online bool) error {
	return storage.DB().Model(&model.User{}).Where("id = ?", id).Update("online", online).Error
}

// listDevicesForPush 列出某客服已登记的推送设备（按 id 倒序）。
func listDevicesForPush(tenantID, userID uint) ([]*model.Device, error) {
	var list []*model.Device
	if err := storage.DB().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
