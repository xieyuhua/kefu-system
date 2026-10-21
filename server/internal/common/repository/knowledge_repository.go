package repository

// 本文件是 common/repository 数据访问层中「知识库（AI / RAG 资料来源）」域。
//
// 写表范围：knowledge_docs（本内核自己的表）。
// 读表范围：只读 knowledge_docs 一张表，不做跨表 JOIN。
//
// 形态约定：所有数据访问都是 KnowledgeRepository 的方法（指针接收者）；
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// KnowledgeRepository 知识库条目的数据访问层（无状态，可随时构造）。
type KnowledgeRepository struct{}

// NewKnowledgeRepository 创建数据访问层实例。
func NewKnowledgeRepository() *KnowledgeRepository { return &KnowledgeRepository{} }

// knowledgeKeywordWhere 给标题 / 正文 / 标签追加模糊匹配条件（keyword 为空时不加条件）。
func knowledgeKeywordWhere(keyword string) (string, []interface{}) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return "", nil
	}
	like := "%" + kw + "%"
	return " (title LIKE ? OR content LIKE ? OR tags LIKE ?)", []interface{}{like, like, like}
}

// ListKnowledgePaged 分页查询租户的知识库条目：keyword 同时匹配标题 / 正文 / 标签。
// 排序：更新时间倒序（最近维护的在前），同时间按 id 倒序，保证翻页稳定。
func (r *KnowledgeRepository) ListKnowledgePaged(tenantID uint, keyword string, page, pageSize int) ([]*model.KnowledgeDoc, int64, error) {
	q := storage.Default().DB().Model(&model.KnowledgeDoc{}).Where("tenant_id = ?", tenantID)
	if cond, args := knowledgeKeywordWhere(keyword); cond != "" {
		q = q.Where(cond, args...)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.KnowledgeDoc
	offset, limit := storage.Default().Paginate(page, pageSize)
	if err := q.Order("updated_at desc, id desc").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// FindKnowledgeByID 按租户 + 主键查询知识条目；未命中返回 gorm.ErrRecordNotFound。
func (r *KnowledgeRepository) FindKnowledgeByID(tenantID, id uint) (*model.KnowledgeDoc, error) {
	var doc model.KnowledgeDoc
	if err := storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

// InsertKnowledge 新增知识条目，成功后回填主键与时间戳。
func (r *KnowledgeRepository) InsertKnowledge(doc *model.KnowledgeDoc) error {
	return storage.Default().DB().Create(doc).Error
}

// SaveKnowledge 保存知识条目全字段（更新路径）。
func (r *KnowledgeRepository) SaveKnowledge(doc *model.KnowledgeDoc) error {
	return storage.Default().DB().Save(doc).Error
}

// RemoveKnowledge 按租户 + 主键删除知识条目。
func (r *KnowledgeRepository) RemoveKnowledge(tenantID, id uint) error {
	return storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.KnowledgeDoc{}).Error
}

// ListEnabledKnowledge 取租户全部启用中的知识条目（RAG 检索的候选集）。
// limit 由调用方给出上限（检索侧固定 500 条保护），按 id 升序保证打分顺序稳定。
func (r *KnowledgeRepository) ListEnabledKnowledge(tenantID uint, limit int) ([]*model.KnowledgeDoc, error) {
	if limit <= 0 {
		limit = 500
	}
	var list []*model.KnowledgeDoc
	if err := storage.Default().DB().
		Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
