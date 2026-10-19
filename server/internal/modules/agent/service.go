package agent

// 本文件是客服端（modules/agent）的端内编排：离线消息补拉、演示面板。
// 会话与消息的读写、客户与会话的定位、自动回复判定都在 core/*，这里只做「客服端专属」的组合。
//
// 依赖方向：modules/agent → core/* → platform → pkg。
//
// 数据访问说明：设备与站内提醒已下沉到 core/notify（后台端推送自检也要用），
// 本端不再持有 repository.go；会话 / 消息的只读查询统一走 core/conversation 的 Service 门面，
// 因此本端没有自己的数据访问层。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨内核依赖在构造时建好；
// 只有无状态的纯工具（randomNickname / randomPhone）保留为包级函数。

import (
	"kefu-server/internal/core/automation"
	"kefu-server/internal/core/channel"
	"kefu-server/internal/core/conversation"
	"kefu-server/internal/core/customer"
)

// Service 客服端的端内编排层，持有四个跨内核依赖（设备 / 提醒由 handler 直接走 core/notify）。
type Service struct {
	conv *conversation.Service
	cust *customer.Service
	ch   *channel.Service
	auto *automation.Service
}

// NewService 创建端内业务层实例（自带各内核依赖）。
func NewService() *Service {
	return &Service{
		conv: conversation.NewService(),
		cust: customer.NewService(),
		ch:   channel.NewService(),
		auto: automation.NewService(),
	}
}
