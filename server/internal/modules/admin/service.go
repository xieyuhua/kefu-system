package admin

// 本文件是后台端（modules/admin）的端内编排：为各后台能力建好内核依赖，并把统计委派给共享内核。
// 其余后台能力（渠道、子账号、客户与标签、自动回复、设置）直接调 core/* 的 service，
// 概览统计的 SQL 与口径已下沉到 core/stats，本端只保留委派，不复制任何统计逻辑。
//
// 依赖方向：modules/admin → core/stats → platform → pkg。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨内核依赖在构造时建好。

import (
	"kefu-server/internal/core/account"
	"kefu-server/internal/core/automation"
	"kefu-server/internal/core/channel"
	"kefu-server/internal/core/conversation"
	"kefu-server/internal/core/customer"
	"kefu-server/internal/core/stats"
)

// Service 后台端的端内编排层，持有各内核依赖（含共享统计内核）。
type Service struct {
	conv  *conversation.Service
	cust  *customer.Service
	ch    *channel.Service
	acc   *account.Service
	auto  *automation.Service
	stats *stats.Service
}

// NewService 创建端内业务层实例（自带各内核依赖）。
func NewService() *Service {
	return &Service{
		conv:  conversation.NewService(),
		cust:  customer.NewService(),
		ch:    channel.NewService(),
		acc:   account.NewService(),
		auto:  automation.NewService(),
		stats: stats.NewService(),
	}
}

// ---------------- 数据统计 ----------------

// StatsOverviewData 生成指定天数的数据概览：口径在 core/stats，本端只做委派。
func (s *Service) StatsOverviewData(tenantID uint, days int) (*stats.StatsOverview, error) {
	return s.stats.StatsOverviewData(tenantID, days)
}
