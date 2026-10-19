package repository

import (
	"kefu-server/internal/model"

	"gorm.io/gorm"
)

// ---------------- 渠道（Channel）数据访问 ----------------

// FindChannelByID 按租户 + 主键查询渠道；未命中返回 gorm.ErrRecordNotFound。
func FindChannelByID(tenantID, id uint) (*model.Channel, error) {
	var c model.Channel
	if err := db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListChannelsByTenant 列出租户的渠道（按 id 升序）。
// isAdmin 为 false 时只返回该客服已被分配的渠道（agent_channels 关联）。
func ListChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]model.Channel, error) {
	var list []model.Channel
	q := db.Where("tenant_id = ?", tenantID)
	if !isAdmin {
		q = q.Where("id IN (SELECT channel_id FROM agent_channels WHERE user_id = ?)", agentID)
	}
	if err := q.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountAgentsOfChannel 统计某渠道已分配的接待账号数。
func CountAgentsOfChannel(channelID uint) (int64, error) {
	var cnt int64
	if err := db.Model(&model.AgentChannel{}).Where("channel_id = ?", channelID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountPendingByChannel 统计某渠道下状态为 pending 的会话数（渠道列表的待接待角标）。
func CountPendingByChannel(tenantID, channelID uint) (int64, error) {
	var cnt int64
	err := db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND status = 'pending'", tenantID, channelID).
		Count(&cnt).Error
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountChannelByAppID 统计同租户内占用该 app_id 的渠道数；excludeID 用于更新场景排除自身。
func CountChannelByAppID(tenantID uint, appID string, excludeID uint) (int64, error) {
	var cnt int64
	q := db.Model(&model.Channel{}).
		Where("tenant_id = ? AND app_id = ?", tenantID, appID)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CreateChannel 新增渠道，成功后回填主键与时间戳。
func CreateChannel(c *model.Channel) error {
	return db.Create(c).Error
}

// SaveChannel 保存渠道的全字段（含启用状态与用户端展示开关）。
func SaveChannel(c *model.Channel) error {
	return db.Save(c).Error
}

// DeleteChannelWithBindings 在同一事务内删除渠道并清理其客服分配关系（agent_channels）。
func DeleteChannelWithBindings(tenantID, id uint) error {
	return Tx(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&model.AgentChannel{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Channel{}).Error
	})
}

// ---------------- 渠道 ↔ 接待账号 ----------------

// ListAgentChannelsByChannel 列出某渠道已分配的账号关联，用于回显勾选状态。
func ListAgentChannelsByChannel(channelID uint) ([]model.AgentChannel, error) {
	var acs []model.AgentChannel
	if err := db.Where("channel_id = ?", channelID).Find(&acs).Error; err != nil {
		return nil, err
	}
	return acs, nil
}

// ListTenantAgents 列出租户下所有客服子账号（role = agent，按 id 升序）。
func ListTenantAgents(tenantID uint) ([]model.User, error) {
	var users []model.User
	if err := db.Where("tenant_id = ? AND role = 'agent'", tenantID).
		Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindAgentUsersByIDs 按主键批量取租户内的客服账号，用于校验账号归属本租户。
func FindAgentUsersByIDs(tenantID uint, ids []uint) ([]model.User, error) {
	var users []model.User
	if err := db.Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// DeleteAgentChannels 清空某渠道的全部接待账号分配（批量设置时先删后建）。
func DeleteAgentChannels(channelID uint) error {
	return db.Where("channel_id = ?", channelID).Delete(&model.AgentChannel{}).Error
}

// CreateAgentChannel 给某渠道分配一个接待账号。
func CreateAgentChannel(ac *model.AgentChannel) error {
	return db.Create(ac).Error
}

// HasAgentChannel 判断客服是否配置了接待该渠道（接管 / 转接的准入校验）。
func HasAgentChannel(userID, channelID uint) (bool, error) {
	var n int64
	if err := db.Model(&model.AgentChannel{}).
		Where("user_id = ? AND channel_id = ?", userID, channelID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
