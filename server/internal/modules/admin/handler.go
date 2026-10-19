package admin

// 本文件是后台端（modules/admin）的 HTTP 层：渠道与渠道类型、子账号、客户与标签、
// 自动回复、设置与统计、设备自检、对话记录。
// handler 层不持有数据库连接：参数绑定 → 调本端 service（其内已持有各内核 service）→ 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 route 在 Setup 里创建一次。
//
// 依赖方向：modules/admin → core/* → platform → pkg。
// 本包不 import modules/client、modules/agent，也不被 core/* import。
//
// 说明：渠道、子账号、客户标签、自动回复这些能力的 service 全在 core/*，
// 后台端只负责「管理员视角的入参与鉴权分支」；概览统计的 SQL 与口径已下沉到 core/stats，
// 本端 service 只做委派（见 service.go）。

// Handler 后台端的 HTTP 层，持有本端 service（service 内已建好各内核依赖）。
type Handler struct {
	svc *Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler { return &Handler{svc: NewService()} }
