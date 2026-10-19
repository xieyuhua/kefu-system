package conversation

// 本文件是 core/conversation 的数据访问层。
//
// 写表范围：conversations / messages。
// 跨表只读：客服可见范围要读 agent_channels，对话记录要读 customers（子查询），
// 这些只读语句原本就在这里，保持不变（不写对方内核的表）。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"database/sql"
	"strings"
	"time"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// Repository 会话内核的数据访问层（无状态，可随时构造）。
type Repository struct{}

// NewRepository 创建数据访问层实例。
func NewRepository() *Repository { return &Repository{} }

// ---------------- 会话（Conversation）数据访问 ----------------
//
// 「谁能看到哪些会话」属于数据访问规则的一部分：列表、统计、批量归档必须共用同一套条件，
// 否则会出现「列表看不到、统计却算进去」的口径漂移。所以可见范围统一收敛在
// applyVisibility 里，service 只负责把角色信息（userID / isAdmin / scope）传进来。

// ConversationFilter 会话列表查询条件。
// 由 service 层的 ConvQuery 转换而来 —— 跨层不直接传递 service 包的类型。
type ConversationFilter struct {
	ChannelID       uint
	Status          string
	Scope           string // mine：只看自己接待的
	Keyword         string // 匹配最近消息内容 / 客户昵称
	IncludeArchived bool   // 是否包含已归档会话
	Page            int
	PageSize        int
}

// applyVisibility 施加角色可见范围条件：
//   - scope=mine：只看自己接待的
//   - 管理员：全量可见，不做额外过滤
//   - 普通客服：自己接待的 + 自己可接待渠道下尚未被领取的（agent_id = 0）；
//     已被同渠道其他客服领取的会话对其隐藏（实现互斥接待）
//
// 已归档过滤不在这里做（列表用 IncludeArchived 控制，统计恒为 false），由调用方自行施加。
func (r *Repository) applyVisibility(q *gorm.DB, userID uint, isAdmin bool, scope string) *gorm.DB {
	if scope == "mine" {
		return q.Where("agent_id = ?", userID)
	}
	if isAdmin {
		return q
	}
	var chIDs []uint
	// 渠道集合查询失败按「无渠道」处理，退回只看自己的会话（与原逻辑一致）
	storage.DB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
	if len(chIDs) > 0 {
		// 自己接待的，或（未接待 且 在自己渠道内）——别人已领取的自然被排除
		return q.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
	}
	return q.Where("agent_id = ?", userID)
}

