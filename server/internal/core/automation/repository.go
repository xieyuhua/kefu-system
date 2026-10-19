package automation

// 本文件是 core/automation 的数据访问层。
//
// 写表范围：auto_replies（本内核自己的表）。
// 例外（沿用原实现）：一次自动回复判定要读会话状态、回写会话摘要并落一条机器人消息，
// 这三条语句只服务本域（方法名都带 AutoReply 前缀），因此就近保留在本文件，
// 与 core/conversation 的会话/消息读写互不替代。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// Repository 自动回复内核的数据访问层（无状态，可随时构造）。
type Repository struct{}

// NewRepository 创建数据访问层实例。
func NewRepository() *Repository { return &Repository{} }

// ---------------- 自动回复规则（AutoReply）数据访问 ----------------

// ListAutoRepliesByTenant 列出租户的全部规则，优先级高的在前、同优先级按 id 升序。
func (r *Repository) ListAutoRepliesByTenant(tenantID uint) ([]*model.AutoReply, error) {
	var list []*model.AutoReply
	if err := storage.DB().Where("tenant_id = ?", tenantID).
		Order("priority desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListEnabledAutoReplies 列出参与关键词匹配的启用规则（enabled = true），排序同上。
func (r *Repository) ListEnabledAutoReplies(tenantID uint) ([]*model.AutoReply, error) {
	var list []*model.AutoReply
	if err := storage.DB().Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Order("priority desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindAutoReplyByID 按租户 + 主键查询规则；未命中返回 gorm.ErrRecordNotFound。
func (r *Repository) FindAutoReplyByID(tenantID, id uint) (*model.AutoReply, error) {
	var ar model.AutoReply
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&ar).Error; err != nil {
		return nil, err
	}
	return &ar, nil
}

// InsertAutoReply 新增规则，成功后回填主键与时间戳。
func (r *Repository) InsertAutoReply(ar *model.AutoReply) error {
	return storage.DB().Create(ar).Error
}

// SaveAutoReply 保存规则全字段（更新路径）。
func (r *Repository) SaveAutoReply(ar *model.AutoReply) error {
	return storage.DB().Save(ar).Error
}

// RemoveAutoReply 按租户 + 主键删除规则。
func (r *Repository) RemoveAutoReply(tenantID, id uint) error {
	return storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.AutoReply{}).Error
}

// UpdateAutoReplyHitCount 回写规则的命中次数（命中即 +1，按主键更新，不做租户过滤）。
func (r *Repository) UpdateAutoReplyHitCount(id uint, hitCount int64) error {
	return storage.DB().Model(&model.AutoReply{}).Where("id = ?", id).Update("hit_count", hitCount).Error
}

// FindAutoReplyChannel 规则回显渠道名用：按主键直取渠道（只读 channels 表）。
// 规则可能指向历史渠道，故这里不叠加租户过滤，保持与原实现一致。
func (r *Repository) FindAutoReplyChannel(channelID uint) (*model.Channel, error) {
	var ch model.Channel
	if err := storage.DB().First(&ch, channelID).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ---------------- 自动回复同步链路所需的会话 / 消息读写 ----------------

// FindAutoReplyConversation 按租户 + 主键读取会话（判定是否已由人工接待）。
func (r *Repository) FindAutoReplyConversation(tenantID, convID uint) (*model.Conversation, error) {
	var conv model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateAutoReplyConversation 按主键更新会话的指定字段（转人工置 pending / 回写消息摘要）。
// 用 map 更新才能把 agent_id 显式写成 0，保持原行为。
func (r *Repository) UpdateAutoReplyConversation(convID uint, fields map[string]interface{}) error {
	return storage.DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// CreateAutoReplyMessage 落一条机器人发出的消息，成功后回填主键。
func (r *Repository) CreateAutoReplyMessage(m *model.Message) error {
	return storage.DB().Create(m).Error
}
