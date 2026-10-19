package customer

// 本文件是 customer 模块的数据访问层
// （原 repository/customer.go 的客户部分 + repository/order.go + product.go + tag.go）。
//
// 写表范围：customers / tags / customer_tags / orders / products。
// 跨表只读：客户列表与详情要回显「所属渠道 / 接待客服 / 历史会话」，
// 而 customer 模块只允许依赖 platform，因此在这里保留几条对 channels / users /
// conversations 的只读查询（不写这些表），SQL 与对应模块完全一致。

import (
	"strings"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// ---------------- 客户（Customer）数据访问 ----------------

// FindCustomerInTenant 按租户 + 主键查询客户；未命中返回 gorm.ErrRecordNotFound。
// 租户条件保证后台任何入口都拿不到别人家的客户。
func FindCustomerInTenant(tenantID, id uint) (*model.Customer, error) {
	var c model.Customer
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByChannelExternalID 按「租户 + 渠道 + 渠道侧用户标识」查客户。
// 这是用户端（访客）识人的统一口径：同一渠道下同一个 uid 视为同一个客户。
func FindCustomerByChannelExternalID(tenantID, channelID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := storage.DB().Where("tenant_id = ? AND channel_id = ? AND external_id = ?",
		tenantID, channelID, externalID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// FindCustomerByExternalID 按「租户 + 渠道侧用户标识」查客户，不限渠道。
// 用于用户端无 WebSocket 时的轮询降级接口（只带 uid，不带 channel）。
func FindCustomerByExternalID(tenantID uint, externalID string) (*model.Customer, error) {
	var c model.Customer
	if err := storage.DB().Where("tenant_id = ? AND external_id = ?", tenantID, externalID).
		First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCustomersPaged 分页查询客户列表，按最近活跃时间倒序。
// keyword 同时匹配昵称 / 手机号 / 外部 ID；channelID > 0 时只查该渠道。
func ListCustomersPaged(tenantID uint, keyword string, channelID uint, page, pageSize int) ([]*model.Customer, int64, error) {
	q := storage.DB().Model(&model.Customer{}).Where("tenant_id = ?", tenantID)
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
	offset, limit := storage.Paginate(page, pageSize)
	if err := q.Order("last_seen_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CreateCustomer 新增客户，成功后回填主键与时间戳。
func CreateCustomer(c *model.Customer) error {
	return storage.DB().Create(c).Error
}

// UpdateCustomerColumns 按主键更新客户的部分字段（昵称、最近活跃时间等）。
func UpdateCustomerColumns(id uint, updates map[string]interface{}) error {
	return storage.DB().Model(&model.Customer{}).Where("id = ?", id).Updates(updates).Error
}

// SaveCustomerRemark 更新客服备注（跨客服可见）。
func SaveCustomerRemark(tenantID, customerID uint, remark string) error {
	return storage.DB().Model(&model.Customer{}).
		Where("tenant_id = ? AND id = ?", tenantID, customerID).
		Update("remark", remark).Error
}

// ---------------- 客户相关的会话只读查询 ----------------
// 会话的定位 / 新建 / 全量读写由 chat 模块负责（FindLatestConversationByCustomer、
// CreateConversation ...），这里只保留客户维度独有的口径（只读 conversations 表）。

// FindActiveConversationOfCustomer 取客户最近一个未结束（status != closed）且未归档的会话。
// 一处未命中即返回 gorm.ErrRecordNotFound，由调用方决定是否新建会话。
func FindActiveConversationOfCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	var c model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ? AND status != 'closed' AND archived = ?",
		tenantID, customerID, false).
		Order("id desc").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// CountConversationsOfCustomer 统计客户名下的会话总数（客户列表的统计口径）。
func CountConversationsOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.Conversation{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// listConversationsByCustomer 按客户列出历史会话（新的在前）；limit <= 0 时取 50 条。
// 只读 conversations 表，供客户详情拼装历史档案用。
func listConversationsByCustomer(tenantID, customerID uint, limit int) ([]*model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []*model.Conversation
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindCustomerByID 按主键读取客户（不加租户条件）。
// 用于会话/消息组装 VO：调用方已按租户确认过会话归属，此处只需取客户展示字段。
func FindCustomerByID(customerID uint) (*model.Customer, error) {
	var cu model.Customer
	if err := storage.DB().First(&cu, customerID).Error; err != nil {
		return nil, err
	}
	return &cu, nil
}

// ---------------- 跨表只读：渠道 / 账号 ----------------

// findChannelByID 按租户 + 主键只读一条渠道（客户列表/详情回显渠道名与类型）。
func findChannelByID(tenantID, id uint) (*model.Channel, error) {
	var c model.Channel
	if err := storage.DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// findUserBriefByID 按主键只读账号的展示字段（历史会话里的接待客服）。
func findUserBriefByID(userID uint) (*model.User, error) {
	var u model.User
	if err := storage.DB().Select("id", "name", "role", "avatar", "online", "max_concurrent").
		Where("id = ?", userID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FirstTenantID 返回第一个租户的 ID。
// 用户端未登录、未显式传 ?tenant= 时用它兜底，便于演示环境直接打开链接。
func FirstTenantID() (uint, error) {
	var t model.Tenant
	if err := storage.DB().Order("id asc").First(&t).Error; err != nil {
		return 0, err
	}
	return t.ID, nil
}

// ---------------- 订单（Order）数据访问 ----------------

// FindOrderByNo 按租户 + 订单号查订单（用户端 ?order_no= 生成聊天订单卡片）。
// 未命中返回 gorm.ErrRecordNotFound。
func FindOrderByNo(tenantID uint, orderNo string) (*model.Order, error) {
	var o model.Order
	if err := storage.DB().Where("tenant_id = ? AND order_no = ?", tenantID, orderNo).First(&o).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrdersPaged 分页查询订单列表，按创建时间倒序。
// customerID > 0 时只查该客户的订单；keyword 同时匹配订单号 / 标题。
func ListOrdersPaged(tenantID, customerID uint, keyword string, page, pageSize int) ([]*model.Order, int64, error) {
	q := storage.DB().Model(&model.Order{}).Where("tenant_id = ?", tenantID)
	if customerID > 0 {
		q = q.Where("customer_id = ?", customerID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("order_no LIKE ? OR title LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Order
	offset, limit := storage.Paginate(page, pageSize)
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountOrdersOfCustomer 统计客户名下的订单总数（客户列表 / 客户详情的统计口径）。
func CountOrdersOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.Order{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// ListOrdersOfCustomer 取客户最近的若干条订单（历史档案），按创建时间倒序。
func ListOrdersOfCustomer(customerID uint, limit int) ([]*model.Order, error) {
	var list []*model.Order
	if err := storage.DB().Where("customer_id = ?", customerID).
		Order("created_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ---------------- 商品（Product）数据访问 ----------------

// FindProductByPID 按租户 + 对外商品 ID（如 P10086）查商品。
// 未命中返回 gorm.ErrRecordNotFound。
func FindProductByPID(tenantID uint, pid string) (*model.Product, error) {
	var p model.Product
	if err := storage.DB().Where("tenant_id = ? AND p_id = ?", tenantID, pid).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProductsPaged 分页查询商品列表，按主键升序。
// keyword 同时匹配标题 / 对外商品 ID / SKU。
func ListProductsPaged(tenantID uint, keyword string, page, pageSize int) ([]*model.Product, int64, error) {
	q := storage.DB().Model(&model.Product{}).Where("tenant_id = ?", tenantID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR p_id LIKE ? OR sku LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Product
	offset, limit := storage.Paginate(page, pageSize)
	if err := q.Order("id asc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ---------------- 客户标签（Tag / CustomerTag）数据访问 ----------------

// ListTagsByTenant 列出租户的标签库（先按 sort 再按 id 升序）。
func ListTagsByTenant(tenantID uint) ([]*model.Tag, error) {
	var list []*model.Tag
	if err := storage.DB().Where("tenant_id = ?", tenantID).Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListTagsByIDs 按主键批量取租户内的标签（按 sort 升序）。
func ListTagsByIDs(tenantID uint, ids []uint) ([]*model.Tag, error) {
	var list []*model.Tag
	if err := storage.DB().Where("tenant_id = ? AND id IN ?", tenantID, ids).Order("sort asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountTagUsage 统计某标签被多少个客户使用（标签列表的使用次数）。
func CountTagUsage(tenantID, tagID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.CustomerTag{}).
		Where("tenant_id = ? AND tag_id = ?", tenantID, tagID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// FindTagByName 按租户 + 名称查标签，用于新建前的重名校验。
func FindTagByName(tenantID uint, name string) (*model.Tag, error) {
	var t model.Tag
	if err := storage.DB().Where("tenant_id = ? AND name = ?", tenantID, name).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// InsertTag 新增标签，成功后回填主键与时间戳。
func InsertTag(t *model.Tag) error {
	return storage.DB().Create(t).Error
}

// DeleteTagWithUsages 在同一事务内删除标签并清理客户关联关系（customer_tags）。
func DeleteTagWithUsages(tenantID, tagID uint) error {
	return storage.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND tag_id = ?", tenantID, tagID).
			Delete(&model.CustomerTag{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, tagID).Delete(&model.Tag{}).Error
	})
}

// ListCustomerTags 列出某客户已打的标签关联。
func ListCustomerTags(tenantID, customerID uint) ([]*model.CustomerTag, error) {
	var list []*model.CustomerTag
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteCustomerTag 解除客户与某个标签的关联。
func DeleteCustomerTag(tenantID, customerID, tagID uint) error {
	return storage.DB().Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, tagID).
		Delete(&model.CustomerTag{}).Error
}

// CountCustomerTag 统计客户与某标签的关联是否已存在（避免重复插入）。
func CountCustomerTag(tenantID, customerID, tagID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.CustomerTag{}).
		Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, tagID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CreateCustomerTag 新增一条客户标签关联（带上操作人与时间，便于追溯）。
func CreateCustomerTag(ct *model.CustomerTag) error {
	return storage.DB().Create(ct).Error
}
