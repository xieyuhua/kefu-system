package repository

// 本文件是 common/repository 的数据访问层。
//
// 写表范围：conversations / messages。
// 跨表只读：客服可见范围要读 agent_channels，对话记录要读 customers（子查询），
// 这些只读语句原本就在这里，保持不变（不写对方内核的表）。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。
import (
	"strings"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// Repository 会话内核的数据访问层（无状态，可随时构造）。
type ConversationRepository struct{}

// NewRepository 创建数据访问层实例。
func NewConversationRepository() *ConversationRepository { return &ConversationRepository{} }

// ---------------- 会话（Conversation）数据访问 ----------------

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

// FindConversationByID 按租户 + 主键读取会话；未命中返回 gorm.ErrRecordNotFound。
func (r *ConversationRepository) FindConversationByID(tenantID, id uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindLatestConversationByCustomer 取某客户最近一条会话（id 倒序第一条）。
// 这是「同一客户始终复用最近一个会话」的定位入口，故意不过滤 closed / archived：
// 已结束或已归档的会话由上层就地重新激活，而不是另开新会话（聊天记录自然合并）。
// 未命中返回 gorm.ErrRecordNotFound —— 表示该客户首次咨询。
func (r *ConversationRepository) FindLatestConversationByCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.Default().DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Order("id desc").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversationsByCustomer 按客户列出历史会话（新的在前）；limit <= 0 时取 50 条。
func (r *ConversationRepository) ListConversationsByCustomer(tenantID, customerID uint, limit int) ([]*model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []*model.Conversation
	if err := storage.Default().DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountCustomerHandledByOthers 统计「与 customerID 同 external_id（跨渠道同一用户）的客户」
// 是否有被 excludeAgentID 之外客服接待的未归档会话。用于领取时的客户互斥校验。
func (r *ConversationRepository) CountCustomerHandledByOthers(tenantID, customerID, excludeAgentID uint) (int64, error) {
	var n int64
	err := storage.Default().DB().Raw(`
		SELECT COUNT(*)
		FROM conversations cv
		JOIN customers cu ON cu.id = cv.customer_id
		WHERE cv.tenant_id = ?
		  AND cu.external_id = (SELECT external_id FROM customers WHERE id = ?)
		  AND cv.agent_id <> 0 AND cv.agent_id <> ?
		  AND cv.archived = ?
	`, tenantID, customerID, excludeAgentID, false).Scan(&n).Error
	return n, err
}

// CreateConversation 新增会话，成功后回填主键与时间戳。
func (r *ConversationRepository) CreateConversation(c *model.Conversation) error {
	return storage.Default().DB().Create(c).Error
}

// SaveConversation 全字段保存会话（状态 / 归属 / 最近消息 / 消息数 / 未读 / 归档等）。
func (r *ConversationRepository) SaveConversation(c *model.Conversation) error {
	return storage.Default().DB().Save(c).Error
}

// UpdateConversationByID 按主键更新会话的指定字段（归档、分配、状态、摘要、未读清零）。
// 用 map 更新才能把 agent_id 显式写成 0，保持原行为；调用方必须先校验过归属。
func (r *ConversationRepository) UpdateConversationByID(convID uint, fields map[string]interface{}) error {
	return storage.Default().DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// UpdateConversationInTenant 按租户 + 主键更新会话的指定字段。
// 主键来自接口入参时一律用这个版本，跨租户 id 落到「未更新」而不是越权改到别家数据。
func (r *ConversationRepository) UpdateConversationInTenant(tenantID, convID uint, fields map[string]interface{}) error {
	return storage.Default().DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, convID).
		Updates(fields).Error
}

// ClearConversationUnread 会话未读清零（客服打开会话 / 置读时调用）。
func (r *ConversationRepository) ClearConversationUnread(tenantID, convID uint) error {
	return r.UpdateConversationInTenant(tenantID, convID, map[string]interface{}{"unread": 0})
}

// SetConversationUnarchived 取消归档并清空归档时间（恢复回工作台列表）。
func (r *ConversationRepository) SetConversationUnarchived(tenantID, convID uint) error {
	return r.UpdateConversationInTenant(tenantID, convID,
		map[string]interface{}{"archived": false, "archived_at": nil})
}

// ReopenArchivedConversation 仅在「已归档」时取消归档。
// 客户重新咨询时自动恢复，条件里带 archived = true 让重复调用保持幂等。
func (r *ConversationRepository) ReopenArchivedConversation(tenantID, convID uint) error {
	return storage.Default().DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ? AND archived = ?", tenantID, convID, true).
		Updates(map[string]interface{}{"archived": false, "archived_at": nil}).Error
}

// ListConversationAgentRefs 批量取会话的主键与归属客服。
// 批量归档前用它判断哪些会话需要释放归属（agent_id = 0 回到待接待池）。
func (r *ConversationRepository) ListConversationAgentRefs(tenantID uint, ids []uint) ([]*model.Conversation, error) {
	var list []*model.Conversation
	if err := storage.Default().DB().Select("id", "agent_id").
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteConversationWithMessages 在同一事务内删除会话及其全部消息（不可恢复）。
func (r *ConversationRepository) DeleteConversationWithMessages(tenantID, convID uint) error {
	return storage.Default().Tx(func(tx *gorm.DB) error {
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
func (r *ConversationRepository) SearchConversationRecords(tenantID uint, f ConversationRecordFilter) ([]*model.Conversation, int64, error) {
	q := storage.Default().DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
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
	offset, limit := storage.Default().Paginate(f.Page, f.PageSize)
	var list []*model.Conversation
	if err := q.Order("last_message_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListOpenConversationsByAgent 取某客服当前「进行中」的会话。
// 用于客服离线时释放其占用的会话（回到待接待池）。
func (r *ConversationRepository) ListOpenConversationsByAgent(tenantID, agentID uint) ([]*model.Conversation, error) {
	var convs []*model.Conversation
	err := storage.Default().DB().Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, "open").
		Find(&convs).Error
	return convs, err
}

// ---------------- 端侧离线同步（可见会话范围 / 批量消息） ----------------
//
// 这两条只读查询原在 agent 的 repository 里，但读的是 conversations / messages 表，
// 属于会话域；端与端之间不允许互相 import，故归位到本内核（SQL 与签名逐字不变）。

// ListVisibleConversationIDs 列出客服可见范围内的会话 ID：
// 自己接待的 + 自己渠道下尚未分配的（与原离线同步口径一致）。
func (r *ConversationRepository) ListVisibleConversationIDs(tenantID, userID uint) ([]uint, error) {
	var chIDs []uint
	if err := storage.Default().DB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).
		Pluck("channel_id", &chIDs).Error; err != nil {
		return nil, err
	}
	var convIDs []uint
	q := storage.Default().DB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if len(chIDs) > 0 {
		q = q.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
	} else {
		q = q.Where("agent_id = ?", userID)
	}
	if err := q.Pluck("id", &convIDs).Error; err != nil {
		return nil, err
	}
	return convIDs, nil
}
