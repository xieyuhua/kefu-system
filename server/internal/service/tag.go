package service

import (
	"errors"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
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
	list, err := repository.ListTagsByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]TagVO, 0, len(list))
	for _, t := range list {
		// 使用次数统计失败按 0 处理，不阻断列表展示
		uc, _ := repository.CountTagUsage(tenantID, t.ID)
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
	if _, err := repository.FindTagByName(tenantID, name); err == nil {
		return nil, errors.New("标签已存在")
	}
	color := req.Color
	if color == "" {
		color = "blue"
	}
	t := model.Tag{TenantID: tenantID, Name: name, Color: color, Sort: req.Sort}
	if err := repository.CreateTag(&t); err != nil {
		return nil, err
	}
	return &TagVO{ID: t.ID, Name: t.Name, Color: t.Color, Sort: t.Sort, CreatedAt: fmtTime(t.CreatedAt)}, nil
}

func DeleteTag(tenantID, id uint) error {
	return repository.DeleteTagWithUsages(tenantID, id)
}

// GetCustomerTags 读取某客户的标签
func GetCustomerTags(tenantID, customerID uint) []dto.TagRef {
	rels, err := repository.ListCustomerTags(tenantID, customerID)
	if err != nil {
		return []dto.TagRef{}
	}
	if len(rels) == 0 {
		return []dto.TagRef{}
	}
	ids := make([]uint, 0, len(rels))
	for _, r := range rels {
		ids = append(ids, r.TagID)
	}
	tags, _ := repository.ListTagsByIDs(tenantID, ids)
	out := make([]dto.TagRef, 0, len(tags))
	for _, t := range tags {
		out = append(out, dto.TagRef{ID: t.ID, Name: t.Name, Color: t.Color})
	}
	return out
}

// SetCustomerTags 全量覆盖客户的标签集合
func SetCustomerTags(tenantID, customerID, operatorID uint, tagIDs []uint) ([]dto.TagRef, error) {
	if _, err := repository.FindCustomerInTenant(tenantID, customerID); err != nil {
		return nil, errors.New("客户不存在")
	}
	// 只接受本租户真实存在的标签，非法 ID 一律忽略（与原逻辑一致）
	valid := map[uint]bool{}
	if len(tagIDs) > 0 {
		tags, _ := repository.ListTagsByIDs(tenantID, tagIDs)
		for _, t := range tags {
			valid[t.ID] = true
		}
	}
	// 先删掉不再需要的旧关联
	old, _ := repository.ListCustomerTags(tenantID, customerID)
	for _, o := range old {
		if !valid[o.TagID] {
			_ = repository.DeleteCustomerTag(tenantID, customerID, o.TagID)
		}
	}
	// 再补齐新增的关联（已存在则跳过）
	now := time.Now()
	for _, id := range tagIDs {
		if !valid[id] {
			continue
		}
		cnt, _ := repository.CountCustomerTag(tenantID, customerID, id)
		if cnt == 0 {
			_ = repository.CreateCustomerTag(&model.CustomerTag{
				TenantID: tenantID, CustomerID: customerID, TagID: id,
				CreatedBy: operatorID, CreatedAt: now,
			})
		}
	}
	return GetCustomerTags(tenantID, customerID), nil
}

// UpdateCustomerRemark 客服备注（同样跨客服可见）
func UpdateCustomerRemark(tenantID, customerID uint, remark string) error {
	return repository.UpdateCustomerRemark(tenantID, customerID, remark)
}
