package repository

// 「谁能看到哪些会话」属于数据访问规则的一部分：列表、统计、批量归档必须共用同一套条件，
// 否则会出现「列表看不到、统计却算进去」的口径漂移。所以可见范围统一收敛在
// applyVisibility 里，service 只负责把角色信息（userID / isAdmin / scope）传进来。
import (
	"database/sql"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// ConversationVisibilityRepository 会话可见范围的数据访问层：
// 按角色（管理员 / 普通客服）与归属（mine / 渠道准入）拼装过滤条件。
type ConversationVisibilityRepository struct{}

// NewConversationVisibilityRepository 创建数据访问层实例。
func NewConversationVisibilityRepository() *ConversationVisibilityRepository {
	return &ConversationVisibilityRepository{}
}

// applyVisibility 施加角色可见范围条件：
//   - scope=mine：只看自己接待的
//   - 管理员：全量可见，不做额外过滤
//   - 普通客服：自己接待的 + 自己可接待渠道下尚未被领取的（agent_id = 0）；
//     已被同渠道其他客服领取的会话对其隐藏（渠道互斥）；
//     同一客户已被其他客服接待时，该客户的其它会话也一并隐藏（客户互斥，避免抢同一客户）。
//
// 已归档过滤不在这里做（列表用 IncludeArchived 控制，统计恒为 false），由调用方自行施加。
func (r *ConversationVisibilityRepository) applyVisibility(q *gorm.DB, tenantID, userID uint, isAdmin bool, scope string) *gorm.DB {
	if scope == "mine" {
		return q.Where("agent_id = ?", userID)
	}
	if isAdmin {
		return q
	}
	var chIDs []uint
	// 渠道集合查询失败按「无渠道」处理，退回只看自己的会话（与原逻辑一致）
	storage.Default().DB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
	if len(chIDs) > 0 {
		// 自己接待的，或（未接待 且 在自己渠道内 且 该用户尚未被其他客服接待）。
		// 「该用户」按 customers.external_id 跨渠道判定：同一 uid 在多个渠道会有多条 customer 记录，
		// 只要其中任一会话被其他客服接待，其它渠道的待接待会话也一并隐藏（客户互斥）。
		//
		// 性能：先一次性查出「已被同事接待的用户 external_id」（1 条 SQL，命中
		// conversations(agent_id) 与 customers(external_id) 索引），再用 IN 过滤。
		// 原来是逐行相关子查询（每行都要 join + 一次标量子查询），页内 20 行就是 20 次，
		// 跨网访问 MySQL 时单次列表被拖到 100ms 以上。
		blocked := r.handledExternalIDs(tenantID, userID)
		if len(blocked) == 0 {
			return q.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
		}
		return q.Where(
			"agent_id = ? OR (agent_id = 0 AND channel_id IN ? AND customer_id NOT IN ("+
				"SELECT id FROM customers WHERE external_id IN ?))",
			userID, chIDs, blocked,
		)
	}
	return q.Where("agent_id = ?", userID)
}

// handledExternalIDs 返回「已被其他客服接待」的客户 external_id 集合。
// 查询失败按空集处理：宁可多显示几条待接待会话，也不因为一次查询失败让列表变成空的。
func (r *ConversationVisibilityRepository) handledExternalIDs(tenantID, userID uint) []string {
	var ids []string
	_ = storage.Default().DB().Raw(
		"SELECT DISTINCT cu.external_id FROM conversations cv "+
			"JOIN customers cu ON cu.id = cv.customer_id "+
			"WHERE cv.tenant_id = ? AND cv.agent_id <> 0 AND cv.agent_id <> ? AND cv.archived = ? "+
			"AND cu.external_id <> ''",
		tenantID, userID, false,
	).Scan(&ids).Error
	return ids
}

// QueryConversations 分页列出会话（按最近消息时间倒序），同时返回满足条件的总数。
// 可见范围见 applyVisibility；默认不返回已归档会话。
func (r *ConversationVisibilityRepository) QueryConversations(tenantID, userID uint, isAdmin bool, f ConversationFilter) ([]*model.Conversation, int64, error) {
	q := storage.Default().DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	// 默认不展示已归档会话；IncludeArchived 时一并返回（历史档案 / 统计用）
	if !f.IncludeArchived {
		q = q.Where("archived = ?", false)
	}
	if f.ChannelID > 0 {
		q = q.Where("channel_id = ?", f.ChannelID)
	}
	if f.Status != "" && f.Status != "all" {
		q = q.Where("status = ?", f.Status)
	}
	q = r.applyVisibility(q, tenantID, userID, isAdmin, f.Scope)
	if f.Keyword != "" {
		like := "%" + strings.TrimSpace(f.Keyword) + "%"
		q = q.Where(
			"last_message LIKE ? OR customer_id IN (SELECT id FROM customers WHERE nickname LIKE ?)",
			like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := storage.Default().Paginate(f.Page, f.PageSize)
	var list []*model.Conversation
	if err := q.Order("last_message_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountVisibleConversations 统计当前角色可见、且未归档的会话数；status 为空表示不限状态。
// 口径必须与 QueryConversations 保持一致（同一套可见范围 + 默认排除已归档），
// 否则会出现「侧边栏显示未接待 1，点进去却是空的」。
func (r *ConversationVisibilityRepository) CountVisibleConversations(tenantID, userID uint, isAdmin bool, status string) (int64, error) {
	q := storage.Default().DB().Model(&model.Conversation{}).
		Where("tenant_id = ?", tenantID).
		Where("archived = ?", false)
	q = r.applyVisibility(q, tenantID, userID, isAdmin, "")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// SumVisibleConversationUnread 把当前角色可见、未归档会话的 unread 求和。
// 会话 unread 字段 = 该会话未读消息条数，因此这个和就是「未读消息总数」。
func (r *ConversationVisibilityRepository) SumVisibleConversationUnread(tenantID, userID uint, isAdmin bool) (int64, error) {
	q := storage.Default().DB().Model(&model.Conversation{}).
		Where("tenant_id = ?", tenantID).
		Where("archived = ?", false)
	q = r.applyVisibility(q, tenantID, userID, isAdmin, "")
	var total sql.NullInt64
	if err := q.Select("SUM(unread)").Row().Scan(&total); err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}
