package repository

import (
	"kefu-server/internal/model"

	"gorm.io/gorm"
)

// ---------------- 消息（Message）数据访问 ----------------
//
// 游标方向约定（务必与前端翻页语义一致）：
//   - ListMessagesBefore：id < beforeID，向上翻页 / 首次进入取最新 N 条，返回按 id 升序
//   - ListMessagesAfter ：id > sinceID，正向增量补拉（离线同步），返回按 id 升序
// 两个方向相反，不可混用。

// CreateMessage 新增一条消息，成功后回填主键与时间戳。
func CreateMessage(m *model.Message) error {
	return db.Create(m).Error
}

// AddSystemMessage 落一条系统消息，并把所属会话的 message_count 加一。
// 「系统提示语 + 计数」必须同生同灭，故放在同一事务里。
func AddSystemMessage(m *model.Message) error {
	return Tx(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Model(&model.Conversation{}).Where("id = ?", m.ConversationID).
			Update("message_count", gorm.Expr("message_count + 1")).Error
	})
}

// UpdateConversationDigest 定点回写会话摘要相关字段（最近一条消息、最近消息时间、消息数、未读数）。
// 只更新传入字段、不整行保存，避免覆盖并发写入的归属与状态。
func UpdateConversationDigest(convID uint, fields map[string]interface{}) error {
	return db.Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// BumpConversationMessageCount 会话消息计数增减（delta 可为负）。
func BumpConversationMessageCount(convID uint, delta int) error {
	return db.Model(&model.Conversation{}).Where("id = ?", convID).
		Update("message_count", gorm.Expr("message_count + ?", delta)).Error
}

// CountMessagesOfConversation 统计会话内的消息条数。
func CountMessagesOfConversation(tenantID, convID uint) (int64, error) {
	var n int64
	if err := db.Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// MarkMessagesRead 把会话内消息标记为已读。
func MarkMessagesRead(tenantID, convID uint) error {
	return db.Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Update("read", true).Error
}

// ListMessagesBefore 向前翻页：取 id < beforeID 的最近 limit 条，返回按 id 升序。
// beforeID = 0 表示无游标（客户端首次进入会话），直接取最新 limit 条。
// 内部先按 id desc 取最近若干条再整体反转，保证拿到的是紧邻的上一页。
func ListMessagesBefore(tenantID, convID, beforeID uint, limit int) ([]*model.Message, error) {
	q := db.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
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
func ListMessagesAfter(tenantID, convID, sinceID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := db.Where("tenant_id = ? AND conversation_id = ? AND id > ?", tenantID, convID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindLastSystemMessage 取会话最后一条系统消息（id 倒序第一条）。
// 未命中返回 gorm.ErrRecordNotFound，表示该会话还没有系统提示语。
func FindLastSystemMessage(tenantID, convID uint) (*model.Message, error) {
	var m model.Message
	if err := db.Where("tenant_id = ? AND conversation_id = ? AND type = 'system'", tenantID, convID).
		Order("id desc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindFirstUserMessage 取会话首条非系统消息（id 正序第一条），供对话记录列表预览对话开头。
func FindFirstUserMessage(convID uint) (*model.Message, error) {
	var m model.Message
	if err := db.Where("conversation_id = ? AND type != 'system'", convID).
		Order("id asc").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// SearchMessagesByContent 在会话内按内容关键词搜索：先按 id 倒序取最近 limit 条，再反转为升序，
// 与消息流顺序保持一致。
func SearchMessagesByContent(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := db.Where("tenant_id = ? AND conversation_id = ? AND content LIKE ?",
		tenantID, convID, "%"+keyword+"%").
		Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	reverseMessages(list)
	return list, nil
}

// reverseMessages 原地反转为 id 升序（列表均为 id desc 取出后使用）。
func reverseMessages(list []*model.Message) {
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
}
