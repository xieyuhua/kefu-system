package repository

// 本文件是 common/repository 的数据访问层。
//
// 写表范围：customers / tags / customer_tags / orders / products。
// 跨表只读：客户列表与详情要回显「所属渠道 / 接待客服 / 历史会话」，
// 而 common/repository 不依赖 common/repository、common/repository、common/repository（避免内核层出现环），
// 因此在这里保留几条对 channels / users / conversations 的只读查询（不写这些表），
// SQL 与对应内核模块完全一致。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）。
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。
import (
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// Repository 客户内核的数据访问层（无状态，可随时构造）。
type CustomerRepository struct{}

// NewRepository 创建数据访问层实例。
func NewCustomerRepository() *CustomerRepository { return &CustomerRepository{} }

// ---------------- 客户（Customer）数据访问 ----------------

// FindCustomerInTenant 按租户 + 主键查询客户；未命中返回 gorm.ErrRecordNotFound。
// 租户条件保证后台任何入口都拿不到别人家的客户。
func (r *CustomerRepository) FindCustomerInTenant(tenantID, id uint) (*model.Customer, error) {
	var c model.Customer
	if err := storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByChannelExternalID 按「租户 + 渠道 + 渠道侧用户标识」查客户。
// 这是用户端（访客）识人的统一口径：同一渠道下同一个 uid 视为同一个客户。
func (r *CustomerRepository) FindCustomerByChannelExternalID(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := storage.Default().DB().Where("tenant_id = ? AND channel_id = ? AND external_id = ?",
		tenantID, channelID, externalID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByExternalID 按「租户 + 渠道侧用户标识」查客户，不限渠道。
// 用于用户端无 WebSocket 时的轮询降级接口（只带 uid，不带 channel）。
func (r *CustomerRepository) FindCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := storage.Default().DB().Where("tenant_id = ? AND external_id = ?", tenantID, externalID).
		First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCustomersPaged 分页查询客户列表，按最近活跃时间倒序。
// keyword 同时匹配昵称 / 手机号 / 外部 ID；channelID > 0 时只查该渠道。
func (r *CustomerRepository) ListCustomersPaged(tenantID uint, keyword string, channelID uint, page, pageSize int) ([]*model.Customer, int64, error) {
	q := storage.Default().DB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID)
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
	offset, limit := storage.Default().Paginate(page, pageSize)
	if err := q.Order("last_seen_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CreateCustomer 新增客户，成功后回填主键与时间戳。
func (r *CustomerRepository) CreateCustomer(c *model.Customer) error {
	return storage.Default().DB().Create(c).Error
}

// UpdateCustomerColumns 按主键更新客户的部分字段（昵称、最近活跃时间等）。
func (r *CustomerRepository) UpdateCustomerColumns(id uint, updates map[string]interface{}) error {
	return storage.Default().DB().Model(&model.Customer{}).Where("id = ?", id).Updates(updates).Error
}

// SaveCustomerRemark 更新客服备注（跨客服可见）。
func (r *CustomerRepository) SaveCustomerRemark(tenantID, customerID uint, remark string) error {
	return storage.Default().DB().Model(&model.Customer{}).
		Where("tenant_id = ? AND id = ?", tenantID, customerID).
		Update("remark", remark).Error
}

// ---------------- 客户相关的会话只读查询 ----------------
// 会话的定位 / 新建 / 全量读写由 common/repository 负责（FindLatestConversationByCustomer、
// CreateConversation ...），这里只保留客户维度独有的口径（只读 conversations 表）。

// FindActiveConversationOfCustomer 取客户最近一个未结束（status != closed）且未归档的会话。
// 一处未命中即返回 gorm.ErrRecordNotFound，由调用方决定是否新建会话。
func (r *CustomerRepository) FindActiveConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.Default().DB().Where("tenant_id = ? AND customer_id = ? AND status != 'closed' AND archived = ?",
		tenantID, customerID, false).
		Order("id desc").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// CountConversationsOfCustomer 统计客户名下的会话总数（客户列表的统计口径）。
func (r *CustomerRepository) CountConversationsOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := storage.Default().DB().Model(&model.Conversation{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// ListCustomerConversations 按客户列出历史会话（新的在前）；limit <= 0 时取 50 条。
// 只读 conversations 表，供客户详情拼装历史档案用。
func (r *CustomerRepository) ListCustomerConversations(tenantID, customerID uint, limit int) ([]*model.Conversation, error) {
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

// FindCustomerByID 按主键读取客户（不加租户条件）。
// 用于会话/消息组装 VO：调用方已按租户确认过会话归属，此处只需取客户展示字段。
func (r *CustomerRepository) FindCustomerByID(customerID uint) (*model.Customer, error) {
	var cu model.Customer
	if err := storage.Default().DB().First(&cu, customerID).Error; err != nil {
		return nil, err
	}
	return &cu, nil
}

// ListCustomersByIDs 按主键批量取客户（会话列表一次性预取，替代逐条查询）。
func (r *CustomerRepository) ListCustomersByIDs(ids []uint) ([]*model.Customer, error) {
	if len(ids) == 0 {
		return []*model.Customer{}, nil
	}
	var list []*model.Customer
	if err := storage.Default().DB().Where("id IN ?", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
