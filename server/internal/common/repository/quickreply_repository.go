package repository

// 本文件是 common/repository 的数据访问层中「快捷回复（常用语）」域。
//
// 写表范围：quick_replies（本内核自己的表）。
// 读表范围：只读 quick_replies 一张表；创建人昵称由 service 层通过 UserRepository 读取，
// 本文件不做跨表 JOIN，避免仓储层出现跨域依赖。
//
// 形态约定：所有数据访问都是 Repository 的方法（指针接收者）；
// Repository 不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。

import (
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// QuickReplyRepository 快捷回复的数据访问层（无状态，可随时构造）。
type QuickReplyRepository struct{}

// NewQuickReplyRepository 创建数据访问层实例。
func NewQuickReplyRepository() *QuickReplyRepository { return &QuickReplyRepository{} }

// keywordWhere 给标题 / 正文追加模糊匹配条件（keyword 为空时不加条件）。
func keywordWhere(keyword string) (string, string) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return "", ""
	}
	like := "%" + kw + "%"
	return " (title LIKE ? OR content LIKE ?)", like
}

// ListVisibleQuickReplies 列出「我的私有 + 租户共享（user_id = 0）」的快捷回复：
// 租户隔离 + 归属过滤都在 SQL 里，别人的私有话术查不出来。排序：sort 升序、同序按 id 升序。
func (r *QuickReplyRepository) ListVisibleQuickReplies(tenantID, userID uint, keyword string) ([]*model.QuickReply, error) {
	var list []*model.QuickReply
	q := storage.Default().DB().
		Where("tenant_id = ? AND (user_id = 0 OR user_id = ?)", tenantID, userID)
	if cond, like := keywordWhere(keyword); cond != "" {
		q = q.Where(cond, like, like)
	}
	if err := q.Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListQuickRepliesByTenant 列出租户全部快捷回复（管理员维护用，含所有客服的私有项）。
func (r *QuickReplyRepository) ListQuickRepliesByTenant(tenantID uint, keyword string) ([]*model.QuickReply, error) {
	var list []*model.QuickReply
	q := storage.Default().DB().Where("tenant_id = ?", tenantID)
	if cond, like := keywordWhere(keyword); cond != "" {
		q = q.Where(cond, like, like)
	}
	if err := q.Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListQuickRepliesForUsers 列出「公共(user_id=0) + 指定客服私有」的启用快捷回复。
// AI 检索用：把当前渠道接待客服的快捷回复作为语料之一。
func (r *QuickReplyRepository) ListQuickRepliesForUsers(tenantID uint, userIDs []uint) ([]*model.QuickReply, error) {
	var list []*model.QuickReply
	q := storage.Default().DB().Where("tenant_id = ? AND enabled = true", tenantID)
	if len(userIDs) > 0 {
		q = q.Where("(user_id = 0 OR user_id IN ?)", userIDs)
	} else {
		q = q.Where("user_id = 0")
	}
	if err := q.Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindQuickReplyByID 按租户 + 主键查询快捷回复；未命中返回 gorm.ErrRecordNotFound。
func (r *QuickReplyRepository) FindQuickReplyByID(tenantID, id uint) (*model.QuickReply, error) {
	var qr model.QuickReply
	if err := storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&qr).Error; err != nil {
		return nil, err
	}
	return &qr, nil
}

// InsertQuickReply 新增快捷回复，成功后回填主键与时间戳。
func (r *QuickReplyRepository) InsertQuickReply(qr *model.QuickReply) error {
	return storage.Default().DB().Create(qr).Error
}

// SaveQuickReply 保存快捷回复全字段（更新路径）。
func (r *QuickReplyRepository) SaveQuickReply(qr *model.QuickReply) error {
	return storage.Default().DB().Save(qr).Error
}

// RemoveQuickReply 按租户 + 主键删除快捷回复。
func (r *QuickReplyRepository) RemoveQuickReply(tenantID, id uint) error {
	return storage.Default().DB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.QuickReply{}).Error
}
