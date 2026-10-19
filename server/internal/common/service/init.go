package service

// 推送设备回调的启动期注册（原 notify 内核的 init）。
//
// platform/realtime 的厂商推送下发前需要「该客服已注册的设备列表」，
// 而 platform 不能反向依赖业务层，因此由本包在 init 里把读取函数注入进去。
// 注入的是方法值，真正取数发生在调用时（此时 storage 连接已由 main 注入），不会拿到 nil 连接。

import "kefu-server/pkg/realtime"

func init() {
	realtime.SetDeviceLister(NewNotifyService().ListDevicesForPush)
}
