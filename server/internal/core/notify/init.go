// Package notify 的启动期回调注册。
//
// platform/realtime 的厂商推送下发前需要「该客服已注册的设备列表」，
// 而 platform 不能反向依赖 core，因此由本包在 init 里把读取函数注入进去。
// 注入的是方法值，真正取数发生在调用时（此时 storage 连接已由 main 注入），不会拿到 nil 连接。
package notify

import "kefu-server/internal/platform/realtime"

func init() {
	realtime.SetDeviceLister(NewService().ListDevicesForPush)
}
