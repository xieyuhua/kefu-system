package repository

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// MessageRepository 消息（messages）的数据访问层。
type MessageRepository struct{}

// NewMessageRepository 创建数据访问层实例。
func NewMessageRepository() *MessageRepository { return &MessageRepository{} }

// ---------------- 消息（Message）数据访问 ----------------
//
// 游标方向约定（务必与前端翻页语义一致）：
//   - ListMessagesBefore：id < beforeID，向上翻页 / 首次进入取最新 N 条，返回按 id 升序
//   - ListMessagesAfter ：id > sinceID，正向增量补拉（离线同步），返回按 id 升序
// 两个方向相反，不可混用。

// CreateMessage 新增一条消息，成功后回填主键与时间戳。
func (r *MessageRepository) CreateMessage(m *model.Message) error {
	return storage.Default().DB().Create(m).Error
}

// AddSystemMessage 落一条系统消息，并把所属会话的 message_count 加一。
// 「系统提示语 + 计数」必须同生同灭，故放在同一事务里。
func (r *MessageRepository) AddSystemMessage(m *model.Message) error {
	return storage.Default().Tx(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Model(&model.Conversation{}).Where("id = ?", m.ConversationID).
			Update("message_count", gorm.Expr("message_count + 1")).Error
	})
}

// UpdateConversationDigest 定点回写会话摘要相关字段（最近一条消息、最近消息时间、消息数、未读数）。
// 只更新传入字段、不整行保存，避免覆盖并发写入的归属与状态。
func (r *MessageRepository) UpdateConversationDigest(convID uint, fields map[string]interface{}) error {
	return storage.Default().DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// BumpConversationMessageCount 会话消息计数增减（delta 可为负）。
func (r *MessageRepository) BumpConversationMessageCount(convID uint, delta int) error {
	return storage.Default().DB().Model(&model.Conversation{}).Where("id = ?", convID).
		Update("message_count", gorm.Expr("message_count + ?", delta)).Error
}

// CountMessagesOfConversation 统计会话内的消息条数。
func (r *MessageRepository) CountMessagesOfConversation(tenantID, convID uint) (int64, error) {
	var n int64
	if err := storage.Default().DB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// MarkMessagesRead 把会话内消息标记为已读。
func (r *MessageRepository) MarkMessagesRead(tenantID, convID uint) error {
	return storage.Default().DB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Update("read", true).Error
}

// ListMessagesBefore 向前翻页：取 id < beforeID 的最近 limit 条，返回按 id 升序。
// beforeID = 0 表示无游标（客户端首次进入会话），直接取最新 limit 条。
// 内部先按 id desc 取最近若干条再整体反转，保证拿到的是紧邻的上一页。
func (r *MessageRepository) ListMessagesBefore(tenantID, convID, beforeID uint, limit int) ([]*model.Message, error) {
	q := storage.Default().DB().Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
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
func (r *MessageRepository) ListMessagesAfter(tenantID, convID, sinceID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.Default().DB().Where("tenant_id = ? AND conversation_id = ? AND id > ?", tenantID, convID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindLastSystemMessage 取会话最后一条系统消息（id 倒序第一条）。
// 未命中返回 gorm.ErrRecordNotFound，表示该会话还没有系统提示语。
func (r *MessageRepository) FindLastSystemMessage(tenantID, convID uint) (*model.Message, error) {
	var m model.Message
	if err := storage.Default().DB().Where("tenant_id = ? AND conversation_id = ? AND type = 'system'", tenantID, convID).
		Order("id desc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindFirstUserMessage 取会话首条非系统消息（id 正序第一条），供对话记录列表预览对话开头。
func (r *MessageRepository) FindFirstUserMessage(convID uint) (*model.Message, error) {
	var m model.Message
	if err := storage.Default().DB().Where("conversation_id = ? AND type != 'system'", convID).
		Order("id asc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindLastMessageOfConversation 取会话最后一条消息（id 倒序第一条）。
// 供单条会话 VO 组装时判定「最后一条是否客户消息」（即客服是否还没回复）。
// 未命中返回 gorm.ErrRecordNotFound（该会话还没有任何消息）。
func (r *MessageRepository) FindLastMessageOfConversation(tenantID, convID uint) (*model.Message, error) {
	var m model.Message
	if err := storage.Default().DB().Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Order("id desc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindLastMessagesOfConversations 批量取一批会话各自的最后一条消息，返回 conversation_id → 消息。
//
// 会话列表要按「最后一条消息是不是客户发的」判断未回复状态，逐会话查一次就是 N+1；
// 这里用一条聚合查询取回整页（每个会话的 MAX(id) 再回表），SQL 条件与行数都与会话数同级：
//
//	SELECT * FROM messages
//	WHERE tenant_id = ? AND conversation_id IN (…)
//	  AND id IN (SELECT MAX(id) FROM messages WHERE tenant_id = ? AND conversation_id IN (…) GROUP BY conversation_id)
//
// 返回的 map 只含「有消息的会话」，调用方按会话 id 取不到键即表示该会话没有消息。
func (r *MessageRepository) FindLastMessagesOfConversations(tenantID uint, convIDs []uint) (map[uint]*model.Message, error) {
	out := make(map[uint]*model.Message, len(convIDs))
	if len(convIDs) == 0 {
		return out, nil
	}
	// 子查询：每个会话的最大消息 id（与外表同表，MySQL 允许在 SELECT 的子查询里读同一张表）
	sub := storage.Default().DB().Model(&model.Message{}).
		Select("MAX(id)").
		Where("tenant_id = ? AND conversation_id IN ?", tenantID, convIDs).
		Group("conversation_id")
	var list []*model.Message
	if err := storage.Default().DB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id IN ?", tenantID, convIDs).
		Where("id IN (?)", sub).
		Find(&list).Error; err != nil {
		return nil, err
	}
	for _, m := range list {
		out[m.ConversationID] = m
	}
	return out, nil
}

// SearchMessagesByContent 在会话内按内容关键词搜索：先按 id 倒序取最近 limit 条，再反转为升序，
// 与消息流顺序保持一致。
func (r *MessageRepository) SearchMessagesByContent(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.Default().DB().Where("tenant_id = ? AND conversation_id = ? AND content LIKE ?",
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

// ListMessagesOfConversations 取一批会话中 id > afterID 的消息，按 id 升序，最多 limit 条。
// 调用方按「多取一条」判断是否还有更多（has_more）。
func (r *MessageRepository) ListMessagesOfConversations(tenantID uint, convIDs []uint, afterID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := storage.Default().DB().Where("tenant_id = ? AND conversation_id IN ? AND id > ?", tenantID, convIDs, afterID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
