package repository

// 本文件是 common/repository 的数据访问层。
//
// 说明：account 是全局唯一字段，因此「按账号查」与「账号查重」不带租户条件；
// 其余按主键的操作一律带 tenant_id，保证越权访问落到「不存在」分支上。
//
// 跨表只读：账号列表 / 登录响应需要回显「该客服可接待的渠道」，
// 而 common/repository 不依赖 common/repository（避免内核层出现环），因此这里保留一条对 channels 表的
// 只读查询（UserListChannelsByTenant，不写 channels 表），SQL 与 common/repository 完全一致。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段：本包的 init 会向 platform/authctx 注册鉴权回调，
// 而连接是 main 启动后才注入 pkg/storage 的，因此每个方法现取 storage.Default().DB()。
import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// Repository 账号内核的数据访问层（无状态，可随时构造）。
type UserRepository struct{}

// NewRepository 创建数据访问层实例。
func NewUserRepository() *UserRepository { return &UserRepository{} }

// ---------------- 后台账号（User）数据访问 ----------------

// FindUserByAccount 按账号查询用户（登录 / 账号查重场景）。
// 未命中返回 gorm.ErrRecordNotFound —— 这是正常分支，调用方据此走「账号不存在」逻辑，
// 不要把它当成系统错误。
func (r *UserRepository) FindUserByAccount(account string) (*model.User, error) {
	var u model.User
	if err := storage.Default().DB().Where("account = ?", account).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FindUserByID 按租户 + 主键查询账号；未命中返回 gorm.ErrRecordNotFound。
// 跨租户的 id 同样命中不到，避免后台接口越权读到别家账号。
func (r *UserRepository) FindUserByID(tenantID, id uint) (*model.User, error) {
	var u model.User
	if err := storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsersByTenant 列出租户下全部后台账号（含 admin，按 id 升序）。
// 注意与 channel 模块的 ListTenantAgents 的区别：后者只返回 role = 'agent' 的子账号，供渠道分配使用。
func (r *UserRepository) ListUsersByTenant(tenantID uint) ([]*model.User, error) {
	var users []*model.User
	if err := storage.Default().DB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CreateUser 新增账号，成功后回填主键与时间戳。
func (r *UserRepository) CreateUser(u *model.User) error {
	return storage.Default().DB().Create(u).Error
}

// SaveUser 全字段保存账号（昵称、角色、并发上限、启用状态、密码哈希等）。
func (r *UserRepository) SaveUser(u *model.User) error {
	return storage.Default().DB().Save(u).Error
}

// DeleteUserWithBindings 在同一事务内删除账号并清理其渠道分配（agent_channels）。
// 两张表要么都删成功，要么都不动，避免残留孤儿分配记录。
func (r *UserRepository) DeleteUserWithBindings(tenantID, id uint) error {
	return storage.Default().Tx(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.AgentChannel{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.User{}).Error
	})
}

// CountUsersByAccount 统计占用该账号的用户数（account 全局唯一）。
// excludeID > 0 时排除该主键，供更新场景「排除自身」查重。
func (r *UserRepository) CountUsersByAccount(account string, excludeID uint) (int64, error) {
	var cnt int64
	q := storage.Default().DB().Model(&model.User{}).Where("account = ?", account)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountUsersByTenant 统计租户下的账号总数（含 admin）。
func (r *UserRepository) CountUsersByTenant(tenantID uint) (int64, error) {
	var cnt int64
	if err := storage.Default().DB().Model(&model.User{}).Where("tenant_id = ?", tenantID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CountUsers 统计全平台账号总数。
func (r *UserRepository) CountUsers() (int64, error) {
	var cnt int64
	if err := storage.Default().DB().Model(&model.User{}).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// UpdateUserOnline 更新账号在线状态（登录 / 登出 / WS 连接状态变更）。
func (r *UserRepository) UpdateUserOnline(tenantID, id uint, online bool) error {
	return storage.Default().DB().Model(&model.User{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("online", online).Error
}

// UpdateUserOnlineByID 只按主键更新在线状态（WebSocket 层只持有 userID 的场景）。
func (r *UserRepository) UpdateUserOnlineByID(id uint, online bool) error {
	return storage.Default().DB().Model(&model.User{}).Where("id = ?", id).Update("online", online).Error
}

// ResetAllOnline 把全平台账号置为离线。进程重启后没有任何 WS 连接，启动时调用一次。
func (r *UserRepository) ResetAllOnline() error {
	return storage.Default().DB().Model(&model.User{}).Where("online = ?", true).Update("online", false).Error
}

// UpdateUserPassword 只更新密码哈希，不触碰其它字段（避免整行覆盖）。
func (r *UserRepository) UpdateUserPassword(id uint, passwordHash string) error {
	return storage.Default().DB().Model(&model.User{}).Where("id = ?", id).
		Update("password_hash", passwordHash).Error
}

// UpdateUserProfile 只更新个人资料：昵称必更，头像仅在非空时更新（空值表示不修改）。
func (r *UserRepository) UpdateUserProfile(id uint, name, avatar string) error {
	updates := map[string]interface{}{"name": name}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	return storage.Default().DB().Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteAgentChannelsByUser 清空某账号的全部渠道分配（按账号批量设置渠道时先删后建）。
// 与 DeleteAgentChannels（按渠道维度清空）方向相反，互不替代。
func (r *UserRepository) DeleteAgentChannelsByUser(userID uint) error {
	return storage.Default().DB().Where("user_id = ?", userID).Delete(&model.AgentChannel{}).Error
}

// ListAgentChannelIDsByUser 取某账号被分配（可接待）的渠道 ID 列表。
// 登录响应与个人资料里的 channels 字段用它拼装。
func (r *UserRepository) ListAgentChannelIDsByUser(userID uint) ([]uint, error) {
	var acs []*model.AgentChannel
	if err := storage.Default().DB().Where("user_id = ?", userID).Find(&acs).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(acs))
	for _, ac := range acs {
		ids = append(ids, ac.ChannelID)
	}
	return ids, nil
}

// UserCreateAgentChannel 给某账号分配一个渠道（写 agent_channels）。
// 这张分配表由 account 与 channel 两个模块共同维护：本方法是「账号维度」的写入，
// channel 模块的 CreateAgentChannel 是「渠道维度」的写入，方向不同、互不替代。
func (r *UserRepository) UserCreateAgentChannel(ac *model.AgentChannel) error {
	return storage.Default().DB().Create(ac).Error
}

// FindUserBriefByID 按主键取账号的接待相关字段（昵称 / 角色 / 头像 / 在线 / 最大同时接待数）。
// 会话与消息的展示场景已确定归属，故不加租户条件；未命中返回 gorm.ErrRecordNotFound。
func (r *UserRepository) FindUserBriefByID(userID uint) (*model.User, error) {
	var u model.User
	if err := storage.Default().DB().Select("id", "name", "role", "avatar", "online", "max_concurrent").
		Where("id = ?", userID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ---------------- 以下为合入本类型的只读方法 ----------------
// 拆分前 account 内核只有一个 Repository 类型，只是按职责分成三个文件书写；
// 合并进 common/repository 一个包后统一由 UserRepository 承载，SQL 逐字未改。

// CountOpenConversations 统计某客服名下处于 open 状态的会话数（并发接待上限校验）。
// 只统计租户内的会话，防止跨租户串号。
func (r *UserRepository) CountOpenConversations(tenantID, agentID uint) (int64, error) {
	var cnt int64
	err := storage.Default().DB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND agent_id = ? AND status = ?", tenantID, agentID, "open").
		Count(&cnt).Error
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

// ---------------- 跨表只读：客服可见渠道 ----------------

// UserListChannelsByTenant 列出租户的渠道（按 id 升序），只读 channels 表、不写。
// isAdmin 为 false 时只返回该客服已被分配的渠道（agent_channels 关联）。
// 之所以留在账号域：账号列表与登录响应都要回显这份数据，跨域调用会让账号域反向依赖渠道域。
// 命名说明：合并进一个包后，ChannelRepository 里有同名同签名的 ListChannelsByTenant
// （同一条只读 SQL 的第二份副本）；为免同包内出现两个同名方法，本方法按账号域加 User 前缀，
// 只改名字、SQL 与参数逐字未动。
func (r *UserRepository) UserListChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]*model.Channel, error) {
	var list []*model.Channel
	q := storage.Default().DB().Where("tenant_id = ?", tenantID)
	if !isAdmin {
		q = q.Where("id IN (SELECT channel_id FROM agent_channels WHERE user_id = ?)", agentID)
	}
	if err := q.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ---------------- 租户（Tenant） ----------------

// FindTenantByID 按主键取租户（登录/个人资料里拼租户名与 Logo）。
func (r *UserRepository) FindTenantByID(id uint) (*model.Tenant, error) {
	var t model.Tenant
	if err := storage.Default().DB().First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
