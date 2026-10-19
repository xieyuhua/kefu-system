package realtime

// ---------------- 离线回调解耦 ----------------
// 基础设施层不反向依赖业务包：客服最后一个连接断开后要「释放其接待中的会话」，
// 这属于会话业务逻辑，由 service 包在 init 里通过 SetAgentOfflineHook 注册进来。

// agentOfflineHook 客服（含管理员）完全离线时的回调
var agentOfflineHook func(tenantID, userID uint)

// SetAgentOfflineHook 注册「客服离线」回调（由 service 包 init 调用）
func SetAgentOfflineHook(fn func(tenantID, userID uint)) { agentOfflineHook = fn }