// QueryConversations 分页列出会话（按最近消息时间倒序），同时返回满足条件的总数。
// 可见范围见 applyVisibility；默认不返回已归档会话。
func (r *Repository) QueryConversations(tenantID, userID uint, isAdmin bool, f ConversationFilter) ([]*model.Conversation, int64, error) {
	q := storage.DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
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
	q = r.applyVisibility(q, userID, isAdmin, f.Scope)
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
	offset, limit := storage.Paginate(f.Page, f.PageSize)
	var list []*model.Conversation
	if err := q.Order("last_message_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountVisibleConversations 统计当前角色可见、且未归档的会话数；status 为空表示不限状态。
// 口径必须与 QueryConversations 保持一致（同一套可见范围 + 默认排除已归档），
// 否则会出现「侧边栏显示未接待 1，点进去却是空的」。
func (r *Repository) CountVisibleConversations(tenantID, userID uint, isAdmin bool, status string) (int64, error) {
	q := storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ?", tenantID).
		Where("archived = ?", false)
	q = r.applyVisibility(q, userID, isAdmin, "")
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
func (r *Repository) SumVisibleConversationUnread(tenantID, userID uint, isAdmin bool) (int64, error) {
	q := storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ?", tenantID).
		Where("archived = ?", false)
	q = r.applyVisibility(q, userID, isAdmin, "")
	var total sql.NullInt64
	if err := q.Select("SUM(unread)").Row().Scan(&total); err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

// FindConversationByID 按租户 + 主键读取会话；未命中返回 gorm.ErrRecordNotFound。
func (r *Repository) FindConversationByID(tenantID, id uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindLatestConversationByCustomer 取某客户最近一条会话（id 倒序第一条）。
// 这是「同一客户始终复用最近一个会话」的定位入口，故意不过滤 closed / archived：
// 已结束或已归档的会话由上层就地重新激活，而不是另开新会话（聊天记录自然合并）。
// 未命中返回 gorm.ErrRecordNotFound —— 表示该客户首次咨询。
func (r *Repository) FindLatestConversationByCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Order("id desc").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversationsByCustomer 按客户列出历史会话（新的在前）；limit <= 0 时取 50 条。
func (r *Repository) ListConversationsByCustomer(tenantID, customerID uint, limit int) ([]*model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []*model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CreateConversation 新增会话，成功后回填主键与时间戳。
func (r *Repository) CreateConversation(c *model.Conversation) error {
	return storage.DB().Create(c).Error
}

// SaveConversation 全字段保存会话（状态 / 归属 / 最近消息 / 消息数 / 未读 / 归档等）。
func (r *Repository) SaveConversation(c *model.Conversation) error {
	return storage.DB().Save(c).Error
}

// UpdateConversationByID 按主键更新会话的指定字段（归档、分配、状态、摘要、未读清零）。
// 用 map 更新才能把 agent_id 显式写成 0，保持原行为；调用方必须先校验过归属。
func (r *Repository) UpdateConversationByID(convID uint, fields map[string]interface{}) error {
	return storage.DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// UpdateConversationInTenant 按租户 + 主键更新会话的指定字段。
// 主键来自接口入参时一律用这个版本，跨租户 id 落到「未更新」而不是越权改到别家数据。
func (r *Repository) UpdateConversationInTenant(tenantID, convID uint, fields map[string]interface{}) error {
	return storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, convID).
		Updates(fields).Error
}

// ClearConversationUnread 会话未读清零（客服打开会话 / 置读时调用）。
func (r *Repository) ClearConversationUnread(tenantID, convID uint) error {
	return r.UpdateConversationInTenant(tenantID, convID, map[string]interface{}{"unread": 0})
}

// SetConversationUnarchived 取消归档并清空归档时间（恢复回工作台列表）。
func (r *Repository) SetConversationUnarchived(tenantID, convID uint) error {
	return r.UpdateConversationInTenant(tenantID, convID,
		map[string]interface{}{"archived": false, "archived_at": nil})
}

// ReopenArchivedConversation 仅在「已归档」时取消归档。
// 客户重新咨询时自动恢复，条件里带 archived = true 让重复调用保持幂等。
func (r *Repository) ReopenArchivedConversation(tenantID, convID uint) error {
	return storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ? AND archived = ?", tenantID, convID, true).
		Updates(map[string]interface{}{"archived": false, "archived_at": nil}).Error
}

// ListConversationAgentRefs 批量取会话的主键与归属客服。
// 批量归档前用它判断哪些会话需要释放归属（agent_id = 0 回到待接待池）。
func (r *Repository) ListConversationAgentRefs(tenantID uint, ids []uint) ([]*model.Conversation, error) {
	var list []*model.Conversation
	if err := storage.DB().Select("id", "agent_id").
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteConversationWithMessages 在同一事务内删除会话及其全部消息（不可恢复）。
func (r *Repository) DeleteConversationWithMessages(tenantID, convID uint) error {
	return storage.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
			Delete(&model.Message{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, convID).
			Delete(&model.Conversation{}).Error
	})
}

// ---------------- 全局对话记录查询（管理后台） ----------------

// ConversationRecordFilter 全局对话记录查询条件。
// 时间条件由上层按天解析好后以半开区间传入（From 含、To 不含）。
type ConversationRecordFilter struct {
	Keyword   string // 昵称 / 手机号 / 外部 ID / 消息内容
	ChannelID uint
	Status    string // 空或 all 表示不限
	AgentID   uint
	From      *time.Time
	To        *time.Time
	Page      int
	PageSize  int
}

// SearchConversationRecords 跨会话检索对话记录（支持按消息内容全文匹配），
// 返回当页会话与总数，按最近消息时间倒序。
func (r *Repository) SearchConversationRecords(tenantID uint, f ConversationRecordFilter) ([]*model.Conversation, int64, error) {
	q := storage.DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		// 昵称/手机号/外部ID 匹配，或该会话下存在内容匹配的消息
		q = q.Where(
			"(customer_id IN (SELECT id FROM customers WHERE nickname LIKE ? OR phone LIKE ? OR external_id LIKE ?) "+
				"OR id IN (SELECT conversation_id FROM messages WHERE content LIKE ?))",
			like, like, like, like,
		)
	}
	if f.ChannelID > 0 {
		q = q.Where("channel_id = ?", f.ChannelID)
	}
	if f.Status != "" && f.Status != "all" {
		q = q.Where("status = ?", f.Status)
	}
	if f.AgentID > 0 {
		q = q.Where("agent_id = ?", f.AgentID)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at < ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := storage.Paginate(f.Page, f.PageSize)
	var list []*model.Conversation
	if err := q.Order("last_message_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListOpenConversationsByAgent 取某客服当前「进行中」的会话。
// 用于客服离线时释放其占用的会话（回到待接待池）。
func (r *Repository) ListOpenConversationsByAgent(tenantID, agentID uint) ([]*model.Conversation, error) {
	var convs []*model.Conversation
	err := storage.DB().Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, "open").
		Find(&convs).Error
	return convs, err
}

// ---------------- 消息（Message）数据访问 ----------------
//
// 游标方向约定（务必与前端翻页语义一致）：
//   - ListMessagesBefore：id < beforeID，向上翻页 / 首次进入取最新 N 条，返回按 id 升序
//   - ListMessagesAfter ：id > sinceID，正向增量补拉（离线同步），返回按 id 升序
// 两个方向相反，不可混用。

// CreateMessage 新增一条消息，成功后回填主键与时间戳。
func (r *Repository) CreateMessage(m *model.Message) error {
	return storage.DB().Create(m).Error
}

// AddSystemMessage 落一条系统消息，并把所属会话的 message_count 加一。
// 「系统提示语 + 计数」必须同生同灭，故放在同一事务里。
func (r *Repository) AddSystemMessage(m *model.Message) error {
	return storage.Tx(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Model(&model.Conversation{}).Where("id = ?", m.ConversationID).
			Update("message_count", gorm.Expr("message_count + 1")).Error
	})
}

// UpdateConversationDigest 定点回写会话摘要相关字段（最近一条消息、最近消息时间、消息数、未读数）。
// 只更新传入字段、不整行保存，避免覆盖并发写入的归属与状态。
func (r *Repository) UpdateConversationDigest(convID uint, fields map[string]interface{}) error {
	return storage.DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// BumpConversationMessageCount 会话消息计数增减（delta 可为负）。
func (r *Repository) BumpConversationMessageCount(convID uint, delta int) error {
	return storage.DB().Model(&model.Conversation{}).Where("id = ?", convID).
		Update("message_count", gorm.Expr("message_count + ?", delta)).Error
}

// CountMessagesOfConversation 统计会话内的消息条数。
func (r *Repository) CountMessagesOfConversation(tenantID, convID uint) (int64, error) {
	var n int64
	if err := storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// MarkMessagesRead 把会话内消息标记为已读。
func (r *Repository) MarkMessagesRead(tenantID, convID uint) error {
	return storage.DB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Update("read", true).Error
}

// ListMessagesBefore 向前翻页：取 id < beforeID 的最近 limit 条，返回按 id 升序。
// beforeID = 0 表示无游标（客户端首次进入会话），直接取最新 limit 条。
// 内部先按 id desc 取最近若干条再整体反转，保证拿到的是紧邻的上一页。
func (r *Repository) ListMessagesBefore(tenantID, convID, beforeID uint, limit int) ([]*model.Message, error) {
	q := storage.DB().Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []*model.Message
	if err := q.Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	reverseMessages(list)
	return list, nil
}

// ListMessagesAfter 正向增量拉取：取 id > sinceID 的消息，按 id 升序。
// 供用户端 / APP 离线同步补拉，与 ListMessagesBefore 的翻页方向相反。
func (r *Repository) ListMessagesAfter(tenantID, convID, sinceID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.DB().Where("tenant_id = ? AND conversation_id = ? AND id > ?", tenantID, convID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindLastSystemMessage 取会话最后一条系统消息（id 倒序第一条）。
// 未命中返回 gorm.ErrRecordNotFound，表示该会话还没有系统提示语。
func (r *Repository) FindLastSystemMessage(tenantID, convID uint) (*model.Message, error) {
	var m model.Message
	if err := storage.DB().Where("tenant_id = ? AND conversation_id = ? AND type = 'system'", tenantID, convID).
		Order("id desc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindFirstUserMessage 取会话首条非系统消息（id 正序第一条），供对话记录列表预览对话开头。
func (r *Repository) FindFirstUserMessage(convID uint) (*model.Message, error) {
	var m model.Message
	if err := storage.DB().Where("conversation_id = ? AND type != 'system'", convID).
		Order("id asc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// SearchMessagesByContent 在会话内按内容关键词搜索：先按 id 倒序取最近 limit 条，再反转为升序，
// 与消息流顺序保持一致。
func (r *Repository) SearchMessagesByContent(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.DB().Where("tenant_id = ? AND conversation_id = ? AND content LIKE ?",
		tenantID, convID, "%"+keyword+"%").
		Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	reverseMessages(list)
	return list, nil
}

// reverseMessages 原地反转为 id 升序（列表均为 id desc 取出后使用）。纯工具，无状态。
func reverseMessages(list []*model.Message) {
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
}
