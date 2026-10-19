package customer

import (
	"strings"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 商品（Product）数据访问 ----------------

// FindProductByPID 按租户 + 对外商品 ID（如 P10086）查商品。
// 未命中返回 gorm.ErrRecordNotFound。
func (r *Repository) FindProductByPID(tenantID uint, pid string) (*model.Product, error) {
	var p model.Product
	if err := storage.DB().Where("tenant_id = ? AND p_id = ?", tenantID, pid).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProductsPaged 分页查询商品列表，按主键升序。
// keyword 同时匹配标题 / 对外商品 ID / SKU。
func (r *Repository) ListProductsPaged(tenantID uint, keyword string, page, pageSize int) ([]*model.Product, int64, error) {
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
