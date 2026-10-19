// Package storage 提供全局数据库连接与通用查询辅助（事务 / 分页）。
//
// 分层约定（阶段二：按业务模块组织）：
//
//	route              —— 路由汇总，调用各模块导出的处理器
//	modules/<模块>      —— 模块内再技术分层：handler.go / service.go / repository.go
//	pkg/<通用层>        —— response / storage / util / cache / httpx（不依赖 internal）
//	platform/<基础设施> —— model / dto / middleware / auth / authctx / config /
//	                      db / seed / realtime / ipinfo / setting / card / idgen
//
// 各模块的 repository 统一通过本包取连接：
//
//	storage.DB()     —— 全局 *gorm.DB（只读 / 写入都从这里出发）
//	storage.Tx(fn)   —— 事务，出错自动回滚
//	storage.Paginate —— 页码 → offset / limit
//
// 连接由 main 在启动时通过 SetDB 注入一次。
package storage

import "gorm.io/gorm"

// db 全局数据库连接。由 main 在启动时通过 SetDB 注入一次。
var db *gorm.DB

// SetDB 注入数据库连接（仅在应用启动时调用）。
func SetDB(g *gorm.DB) { db = g }

// DB 返回全局连接，供各模块 repository 使用。
// 上层（service / handler）不应直接取值，以保持数据访问集中在各模块的 repository 里。
func DB() *gorm.DB { return db }

// Tx 在事务中执行 fn，出错自动回滚。
// 用于跨多表写入且要求原子性的场景（如「发消息 + 更新会话摘要」）。
func Tx(fn func(tx *gorm.DB) error) error {
	return db.Transaction(fn)
}

// Paginate 按页码/页大小计算 offset，供各仓储复用。
func Paginate(page, pageSize int) (offset, limit int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return (page - 1) * pageSize, pageSize
}
