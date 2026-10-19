package realtime

import "kefu-server/internal/common/model"

// ---------------- 回调解耦 ----------------
// 基础设施层不反向依赖业务包：客服最后一个连接断开后要「释放其接待中的会话」，
// 这属于会话业务逻辑，由 common/service 在 init 里通过 SetAgentOfflineHook 注册进来。
//
// 同理，厂商推送下发前需要「该客服已注册的设备列表」，设备数据属公共业务层，
// 由 common/service 的通知域在 init 里注册读取函数，避免 platform 与业务层各写一份相同 SQL。
var agentOfflineHook func(tenantID, userID uint)

// SetAgentOfflineHook 注册「客服离线」回调（由 conversation 包 init 调用）
func SetAgentOfflineHook(fn func(tenantID, userID uint)) { agentOfflineHook = fn }

// deviceLister 由业务侧提供的设备列表读取函数
var deviceLister func(tenantID, userID uint) ([]*model.Device, error)

// SetDeviceLister 注册「按客服读取推送设备」的函数（由 notify 包 init 调用）
func SetDeviceLister(fn func(tenantID, userID uint) ([]*model.Device, error)) { deviceLister = fn }
