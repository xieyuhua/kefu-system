package repository

import (
	"strings"

	"kefu-server/internal/model"
)

// ---------------- 客户（Customer）数据访问 ----------------

// FindCustomerInTenant 按租户 + 主键查询客户；未命中返回 gorm.ErrRecordNotFound。
// 租户条件保证后台任何入口都拿不到别人家的客户。
func FindCustomerInTenant(tenantID, id uint) (*model.Customer, error) {
	var c model.Customer
	if err := db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByChannelExternalID 按「租户 + 渠道 + 渠道侧用户标识」查客户。
// 这是用户端（访客）识人的统一口径：同一渠道下同一个 uid 视为同一个客户。
func FindCustomerByChannelExternalID(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := db.Where("tenant_id = ? AND channel_id = ? AND external_id = ?",
		tenantID, channelID, externalID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByExternalID 按「租户 + 渠道侧用户标识」查客户，不限渠道。
// 用于用户端无 WebSocket 时的轮询降级接口（只带 uid，不带 channel）。
func FindCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := db.Where("tenant_id = ? AND external_id = ?", tenantID, externalID).
		First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCustomersPaged 分页查询客户列表，按最近活跃时间倒序。
// keyword 同时匹配昵称 / 手机号 / 外部 ID；channelID > 0 时只查该渠道。
func ListCustomersPaged(tenantID uint, keyword string, channelID uint, page, pageSize int) ([]*model.Customer, int64, error) {
	q := db.Model(&model.Customer{}).Where("tenant_id = ?", tenantID)
	if channelID > 0 {
		q = q.Where("channel_id = ?", channelID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("nickname LIKE ? OR phone LIKE ? OR external_id LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Customer
	offset, limit := Paginate(page, pageSize)
	if err := q.Order("last_seen_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CreateCustomer 新增客户，成功后回填主键与时间戳。
func CreateCustomer(c *model.Customer) error {
	return db.Create(c).Error
}

// UpdateCustomerColumns 按主键更新客户的部分字段（昵称、最近活跃时间等）。
func UpdateCustomerColumns(id uint, updates map[string]interface{}) error {
	return db.Model(&model.Customer{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateCustomerRemark 更新客服备注（跨客服可见）。
func UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return db.Model(&model.Customer{}).
		Where("tenant_id = ? AND id = ?", tenantID, customerID).
		Update("remark", remark).Error
}

// ---------------- 客户相关的会话只读查询 ----------------
// 会话的定位 / 新建 / 全量读写由 conversation 域仓储负责（FindLatestConversationByCustomer、
// CreateConversation、ListConversationsByCustomer ...），这里只保留客户维度独有的口径。

// FindActiveConversationOfCustomer 取客户最近一个未结束（status != closed）且未归档的会话。
// 一处未命中即返回 gorm.ErrRecordNotFound，由调用方决定是否新建会话。
func FindActiveConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := db.Where("tenant_id = ? AND customer_id = ? AND status != 'closed' AND archived = ?",
		tenantID, customerID, false).
		Order("id desc").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// CountConversationsOfCustomer 统计客户名下的会话总数（客户列表的统计口径）。
func CountConversationsOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := db.Model(&model.Conversation{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// ---------------- 客服设备（Device：APP / PC 桌面端） ----------------

// CreateDevice 登记一台推送设备。
func CreateDevice(d *model.Device) error {
	return db.Create(d).Error
}

// ListDevices 列出某客服已登记的推送设备（按 id 倒序）。
func ListDevices(tenantID, userID uint) ([]*model.Device, error) {
	var list []*model.Device
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteDevice 删除某客服自己的一台设备。
func DeleteDevice(tenantID, userID, id uint) error {
	return db.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		Delete(&model.Device{}).Error
}

// ---------------- 离线同步（消息 / 提醒） ----------------

// ListVisibleConversationIDs 列出客服可见范围内的会话 ID：
// 自己接待的 + 自己渠道下尚未分配的（与原离线同步口径一致）。
func ListVisibleConversationIDs(tenantID, userID uint) ([]uint, error) {
	var chIDs []uint
	if err := db.Model(&model.AgentChannel{}).Where("user_id = ?", userID).
		Pluck("channel_id", &chIDs).Error; err != nil {
		return nil, err
	}
	var convIDs []uint
	q := db.Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
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

// ListMessagesOfConversations 取一批会话中 id > afterID 的消息，按 id 升序，最多 limit 条。
// 调用方按「多取一条」判断是否还有更多（has_more）。
func ListMessagesOfConversations(tenantID uint, convIDs []uint, afterID uint, limit int) ([]*model.Message, error) {
	var list []*model.Message
	if err := db.Where("tenant_id = ? AND conversation_id IN ? AND id > ?", tenantID, convIDs, afterID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListNotificationsAfter 取客服在 sinceID 之后新增的系统提醒（最多 limit 条，升序）。
func ListNotificationsAfter(tenantID, userID, sinceID uint, limit int) ([]*model.Notification, error) {
	var list []*model.Notification
	if err := db.Where("tenant_id = ? AND user_id = ? AND id > ?", tenantID, userID, sinceID).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// MarkNotificationsRead 把客服的提醒标记为已读；ids 为空时表示全部已读。
func MarkNotificationsRead(tenantID, userID uint, ids []uint) error {
	q := db.Model(&model.Notification{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("read", true).Error
}

// ---------------- 租户（演示兜底） ----------------

// FirstTenantID 返回第一个租户的 ID。
// 用户端未登录、未显式传 ?tenant= 时用它兜底，便于演示环境直接打开链接。
func FirstTenantID() (uint, error) {
	var t model.Tenant
	if err := db.Order("id asc").First(&t).Error; err != nil {
		return 0, err
	}
	return t.ID, nil
}

// FindCustomerByID 按主键读取客户（不加租户条件）。
// 用于会话/消息组装 VO：调用方已按租户确认过会话归属，此处只需取客户展示字段。
func FindCustomerByID(customerID uint) (*model.Customer, error) {
	var cu model.Customer
	if err := db.First(&cu, customerID).Error; err != nil {
		return nil, err
	}
	return &cu, nil
}
