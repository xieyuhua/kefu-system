package realtime

import (
	"sync"
	"time"

	"kefu-server/internal/common/model"
)

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

// ---------------- 离线宽限期 ----------------
// offlineGrace 是「最后一条连接断开」到「标记离线并释放接待会话」之间的宽限期。
//
// 为什么需要：浏览器刷新、切换标签页、手机切后台都会先断开 WebSocket 再重连（通常几秒内）。
// 若在断连瞬间就标记离线并释放会话，客服一刷新，正在接待的会话就被踢回「待接待」，
// 重连后看到的是「未接待」，接待归属丢失。
//
// 因此改为延迟释放：宽限期内重连（Hub.Add）会取消定时器；只有超过 grace 仍未重连，
// 才标记离线并触发释放回调。
const offlineGrace = 90 * time.Second

// offlineTimers 记录每个客服待执行的离线处理（userID -> *time.Timer）
var offlineTimers sync.Map

// scheduleAgentOffline 安排一次延迟离线处理（重复调用会重置计时）。
func scheduleAgentOffline(tenantID, userID uint) {
	if t, ok := offlineTimers.Load(userID); ok {
		if timer, ok := t.(*time.Timer); ok {
			timer.Stop()
		}
	}
	timer := time.AfterFunc(offlineGrace, func() {
		offlineTimers.Delete(userID)
		// 期间已重连则跳过
		if HubInstance().IsUserOnline(userID) {
			return
		}
		_ = updateUserOnlineByID(userID, false)
		if agentOfflineHook != nil {
			agentOfflineHook(tenantID, userID)
		}
	})
	offlineTimers.Store(userID, timer)
}

// CancelAgentOffline 取消待执行的离线处理。
//
// 两个调用点：客服重连（Hub.Add）与客服主动退出登录（账号服务的 LogoutUser）。
// 后者必须在「立即释放接待会话」之前调用，否则宽限定时器到点仍会再释放一次，
// 把退出登录后又被重新领取的会话错误踢回待接待。
func CancelAgentOffline(userID uint) {
	if t, ok := offlineTimers.LoadAndDelete(userID); ok {
		if timer, ok := t.(*time.Timer); ok {
			timer.Stop()
		}
	}
}
