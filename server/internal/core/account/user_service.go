package account

import "kefu-server/internal/platform/model"

// ---------------- 供其它内核 / 端复用的只读门面 ----------------
// core/conversation 等内核与端模块需要「按主键取一条账号 / 统计某客服 open 会话数」这类单表读取，
// 跨包只能走 Service：这里的每个方法都只是把调用转给本包 repository，SQL 与原来逐字一致。

// FindUserByID 按租户 + 主键取账号（跨包只读门面）。
func (s *Service) FindUserByID(tenantID, id uint) (*model.User, error) {
	return s.repo.FindUserByID(tenantID, id)
}

// FindUserBriefByID 按主键取账号的接待相关字段（跨包只读门面）。
func (s *Service) FindUserBriefByID(userID uint) (*model.User, error) {
	return s.repo.FindUserBriefByID(userID)
}

// CountOpenConversations 统计某客服名下 open 状态的会话数（跨包只读门面）。
func (s *Service) CountOpenConversations(tenantID, agentID uint) (int64, error) {
	return s.repo.CountOpenConversations(tenantID, agentID)
}

// ResetAllOnline 把全平台账号置为离线（进程启动时调用一次）。
// 放在 Service 上：main 只与各内核的 Service 打交道，不直接触碰 repository。
func (s *Service) ResetAllOnline() error {
	return s.repo.ResetAllOnline()
}
