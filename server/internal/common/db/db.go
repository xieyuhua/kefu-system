package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"kefu-server/internal/common/config"
	"kefu-server/internal/common/model"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Init 根据配置初始化数据库连接并执行自动迁移。
func Init() error {
	cfg := config.C.Database
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
		dialector = mysql.Open(dsn)
	default:
		if dir := filepath.Dir(cfg.DSN); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		dialector = sqlite.Open(cfg.DSN)
	}

	level := logger.Warn
	if config.C.Server.Mode == "debug" {
		level = logger.Info
	}
	gdb, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(level)})
	if err != nil {
		return err
	}

	// MySQL：库/表必须为 utf8mb4，否则 emoji 等 4 字节字符会报
	// Error 1366 Incorrect string value。历史库常见为 utf8(3 字节)，
	// 这里在 AutoMigrate 之前把库默认字符集升级，新建表即继承 utf8mb4。
	if cfg.Driver == "mysql" {
		if err := ensureMySQLUTF8MB4(gdb, cfg.Name); err != nil {
			log.Printf("[db] 字符集升级失败（emoji 可能仍无法写入）: %v", err)
		}
	}

	// 连接池
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)

	if err := gdb.AutoMigrate(
		&model.Tenant{},
		&model.User{},
		&model.Channel{},
		&model.ChannelType{},
		&model.Setting{},
		&model.AgentChannel{},
		&model.Customer{},
		&model.Tag{},
		&model.CustomerTag{},
		&model.Conversation{},
		&model.Message{},
		&model.Order{},
		&model.Product{},
		&model.Device{},
		&model.Notification{},
		&model.AutoReply{},
	); err != nil {
		return err
	}
	DB = gdb
	return nil
}

// ensureMySQLUTF8MB4 把数据库与既有表升级到 utf8mb4（emoji 等 4 字节字符必需）。
//
// 背景：老版本建库时 MySQL 默认字符集可能是 utf8（3 字节），此时写入 😀🎉 这类
// 4 字节字符会直接报 `Error 1366 (HY000): Incorrect string value`。
// 连接串里的 charset=utf8mb4 只影响「客户端→服务端」的编码，不会改变列本身的字符集，
// 因此必须显式 ALTER。
//
// 幂等：逐个对象检查当前字符集，已经是 utf8mb4 的跳过，避免每次启动都做全表 CONVERT。
func ensureMySQLUTF8MB4(gdb *gorm.DB, dbName string) error {
	var dbCharset string
	if err := gdb.Raw(
		"SELECT DEFAULT_CHARACTER_SET_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", dbName,
	).Scan(&dbCharset).Error; err != nil {
		return err
	}
	if !strings.EqualFold(dbCharset, "utf8mb4") {
		stmt := fmt.Sprintf("ALTER DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)
		if err := gdb.Exec(stmt).Error; err != nil {
			return err
		}
		log.Printf("[db] 数据库 %s 字符集 %s → utf8mb4", dbName, dbCharset)
	}

	// 既有表：按 TABLE_COLLATION 判断，非 utf8mb4 的逐张 CONVERT
	type tableInfo struct {
		Name      string `gorm:"column:name"`
		Collation string `gorm:"column:collation"`
	}
	var tables []tableInfo
	if err := gdb.Raw(`
		SELECT TABLE_NAME AS name, IFNULL(TABLE_COLLATION, '') AS collation
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'`, dbName).Scan(&tables).Error; err != nil {
		return err
	}
	converted := 0
	for _, t := range tables {
		if strings.HasPrefix(strings.ToLower(t.Collation), "utf8mb4") {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE `%s` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", t.Name)
		if err := gdb.Exec(stmt).Error; err != nil {
			return fmt.Errorf("表 %s 转换失败: %w", t.Name, err)
		}
		converted++
	}
	if converted > 0 {
		log.Printf("[db] 已把 %d 张表转换为 utf8mb4", converted)
	}
	return nil
}
