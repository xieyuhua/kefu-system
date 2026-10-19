package service

import (
	"kefu-server/internal/common/dto"
	"kefu-server/internal/common/model"
	"kefu-server/pkg/response"
	"kefu-server/pkg/storage"
	"kefu-server/pkg/util"
)

// ---------------- Order ----------------

func (s *CustomerService) ListOrders(tenantID uint, customerID uint, keyword string, page, pageSize int) (*response.Paged, error) {
	// 统一夹取到合法区间，并回显夹取后的值（否则 page_size=100000000 会原样出现在响应里）
	page, pageSize = storage.Default().ClampPage(page, pageSize)
	list, total, err := s.orders.ListOrdersPaged(tenantID, customerID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, o := range list {
		vos = append(vos, dto.OrderVO{
			ID: o.ID, OrderNo: o.OrderNo, Title: o.Title, Status: o.Status,
			Amount: o.Amount, Quantity: o.Quantity, Image: o.Image, SKU: o.SKU,
			CreatedAt: util.Default().FmtTime(o.CreatedAt), PaidAt: util.Default().FmtTimePtr(o.PaidAt),
		})
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

// CountOrdersOfCustomer 统计客户名下订单数（跨包只读门面）。
func (s *CustomerService) CountOrdersOfCustomer(customerID uint) (int64, error) {
	return s.orders.CountOrdersOfCustomer(customerID)
}

// FindOrderByNo 按租户 + 订单号取订单（跨包只读门面）。
func (s *CustomerService) FindOrderByNo(tenantID uint, orderNo string) (*model.Order, error) {
	return s.orders.FindOrderByNo(tenantID, orderNo)
}
