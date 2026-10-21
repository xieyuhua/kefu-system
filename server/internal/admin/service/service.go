package service

// Package service 后台端（admin）的端内编排：为各后台能力建好公共业务依赖，
// 并把概览统计委派给共享统计内核。
//
// 其余后台能力（渠道、子账号、客户与标签、自动回复、设置）由 admin/handler 直接调用
// 本结构体暴露的字段，端内不再复制一份「转调门面」。
//
// 依赖方向：admin/handler → admin/service → common/service → common/repository → common/model → platform → pkg。
//
// 形态约定：端内编排都是 Service 的方法（指针接收者），跨层依赖在构造时建好。

import (
	"context"

	commonsvc "kefu-server/internal/common/service"
)

// Service 后台端的端内编排层，持有各公共业务依赖（含共享统计内核）。
// 字段导出：handler 已拆到 admin/handler 包，需要按能力直接取用对应的公共 service。
type Service struct {
	Conv       *commonsvc.ConversationService
	Cust       *commonsvc.CustomerService
	Channels   *commonsvc.ChannelService
	Account    *commonsvc.AccountService
	AutoReply  *commonsvc.AutomationService
	QuickReply *commonsvc.QuickReplyService
	Stats      *commonsvc.StatsService
	Storage    *commonsvc.ObjectStorageService
	// AI AI 应答配置（密钥脱敏 / 回填 / 连通性测试）
	AI *commonsvc.AIService
	// Knowledge 知识库（AI / RAG 应答的资料来源）
	Knowledge *commonsvc.KnowledgeService
}

// NewService 创建端内业务层实例（自带各公共业务依赖）。
func NewService() *Service {
	return &Service{
		Conv:       commonsvc.NewConversationService(),
		Cust:       commonsvc.NewCustomerService(),
		Channels:   commonsvc.NewChannelService(),
		Account:    commonsvc.NewAccountService(),
		AutoReply:  commonsvc.NewAutomationService(),
		QuickReply: commonsvc.NewQuickReplyService(),
		Stats:      commonsvc.NewStatsService(),
		Storage:    commonsvc.NewObjectStorageService(),
		AI:         commonsvc.NewAIService(),
		Knowledge:  commonsvc.NewKnowledgeService(),
	}
}

// ---------------- 数据统计 ----------------

// StatsOverviewData 生成指定天数的数据概览：口径在统计内核，本端只做委派。
func (s *Service) StatsOverviewData(tenantID uint, days int) (*commonsvc.StatsOverview, error) {
	return s.Stats.StatsOverviewData(tenantID, days)
}

// ---------------- 对象存储配置（S3 兼容） ----------------

// ObjectStorageConfig 对象存储配置的入参类型（handler 绑定请求体用）。
// 字段与 settings 里 storage.object 的 JSON 一致；secret_key 允许传 ****** 表示沿用已存密钥。
type ObjectStorageConfig = commonsvc.ObjectStorage

// TestObjectStorage 用给定配置测试对象存储连通性：构造客户端并上传 / 删除一个探针对象。
func (s *Service) TestObjectStorage(ctx context.Context, tenantID uint, cfg *ObjectStorageConfig) error {
	return s.Storage.TestObjectStorage(ctx, tenantID, cfg)
}

// ---------------- AI 应答配置（OpenAI 兼容接口） ----------------

// AITestConfig 「测试连接」按钮的入参类型（handler 绑定请求体用）。
// api_key 允许传 ****** 表示沿用已保存的密钥。
type AITestConfig = commonsvc.AITestRequest

// TestAI 用给定（或库中）配置做一次极短实调，验证地址 / 鉴权 / 模型名是否可用。
func (s *Service) TestAI(ctx context.Context, tenantID uint, req *AITestConfig) error {
	return s.AI.TestAI(ctx, tenantID, req)
}
