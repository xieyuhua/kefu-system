package main

import (
	"log"
	"os"

	"kefu-server/internal/common/service"
	"kefu-server/internal/platform/config"
	"kefu-server/internal/platform/db"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/seed"
	"kefu-server/internal/route"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/storage"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	if err := config.Load(cfgPath); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if err := db.Init(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	// 数据访问层接管数据库连接（各模块 repository 统一通过 pkg/storage 取连接）
	storage.SetDB(db.DB)
	log.Printf("数据库就绪: driver=%s", config.C.Database.Driver)

	// Redis（可选，未启用时自动降级为内存实现，单机无需依赖）
	// 配置项由 main 转成 cache.Options：pkg/cache 不认识 config 包。
	if err := cache.Init(cache.Options{
		Enabled:  config.C.Redis.Enabled,
		Addr:     config.C.Redis.Addr,
		Password: config.C.Redis.Password,
		DB:       config.C.Redis.DB,
		Prefix:   config.C.Redis.Prefix,
	}); err != nil {
		log.Printf("缓存初始化警告: %v", err)
	}
	_ = os.MkdirAll(config.C.Upload.Dir, 0755)

	// 厂商推送网关（未在 config.yaml 启用厂商时只记录日志）
	realtime.InitPush()

	// 演示数据（首次启动自动写入）
	if err := seed.Run(); err != nil {
		log.Printf("初始化演示数据失败: %v", err)
	}
	// 历史渠道补齐随机标识 app_id（幂等）
	seed.BackfillChannelAppID()
	// 补齐演示静态资源（幂等，防止 data/demo 被清理后图片全部 404）
	seed.EnsureDemoAssets()
	// 重启后没有任何 WebSocket 连接，把所有客服重置为离线
	if err := service.NewAccountService().ResetAllOnline(); err != nil {
		log.Printf("重置在线状态失败: %v", err)
	}

	r := route.Setup()
	addr := ":" + config.C.Server.Port
	log.Printf("客服系统后端启动: http://127.0.0.1%s  (mode=%s, redis=%v)", addr, config.C.Server.Mode, cache.Get().Enabled())
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
