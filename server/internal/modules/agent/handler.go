package agent

// 本文件是客服端（modules/agent）的 HTTP 层：会话与消息、客服端 WebSocket、
// 客户资料与标签、离线同步与设备、演示面板。
// handler 层不持有数据库连接：参数绑定 → 调本端 service 或 core/* 的 service → 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 route 在 Setup 里创建一次。
//
// 依赖方向：modules/agent → core/* → platform → pkg。
// 本包不 import modules/client、modules/admin，也不被 core/* import。
//
// 会话/消息、客户资料、渠道准入、设备与站内提醒这些可复用能力全在 core/*，
// 端内只保留「客服端专属的编排」（见 service.go：离线消息补拉、演示入口）；
// 设备 / 离线提醒的端点直接调 core/notify 的 service，不在本端 service 里转调。

import (
	"kefu-server/internal/core/notify"
)

// Handler 客服端的 HTTP 层，持有本端 service（service 内已建好各内核依赖）
// 与 core/notify 的 service（设备注册 / 离线提醒不再经本端中转）。
type Handler struct {
	svc    *Service
	notify *notify.Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler {
	return &Handler{svc: NewService(), notify: notify.NewService()}
}
