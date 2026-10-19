package service

// Package service 客服端（agent）的端内编排：离线消息补拉、演示面板。
// 会话与消息的读写、客户与会话的定位、自动回复判定都在公共业务层，这里只做「客服端专属」的组合。
//
// 依赖方向：agent/handler → agent/service → common/service → common/repository → common/model → platform → pkg。
//
// 数据访问说明：设备与站内提醒已下沉到公共业务层的通知内核（后台端推送自检也要用），
// 本端没有自己的数据访问层。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨层依赖在构造时建好；
// 只有无状态的纯工具（randomNickname / randomPhone）保留为包级函数。

import (
	commonsvc "kefu-server/internal/common/service"
)

// Service 客服端的端内编排层，持有四个公共业务依赖（设备 / 提醒由 handler 直接用通知内核）。
type Service struct {
	Conv      *commonsvc.ConversationService
	Cust      *commonsvc.CustomerService
	Channels  *commonsvc.ChannelService
	AutoReply *commonsvc.AutomationService
}

// NewService 创建端内业务层实例（自带各公共业务依赖）。
func NewService() *Service {
	return &Service{
		Conv:      commonsvc.NewConversationService(),
		Cust:      commonsvc.NewCustomerService(),
		Channels:  commonsvc.NewChannelService(),
		AutoReply: commonsvc.NewAutomationService(),
	}
}
