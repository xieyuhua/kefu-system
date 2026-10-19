package repository

import (
	"strings"

	"kefu-server/internal/model"
)

// ---------------- 订单（Order）数据访问 ----------------

// FindOrderByNo 按租户 + 订单号查订单（用户端 ?order_no= 生成聊天订单卡片）。
// 未命中返回 gorm.ErrRecordNotFound。
func FindOrderByNo(tenantID uint, orderNo string) (*model.Order, error) {
	var o model.Order
	if err := db.Where("tenant_id = ? AND order_no = ?", tenantID, orderNo).First(&o).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrdersPaged 分页查询订单列表，按创建时间倒序。
// customerID > 0 时只查该客户的订单；keyword 同时匹配订单号 / 标题。
func ListOrdersPaged(tenantID, customerID uint, keyword string, page, pageSize int) ([]*model.Order, int64, error) {
	q := db.Model(&model.Order{}).Where("tenant_id = ?", tenantID)
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
	offset, limit := Paginate(page, pageSize)
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountOrdersOfCustomer 统计客户名下的订单总数（客户列表 / 客户详情的统计口径）。
func CountOrdersOfCustomer(customerID uint) (int64, error) {
	var cnt int64
	if err := db.Model(&model.Order{}).Where("customer_id = ?", customerID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// ListOrdersOfCustomer 取客户最近的若干条订单（历史档案），按创建时间倒序。
func ListOrdersOfCustomer(customerID uint, limit int) ([]*model.Order, error) {
	var list []*model.Order
	if err := db.Where("customer_id = ?", customerID).
		Order("created_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
