package repository

import (
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// OrderRepository 订单（orders）的数据访问层。
type OrderRepository struct{}

// NewOrderRepository 创建数据访问层实例。
func NewOrderRepository() *OrderRepository { return &OrderRepository{} }

// ---------------- 订单（Order）数据访问 ----------------

// FindOrderByNo 按租户 + 订单号查订单（用户端 ?order_no= 生成聊天订单卡片）。
// 未命中返回 gorm.ErrRecordNotFound。
func (r *OrderRepository) FindOrderByNo(tenantID uint, orderNo string) (*model.Order, error) {
	var o model.Order
	if err := storage.Default().DB().Where("tenant_id = ? AND order_no = ?", tenantID, orderNo).First(&o).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrdersPaged 分页查询订单列表，按创建时间倒序。
// customerID > 0 时只查该客户的订单；keyword 同时匹配订单号 / 标题。
func (r *OrderRepository) ListOrdersPaged(tenantID, customerID uint, keyword string, page, pageSize int) ([]*model.Order, int64, error) {
	q := storage.Default().DB().Model(&model.Order{}).Where("tenant_id = ?", tenantID)
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
	offset, limit := storage.Default().Paginate(page, pageSize)
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountOrdersOfCustomer 统计客户名下的订单总数（客户列表 / 客户详情的统计口径）。
func (r *OrderRepository) CountOrdersOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := storage.Default().DB().Model(&model.Order{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// ListOrdersOfCustomer 取客户最近的若干条订单（历史档案），按创建时间倒序。
func (r *OrderRepository) ListOrdersOfCustomer(customerID uint, limit int) ([]*model.Order, error) {
	var list []*model.Order
	if err := storage.Default().DB().Where("customer_id = ?", customerID).
		Order("created_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
