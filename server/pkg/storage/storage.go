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

// 分页上限常量。handler 会把 query 里的 page / page_size 直接透传进来，
// 必须在这里夹取：否则 ?page_size=100000000 会生成巨大的 LIMIT，形成 DoS 入口。
const (
	// DefaultPageSize 未指定（或非法）页大小时的默认每页条数。
	DefaultPageSize = 20
	// MaxPageSize 单页条数上限：现有前端最大用 200，保持兼容。
	MaxPageSize = 200
	// MaxPage 页码上限：避免 page 极大时 offset 过大拖垮数据库。
	MaxPage = 10000
)

// ClampPage 把页码/页大小夹取到合法区间，返回实际生效的值。
// 用于「响应里回显 page / page_size」的场景：回显原始 query 值会让调用方
// 误以为 100000000 生效了，必须回显夹取后的值。
func ClampPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if page > MaxPage {
		page = MaxPage
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

// Paginate 按页码/页大小计算 offset，供各仓储复用。
//
// 传入值一律先夹取到合法区间：page ∈ [1, MaxPage]，pageSize ∈ [1, MaxPageSize]；
// page <= 0 视为第 1 页，pageSize <= 0 视为 DefaultPageSize。
func Paginate(page, pageSize int) (offset, limit int) {
	page, pageSize = ClampPage(page, pageSize)
	return (page - 1) * pageSize, pageSize
}
