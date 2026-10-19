package repository

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"

	"gorm.io/gorm"
)

// TagRepository 客户标签（tags / customer_tags）的数据访问层。
type TagRepository struct{}

// NewTagRepository 创建数据访问层实例。
func NewTagRepository() *TagRepository { return &TagRepository{} }

// ---------------- 客户标签（Tag / CustomerTag）数据访问 ----------------

// ListTagsByTenant 列出租户的标签库（先按 sort 再按 id 升序）。
func (r *TagRepository) ListTagsByTenant(tenantID uint) ([]*model.Tag, error) {
	var list []*model.Tag
	if err := storage.DB().Where("tenant_id = ?", tenantID).Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListTagsByIDs 按主键批量取租户内的标签（按 sort 升序）。
func (r *TagRepository) ListTagsByIDs(tenantID uint, ids []uint) ([]*model.Tag, error) {
	var list []*model.Tag
	if err := storage.DB().Where("tenant_id = ? AND id IN ?", tenantID, ids).Order("sort asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountTagUsage 统计某标签被多少个客户使用（标签列表的使用次数）。
func (r *TagRepository) CountTagUsage(tenantID, tagID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.CustomerTag{}).
		Where("tenant_id = ? AND tag_id = ?", tenantID, tagID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// FindTagByName 按租户 + 名称查标签，用于新建前的重名校验。
func (r *TagRepository) FindTagByName(tenantID uint, name string) (*model.Tag, error) {
	var t model.Tag
	if err := storage.DB().Where("tenant_id = ? AND name = ?", tenantID, name).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// InsertTag 新增标签，成功后回填主键与时间戳。
func (r *TagRepository) InsertTag(t *model.Tag) error {
	return storage.DB().Create(t).Error
}

// DeleteTagWithUsages 在同一事务内删除标签并清理客户关联关系（customer_tags）。
func (r *TagRepository) DeleteTagWithUsages(tenantID, tagID uint) error {
	return storage.Tx(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND tag_id = ?", tenantID, tagID).
			Delete(&model.CustomerTag{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, tagID).Delete(&model.Tag{}).Error
	})
}

// ListCustomerTags 列出某客户已打的标签关联。
func (r *TagRepository) ListCustomerTags(tenantID, customerID uint) ([]*model.CustomerTag, error) {
	var list []*model.CustomerTag
	if err := storage.DB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteCustomerTag 解除客户与某个标签的关联。
func (r *TagRepository) DeleteCustomerTag(tenantID, customerID, tagID uint) error {
	return storage.DB().Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, tagID).
		Delete(&model.CustomerTag{}).Error
}

// CountCustomerTag 统计客户与某标签的关联是否已存在（避免重复插入）。
func (r *TagRepository) CountCustomerTag(tenantID, customerID, tagID uint) (int64, error) {
	var cnt int64
	if err := storage.DB().Model(&model.CustomerTag{}).
		Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, tagID).
		Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

// CreateCustomerTag 新增一条客户标签关联（带上操作人与时间，便于追溯）。
func (r *TagRepository) CreateCustomerTag(ct *model.CustomerTag) error {
	return storage.DB().Create(ct).Error
}
