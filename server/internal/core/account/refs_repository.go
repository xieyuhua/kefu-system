package account

import (
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// CountOpenConversations 统计某客服名下处于 open 状态的会话数（并发接待上限校验）。
// 只统计租户内的会话，防止跨租户串号。
func (r *Repository) CountOpenConversations(tenantID, agentID uint) (int64, error) {
	var cnt int64
	err := storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, "open").
		Count(&cnt).Error
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

// ---------------- 跨表只读：客服可见渠道 ----------------

// listChannelsByTenant 列出租户的渠道（按 id 升序），只读 channels 表、不写。
// isAdmin 为 false 时只返回该客服已被分配的渠道（agent_channels 关联）。
// 之所以留在 account 模块：账号列表与登录响应都要回显这份数据，
// 而 account 只允许依赖 platform，不能 import channel 模块。
func (r *Repository) listChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]*model.Channel, error) {
	var list []*model.Channel
	q := storage.DB().Where("tenant_id = ?", tenantID)
	if !isAdmin {
		q = q.Where("id IN (SELECT channel_id FROM agent_channels WHERE user_id = ?)", agentID)
	}
	if err := q.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
