package stats

// 本文件是共享统计内核（core/stats）的数据访问层：概览页的统计 SQL（跨域只读聚合）。
//
// 表范围：conversations / messages / customers / orders / users / channels 六张表的只读计数。
// 这些口径原本落在 modules/admin 的 repository 里，但「会话/消息/客户/订单/账号/渠道」的
// 只读聚合是各端都可能要看的数据（后台概览页、将来客服端的个人数据），端与端之间不允许
// 互相 import，若各自复制 SQL 就会漂移，因此下沉为共享内核。
//
// 统计口径约定（改动前先对齐产品口径，这些条件一个字都不能动）：
//   - 「已接待」= agent_id > 0；「已结束」= status = 'closed'；「未接待」= status = 'pending'
//   - 时间窗口统一写 created_at >= from AND created_at < to（左闭右开）
//   - 每一项计数都是一条独立 SQL，不复用条件：多个计数拼在同一条链上会互相污染条件，
//     因此不要为了「省一次查询」把 pending/open/closed 合并成一条 SQL。
//
// 依赖方向：core/stats → platform → pkg。本内核只直接查表，不调用其它 core 包，也不 import 任何 modules。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"time"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// Repository 统计内核的数据访问层（无状态，可随时构造）。
type Repository struct{}

// NewRepository 创建数据访问层实例。
func NewRepository() *Repository { return &Repository{} }

// countRows 执行计数并按 (int64, error) 返回，省掉每处 var/err 样板（纯工具，无状态）。
func countRows(q *gorm.DB) (int64, error) {
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountConversationsInRange 统计租户在 [from, to) 内创建的会话总数。
func (r *Repository) CountConversationsInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to))
}

// CountHandledConversationsInRange 统计 [from, to) 内已接待（agent_id > 0）的会话数。
func (r *Repository) CountHandledConversationsInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND agent_id > 0", tenantID, from, to))
}

// CountConversationsInRangeByStatus 统计 [from, to) 内指定状态（pending/open/closed）的会话数。
func (r *Repository) CountConversationsInRangeByStatus(tenantID uint, from, to time.Time, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status = ?", tenantID, from, to, status))
}

// CountMessagesInRange 统计 [from, to) 内的消息总数。
func (r *Repository) CountMessagesInRange(tenantID uint, from, to time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", tenantID, from, to))
}

// CountMessagesInRangeByDirection 统计 [from, to) 内指定方向（in=客户 / out=客服）的消息数。
func (r *Repository) CountMessagesInRangeByDirection(tenantID uint, from, to time.Time, direction string) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND direction = ?", tenantID, from, to, direction))
}

// ListUsersForStats 列出参与客服工作量统计的账号：admin 与 agent 都算（管理员常亲自坐席，
// 不能只按 role='agent' 过滤），按 role 降序、id 升序返回。
func (r *Repository) ListUsersForStats(tenantID uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ?", tenantID).
		Order("role desc, id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CountAgentConversations 统计某账号接待过的会话总数（不限状态、不限时间）。
func (r *Repository) CountAgentConversations(tenantID, agentID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID))
}

// CountAgentConversationsByStatus 统计某账号在指定状态下的会话数。
func (r *Repository) CountAgentConversationsByStatus(tenantID, agentID uint, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, status))
}

// CountAgentMessagesSince 统计某账号自 since 起的消息数（沿用原有 sender_role='user' 口径，
// 与表头「最近 N 天」对齐）。
func (r *Repository) CountAgentMessagesSince(tenantID, agentID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND sender_id = ? AND sender_role = 'user' AND created_at >= ?",
			tenantID, agentID, since))
}

// ListChannelsForStats 列出参与渠道分布统计的渠道，按 id 升序返回。
func (r *Repository) ListChannelsForStats(tenantID uint) ([]*model.Channel, error) {
	var list []*model.Channel
	if err := storage.DB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountChannelConversationsSince 统计某渠道自 since 起创建的会话总数。
func (r *Repository) CountChannelConversationsSince(tenantID, channelID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND created_at >= ?", tenantID, channelID, since))
}

// CountChannelConversationsSinceByStatus 统计某渠道自 since 起指定状态的会话数。
func (r *Repository) CountChannelConversationsSinceByStatus(tenantID, channelID uint, since time.Time, status string) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND status = ? AND created_at >= ?", tenantID, channelID, status, since))
}

// CountTenantConversations 统计租户全部会话数（汇总卡片用，不限时间）。
func (r *Repository) CountTenantConversations(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID))
}

// CountTenantHandledConversations 统计租户全部已接待会话数（agent_id > 0）。
func (r *Repository) CountTenantHandledConversations(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Conversation{}).Where("tenant_id = ? AND agent_id > 0", tenantID))
}

// CountTenantMessages 统计租户全部消息数（用于计算平均消息数/会话）。
func (r *Repository) CountTenantMessages(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Message{}).Where("tenant_id = ?", tenantID))
}

// CountTenantCustomers 统计租户客户总数。
func (r *Repository) CountTenantCustomers(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID))
}

// CountNewCustomersSince 统计自 since 起新增的客户数。
func (r *Repository) CountNewCustomersSince(tenantID uint, since time.Time) (int64, error) {
	return countRows(storage.DB().Model(&model.Customer{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, since))
}

// CountTenantOrders 统计租户订单总数。
func (r *Repository) CountTenantOrders(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.Order{}).Where("tenant_id = ?", tenantID))
}

// CountOnlineAgents 统计当前在线客服数（online = true）。
func (r *Repository) CountOnlineAgents(tenantID uint) (int64, error) {
	return countRows(storage.DB().Model(&model.User{}).
		Where("tenant_id = ? AND online = ?", tenantID, true))
}
