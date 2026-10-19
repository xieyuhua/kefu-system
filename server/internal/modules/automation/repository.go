package automation

// 本文件是 automation 模块的数据访问层（原 repository/autoreply.go）。
//
// 写表范围：auto_replies（本模块自己的表）。
// 例外（沿用原实现）：一次自动回复判定要读会话状态、回写会话摘要并落一条机器人消息，
// 这三条语句只服务本域（函数名都带 AutoReply 前缀），因此就近保留在本文件，
// 与 chat 模块的会话/消息读写互不替代。

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 自动回复规则（AutoReply）数据访问 ----------------

// ListAutoRepliesByTenant 列出租户的全部规则，优先级高的在前、同优先级按 id 升序。
func ListAutoRepliesByTenant(tenantID uint) ([]*model.AutoReply, error) {
	var list []*model.AutoReply
	if err := storage.DB().Where("tenant_id = ?", tenantID).
		Order("priority desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListEnabledAutoReplies 列出参与关键词匹配的启用规则（enabled = true），排序同上。
func ListEnabledAutoReplies(tenantID uint) ([]*model.AutoReply, error) {
	var list []*model.AutoReply
	if err := storage.DB().Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Order("priority desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindAutoReplyByID 按租户 + 主键查询规则；未命中返回 gorm.ErrRecordNotFound。
func FindAutoReplyByID(tenantID, id uint) (*model.AutoReply, error) {
	var r model.AutoReply
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// InsertAutoReply 新增规则，成功后回填主键与时间戳。
func InsertAutoReply(r *model.AutoReply) error {
	return storage.DB().Create(r).Error
}

// SaveAutoReply 保存规则全字段（更新路径）。
func SaveAutoReply(r *model.AutoReply) error {
	return storage.DB().Save(r).Error
}

// RemoveAutoReply 按租户 + 主键删除规则。
func RemoveAutoReply(tenantID, id uint) error {
	return storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.AutoReply{}).Error
}

// UpdateAutoReplyHitCount 回写规则的命中次数（命中即 +1，按主键更新，不做租户过滤）。
func UpdateAutoReplyHitCount(id uint, hitCount int64) error {
	return storage.DB().Model(&model.AutoReply{}).Where("id = ?", id).Update("hit_count", hitCount).Error
}

// FindAutoReplyChannel 规则回显渠道名用：按主键直取渠道（只读 channels 表）。
// 规则可能指向历史渠道，故这里不叠加租户过滤，保持与原实现一致。
func FindAutoReplyChannel(channelID uint) (*model.Channel, error) {
	var ch model.Channel
	if err := storage.DB().First(&ch, channelID).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ---------------- 自动回复同步链路所需的会话 / 消息读写 ----------------

// FindAutoReplyConversation 按租户 + 主键读取会话（判定是否已由人工接待）。
func FindAutoReplyConversation(tenantID, convID uint) (*model.Conversation, error) {
	var conv model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateAutoReplyConversation 按主键更新会话的指定字段（转人工置 pending / 回写消息摘要）。
// 用 map 更新才能把 agent_id 显式写成 0，保持原行为。
func UpdateAutoReplyConversation(convID uint, fields map[string]interface{}) error {
	return storage.DB().Model(&model.Conversation{}).Where("id = ?", convID).Updates(fields).Error
}

// CreateAutoReplyMessage 落一条机器人发出的消息，成功后回填主键。
func CreateAutoReplyMessage(m *model.Message) error {
	return storage.DB().Create(m).Error
}
