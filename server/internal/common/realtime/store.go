package realtime

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// ---------------- 基础设施层的数据落库例外 ----------------
// platform 不能反向依赖业务模块，但实时通道的几处副作用必须落库，因此在这里
// 直接使用 platform/storage 的连接做最小读写（与原先调用 repository.* 的 SQL 等价）：
//   - 提醒写入（Notify）
//   - 账号在线状态翻转（WS 连接建立 / 断开）
// 这些表的「业务写入」仍由对应模块的 repository 负责，本文件只处理实时通道自身需要的那几条。
//
// 设备列表读取不在此列：它归 common/service，通过 SetDeviceLister 回调注入。

// createNotification 写入一条站内提醒（客服离线时重连后可拉取）。
func createNotification(n *model.Notification) error {
	return storage.Default().DB().Create(n).Error
}

// updateUserOnlineByID 只按主键更新在线状态（WS 层只持有 userID 的场景）。
func updateUserOnlineByID(id uint, online bool) error {
	return storage.Default().DB().Model(&model.User{}).Where("id = ?", id).Update("online", online).Error
}
