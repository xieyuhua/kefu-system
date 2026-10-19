package repository

// 本文件是 common/repository 的数据访问层。
//
// 写表范围：channels / channel_types / agent_channels（渠道 ↔ 接待账号分配关系）。
// 其中 agent_channels 与 common/repository 共用：本模块只做「渠道维度」的清空与重建，
// 账号维度的分配由 common/repository 负责。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。
import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// Repository 渠道内核的数据访问层（无状态，可随时构造）。
type ChannelRepository struct{}

// NewRepository 创建数据访问层实例。
func NewChannelRepository() *ChannelRepository { return &ChannelRepository{} }

// ---------------- 渠道（Channel）数据访问 ----------------

// FindChannelByID 按租户 + 主键查询渠道；未命中返回 gorm.ErrRecordNotFound。
func (r *ChannelRepository) FindChannelByID(tenantID, id uint) (*model.Channel, error) {
	var c model.Channel
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListChannelsByTenant 列出租户的渠道（按 id 升序）。
// isAdmin 为 false 时只返回该客服已被分配的渠道（agent_channels 关联）。
func (r *ChannelRepository) ListChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]*model.Channel, error) {
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

// CountAgentsOfChannel 统计某渠道已分配的接待账号数。
func (r *ChannelRepository) CountAgentsOfChannel(channelID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.AgentChannel{}).Where("channel_id = ?", channelID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountPendingByChannel 统计某渠道下状态为 pending 的会话数（渠道列表的待接待角标）。
func (r *ChannelRepository) CountPendingByChannel(tenantID, channelID uint) (int64, error) {
	var cnt int64
	err := storage.DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND channel_id = ? AND status = 'pending'", tenantID, channelID).
		Count(&cnt).Error
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountChannelByAppID 统计同租户内占用该 app_id 的渠道数；excludeID 用于更新场景排除自身。
func (r *ChannelRepository) CountChannelByAppID(tenantID uint, appID string, excludeID uint) (int64, error) {
	var cnt int64
	q := storage.DB().Model(&model.Channel{}).
		Where("tenant_id = ? AND app_id = ?", tenantID, appID)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// InsertChannel 新增渠道，成功后回填主键与时间戳。
func (r *ChannelRepository) InsertChannel(c *model.Channel) error {
	return storage.DB().Create(c).Error
}

// SaveChannel 保存渠道的全字段（含启用状态与用户端展示开关）。
func (r *ChannelRepository) SaveChannel(c *model.Channel) error {
	return storage.DB().Save(c).Error
}

// DeleteChannelWithBindings 在同一事务内删除渠道并清理其客服分配关系（agent_channels）。
func (r *ChannelRepository) DeleteChannelWithBindings(tenantID, id uint) error {
	return storage.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&model.AgentChannel{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Channel{}).Error
	})
}

// ---------------- 渠道 ↔ 接待账号 ----------------

// ListAgentChannelsByChannel 列出某渠道已分配的账号关联，用于回显勾选状态。
func (r *ChannelRepository) ListAgentChannelsByChannel(channelID uint) ([]*model.AgentChannel, error) {
	var acs []*model.AgentChannel
	if err := storage.DB().Where("channel_id = ?", channelID).Find(&acs).Error; err != nil {
		return nil, err
	}
	return acs, nil
}

// DeleteAgentChannels 清空某渠道的全部接待账号分配（批量设置时先删后建）。
func (r *ChannelRepository) DeleteAgentChannels(channelID uint) error {
	return storage.DB().Where("channel_id = ?", channelID).Delete(&model.AgentChannel{}).Error
}

// CreateAgentChannel 给某渠道分配一个接待账号。
func (r *ChannelRepository) CreateAgentChannel(ac *model.AgentChannel) error {
	return storage.DB().Create(ac).Error
}

// HasAgentChannel 判断客服是否配置了接待该渠道（接管 / 转接的准入校验）。
func (r *ChannelRepository) HasAgentChannel(userID, channelID uint) (bool, error) {
	var n int64
	if err := storage.DB().Model(&model.AgentChannel{}).
		Where("user_id = ? AND channel_id = ?", userID, channelID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// ---------------- 渠道接待账号（跨表只读） ----------------
// 拆分前这两条查询与渠道表同属 channel 内核的同一个 Repository 类型，只是另起文件书写；
// 合并进 common/repository 一个包后统一由 ChannelRepository 承载，SQL 逐字未改。

// ListTenantAgents 列出租户下所有客服子账号（role = agent，按 id 升序）。
func (r *ChannelRepository) ListTenantAgents(tenantID uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ? AND role = 'agent'", tenantID).
		Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindAgentUsersByIDs 按主键批量取租户内的客服账号，用于校验账号归属本租户。
func (r *ChannelRepository) FindAgentUsersByIDs(tenantID uint, ids []uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.DB().Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}
