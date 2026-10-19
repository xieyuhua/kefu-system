package conversation

import "kefu-server/internal/platform/model"

// ---------------- 供其它内核 / 端复用的只读门面 ----------------
// 端模块（modules/client、modules/agent）与 automation 需要「按主键取一条会话」这类单表读取，
// 跨包只能走 Service：这里的每个方法都只是把调用转给本包 repository，SQL 与原来逐字一致。

// FindConversationByID 按租户 + 主键读取会话（跨包只读门面）。
func (s *Service) FindConversationByID(tenantID, id uint) (*model.Conversation, error) {
	return s.repo.FindConversationByID(tenantID, id)
}

// FindLatestConversationByCustomer 取某客户最近一条会话（跨包只读门面）。
func (s *Service) FindLatestConversationByCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return s.repo.FindLatestConversationByCustomer(tenantID, customerID)
}

// CreateConversation 新增会话（跨包写入门面，供演示入口新建会话）。
func (s *Service) CreateConversation(c *model.Conversation) error {
	return s.repo.CreateConversation(c)
}

// ListVisibleConversationIDs 列出客服可见范围内的会话 ID（跨包只读门面，供端侧离线同步）。
func (s *Service) ListVisibleConversationIDs(tenantID, userID uint) ([]uint, error) {
	return s.repo.ListVisibleConversationIDs(tenantID, userID)
}

// ListMessagesOfConversations 取一批会话中 id > afterID 的消息（跨包只读门面，供端侧离线同步）。
func (s *Service) ListMessagesOfConversations(tenantID uint, convIDs []uint, afterID uint, limit int) ([]*model.Message, error) {
	return s.repo.ListMessagesOfConversations(tenantID, convIDs, afterID, limit)
}
