// Package repository 是数据访问层：所有 GORM 查询都收敛在这里。
//
// 分层约定：
//
//	route      —— 路由注册
//	handler    —— HTTP 层：解析参数 → 调 service → 写响应，不直接访问数据库
//	service    —— 业务编排：校验、事务边界、领域规则，通过 repository 读写数据
//	repository —— 数据访问：GORM 查询与写入（唯一允许出现 GORM 语句的层）
//	model      —— 数据实体（GORM 模型）
//	dto        —— 出入参结构
package repository

import "gorm.io/gorm"

// db 全局数据库连接。由 main 在启动时通过 SetDB 注入一次。
var db *gorm.DB

// SetDB 注入数据库连接（仅在应用启动时调用）。
func SetDB(g *gorm.DB) { db = g }

// DB 返回全局连接，仅供 repository 包内及其同域仓储函数使用。
// 上层（service/handler）不应直接调用，以保持数据访问集中在本包。
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
