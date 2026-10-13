package service

import (
	"errors"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
)

// ---------------- 客户标签 ----------------
// 客服给客户打标签 → 其他客服接待同一客户时能看到（会话列表、客户信息栏、历史档案均展示）

type TagVO struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Sort      int    `json:"sort"`
	UseCount  int64  `json:"use_count"`
	CreatedAt string `json:"created_at"`
}

type TagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  int    `json:"sort"`
}

func ListTags(tenantID uint) ([]TagVO, error) {
	var list []model.Tag
	if err := gormDB().Where("tenant_id = ?", tenantID).Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]TagVO, 0, len(list))
	for _, t := range list {
		var uc int64
		gormDB().Model(&model.CustomerTag{}).Where("tenant_id = ? AND tag_id = ?", tenantID, t.ID).Count(&uc)
		out = append(out, TagVO{
			ID: t.ID, Name: t.Name, Color: t.Color, Sort: t.Sort,
			UseCount: uc, CreatedAt: fmtTime(t.CreatedAt),
		})
	}
	return out, nil
}

func CreateTag(tenantID uint, req TagRequest) (*TagVO, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("标签名称不能为空")
	}
	var exist model.Tag
	if gormDB().Where("tenant_id = ? AND name = ?", tenantID, name).First(&exist).Error == nil {
		return nil, errors.New("标签已存在")
	}
	color := req.Color
	if color == "" {
		color = "blue"
	}
	t := model.Tag{TenantID: tenantID, Name: name, Color: color, Sort: req.Sort}
	if err := gormDB().Create(&t).Error; err != nil {
		return nil, err
	}
	return &TagVO{ID: t.ID, Name: t.Name, Color: t.Color, Sort: t.Sort, CreatedAt: fmtTime(t.CreatedAt)}, nil
}

func DeleteTag(tenantID, id uint) error {
	gormDB().Where("tenant_id = ? AND tag_id = ?", tenantID, id).Delete(&model.CustomerTag{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Tag{}).Error
}

// GetCustomerTags 读取某客户的标签
func GetCustomerTags(tenantID, customerID uint) []dto.TagRef {
	var rels []model.CustomerTag
	if err := gormDB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).Find(&rels).Error; err != nil {
		return []dto.TagRef{}
	}
	if len(rels) == 0 {
		return []dto.TagRef{}
	}
	ids := make([]uint, 0, len(rels))
	for _, r := range rels {
		ids = append(ids, r.TagID)
	}
	var tags []model.Tag
	gormDB().Where("tenant_id = ? AND id IN ?", tenantID, ids).Order("sort asc").Find(&tags)
	out := make([]dto.TagRef, 0, len(tags))
	for _, t := range tags {
		out = append(out, dto.TagRef{ID: t.ID, Name: t.Name, Color: t.Color})
	}
	return out
}

// SetCustomerTags 全量覆盖客户的标签集合
func SetCustomerTags(tenantID, customerID, operatorID uint, tagIDs []uint) ([]dto.TagRef, error) {
	var c model.Customer
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, customerID).First(&c).Error; err != nil {
		return nil, errors.New("客户不存在")
	}
	valid := map[uint]bool{}
	if len(tagIDs) > 0 {
		var tags []model.Tag
		gormDB().Where("tenant_id = ? AND id IN ?", tenantID, tagIDs).Find(&tags)
		for _, t := range tags {
			valid[t.ID] = true
		}
	}
	var old []model.CustomerTag
	gormDB().Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).Find(&old)
	for _, o := range old {
		if !valid[o.TagID] {
			gormDB().Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, o.TagID).
				Delete(&model.CustomerTag{})
		}
	}
	now := time.Now()
	for _, id := range tagIDs {
		if !valid[id] {
			continue
		}
		var cnt int64
		gormDB().Model(&model.CustomerTag{}).
			Where("tenant_id = ? AND customer_id = ? AND tag_id = ?", tenantID, customerID, id).Count(&cnt)
		if cnt == 0 {
			gormDB().Create(&model.CustomerTag{
				TenantID: tenantID, CustomerID: customerID, TagID: id,
				CreatedBy: operatorID, CreatedAt: now,
			})
		}
	}
	return GetCustomerTags(tenantID, customerID), nil
}

// UpdateCustomerRemark 客服备注（同样跨客服可见）
func UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return gormDB().Model(&model.Customer{}).
		Where("tenant_id = ? AND id = ?", tenantID, customerID).
		Update("remark", remark).Error
}
