package service

import (
	"kefu-server/internal/common/model"
	"kefu-server/internal/platform/dto"
	"kefu-server/pkg/response"
	"kefu-server/pkg/storage"
)

// ---------------- Product ----------------

func (s *CustomerService) ListProducts(tenantID uint, keyword string, page, pageSize int) (*response.Paged, error) {
	// 统一夹取到合法区间，并回显夹取后的值（否则 page_size=100000000 会原样出现在响应里）
	page, pageSize = storage.ClampPage(page, pageSize)
	list, total, err := s.products.ListProductsPaged(tenantID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, p := range list {
		vos = append(vos, dto.ProductVO{
			ID: p.PID, Title: p.Title, Price: p.Price, Image: p.Image,
			Stock: p.Stock, SKU: p.SKU, URL: p.URL,
		})
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

// FindProductByPID 按租户 + 对外商品 ID 取商品（跨包只读门面）。
func (s *CustomerService) FindProductByPID(tenantID uint, pid string) (*model.Product, error) {
	return s.products.FindProductByPID(tenantID, pid)
}
