package handler

// Package handler 客服端（agent，工作台 / APP / 桌面端）的 HTTP 层：会话与消息、
// 客服端 WebSocket、客户资料与标签、离线同步与设备、演示面板。
//
// handler 层不持有数据库连接：参数绑定 → 调本端 service 或公共业务层 → 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 internal/agent/route 在注册时创建一次。
//
// 依赖方向：agent/handler → agent/service → common/service → common/repository → common/model
// → platform → pkg。本包不 import admin / client 两端的任何包。
//
// 会话/消息、客户资料、渠道准入、设备与站内提醒这些可复用能力全在公共业务层，
// 端内只保留「客服端专属的编排」（见 agent/service：离线消息补拉、演示入口）；
// 设备 / 离线提醒的端点直接用公共通知内核，不在本端 service 里转调。

import (
	"kefu-server/internal/agent/service"
	commonsvc "kefu-server/internal/common/service"
)

// Handler 客服端的 HTTP 层，持有本端 service（service 内已建好公共业务层依赖）
// 与公共通知内核（设备注册 / 离线提醒不再经本端中转）。
type Handler struct {
	svc    *service.Service
	notify *commonsvc.NotifyService
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler {
	return &Handler{svc: service.NewService(), notify: commonsvc.NewNotifyService()}
}
