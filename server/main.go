package main

import (
	"log"
	"os"
	"time"

	"kefu-server/internal/common/config"
	"kefu-server/internal/common/db"
	"kefu-server/internal/common/realtime"
	"kefu-server/internal/common/seed"
	"kefu-server/internal/common/service"
	"kefu-server/internal/route"
	"kefu-server/pkg/auth"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/storage"
)

// revokeTokenTTL 被吊销 token 的黑名单保留时长（与改造前一致：7 天）
const revokeTokenTTL = 7 * 24 * time.Hour

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
	storage.Default().SetDB(db.DB)
	log.Printf("数据库就绪: driver=%s", config.C.Database.Driver)

	// Redis（可选，未启用时自动降级为内存实现，单机无需依赖）
	// 配置项由 main 转成 cache.Options：pkg/cache 不认识 config 包。
	cacheStore := cache.Default()
	if err := cacheStore.Init(cache.Options{
		Enabled:  config.C.Redis.Enabled,
		Addr:     config.C.Redis.Addr,
		Password: config.C.Redis.Password,
		DB:       config.C.Redis.DB,
		Prefix:   config.C.Redis.Prefix,
	}); err != nil {
		log.Printf("缓存初始化警告: %v", err)
	}
	_ = os.MkdirAll(config.C.Upload.Dir, 0755)

	// 鉴权 SDK 的装配：pkg/auth 不认识 config 包，JWT 密钥与有效期由 main 注入一次。
	// token 吊销依赖缓存，因此放在缓存初始化之后注入（此时缓存驱动已就绪）。
	authSDK := auth.Default()
	authSDK.Configure(config.C.JWT.Secret, time.Duration(config.C.JWT.Expire)*time.Hour)
	authSDK.SetRevoker(func(token string) {
		cacheStore.Get().Del(cacheStore.TokenKey(token))
		cacheStore.Get().Set(cacheStore.BlacklistKey(token), true, revokeTokenTTL)
	})

	// 厂商推送网关（未在 config.yaml 启用厂商时只记录日志）
	realtime.InitPush()

	// 演示数据（首次启动自动写入）
	if err := seed.Run(); err != nil {
		log.Printf("初始化演示数据失败: %v", err)
	}
	// 历史渠道补齐随机标识 app_id（幂等）
	seed.BackfillChannelAppID()
	// 补齐演示静态资源（幂等，防止 data/demo 被清理后图片全部 404）
	// 重启后没有任何 WebSocket 连接，把所有客服重置为离线
	seed.EnsureDemoAssets()
	if err := service.NewAccountService().ResetAllOnline(); err != nil {
		log.Printf("重置在线状态失败: %v", err)
	}

	r := route.Setup()
	addr := ":" + config.C.Server.Port
	log.Printf("客服系统后端启动: http://127.0.0.1%s  (mode=%s, redis=%v)", addr, config.C.Server.Mode, cacheStore.Get().Enabled())
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
