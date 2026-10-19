package handler

// Package handler 后台端（admin）的 HTTP 层：渠道与渠道类型、子账号、客户与标签、
// 自动回复、设置与统计、设备推送自检、对话记录。
//
// handler 层不持有数据库连接：参数绑定 → 调本端 service（其内已持有公共业务层依赖）→ 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 internal/admin/route 在注册时创建一次。
//
// 依赖方向：admin/handler → admin/service → common/service → common/repository → common/model
// → platform → pkg。本包不 import agent / client 两端的任何包。
//
// 说明：渠道、子账号、客户标签、自动回复这些能力都在公共业务层，
// 后台端只负责「管理员视角的入参与鉴权分支」；概览统计的 SQL 与口径在公共统计内核，
// 本端 service 只做委派（见 admin/service）。

import (
	"kefu-server/internal/admin/service"
)

// Handler 后台端的 HTTP 层，持有本端 service（service 内已建好公共业务层依赖）。
type Handler struct {
	svc *service.Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler { return &Handler{svc: service.NewService()} }
