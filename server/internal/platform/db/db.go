package db

import (
	"fmt"
	"os"
	"path/filepath"

	"kefu-server/internal/platform/config"
	"kefu-server/internal/platform/model"

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
