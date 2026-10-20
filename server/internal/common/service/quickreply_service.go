package service

// ---------------- 快捷回复（常用语）域 ----------------
//
// 数据模型：一条快捷回复的归属由 user_id 决定 ——
//   - user_id = 0：租户共享话术，由管理员维护，所有客服可见但只读；
//   - user_id > 0：该客服的私有话术，只有本人（和管理员）能改。
//
// 权限口径（与自动回复一致）：
//   - 列表：客服 = 自己的私有 + 租户共享；管理员 = 租户全部（后台维护用）；
//   - 改 / 删：客服只能动自己的；共享项与别人的私有项一律拒绝（中文业务错误）。
//
// 形态约定：业务编排都是 QuickReplyService 的方法（指针接收者），
// 跨域依赖（账号昵称）在构造时建好，只读。

import (
	"errors"
	"strconv"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/util"
)

// QuickReplyService 快捷回复的业务编排层，持有本模块仓储与账号只读依赖。
type QuickReplyService struct {
	repo  *repository.QuickReplyRepository
	users *repository.UserRepository
}

// NewQuickReplyService 创建业务层实例（自带数据访问层依赖）。
func NewQuickReplyService() *QuickReplyService {
	return &QuickReplyService{
		repo:  repository.NewQuickReplyRepository(),
		users: repository.NewUserRepository(),
	}
}

// QuickReplyVO 快捷回复的对外视图。
type QuickReplyVO struct {
	ID        uint   `json:"id"`
	UserID    uint   `json:"user_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Sort      int    `json:"sort"`
	Enabled   bool   `json:"enabled"`
	Shared    bool   `json:"shared"`   // user_id = 0：租户公共话术
	Editable  bool   `json:"editable"` // 当前登录账号能否改 / 删
	Owner     string `json:"owner"`    // 「公共」或创建人昵称，前端直接展示
	CreatedAt string `json:"created_at"`
}

// QuickReplyRequest 新增 / 修改的请求体。
// sort / enabled 用指针：不传表示「保留原值」（新增时取 0 / true）。
// user_id 只有管理员生效：后台可把话术建给某个客服，或建为公共（0）。
type QuickReplyRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Sort    *int   `json:"sort"`
	Enabled *bool  `json:"enabled"`
	UserID  *uint  `json:"user_id"`
}

// ListQuickReplies 列出当前账号可见的快捷回复。
//   - isAdmin = true：租户全部（后台维护公共话术）；
//   - 否则：自己的私有 + 租户共享（user_id = 0）。
func (s *QuickReplyService) ListQuickReplies(tenantID, viewerID uint, isAdmin bool, keyword string) ([]QuickReplyVO, error) {
	var (
		list []*model.QuickReply
		err  error
	)
	if isAdmin {
		list, err = s.repo.ListQuickRepliesByTenant(tenantID, keyword)
	} else {
		list, err = s.repo.ListVisibleQuickReplies(tenantID, viewerID, keyword)
	}
	if err != nil {
		return nil, err
	}
	// 同一批数据里重复的创建人只查一次账号表
	names := make(map[uint]string, 4)
	out := make([]QuickReplyVO, 0, len(list))
	for _, r := range list {
		out = append(out, s.toQuickReplyVO(tenantID, r, viewerID, isAdmin, names))
	}
	return out, nil
}

// toQuickReplyVO 组装视图：权限位与归属展示文案都由这里统一给出。
func (s *QuickReplyService) toQuickReplyVO(tenantID uint, r *model.QuickReply, viewerID uint, isAdmin bool, names map[uint]string) QuickReplyVO {
	vo := QuickReplyVO{
		ID: r.ID, UserID: r.UserID, Title: r.Title, Content: r.Content,
		Sort: r.Sort, Enabled: r.Enabled,
		Shared:    r.UserID == 0,
		Editable:  quickReplyMutable(r, viewerID, isAdmin),
		CreatedAt: util.Default().FmtTime(r.CreatedAt),
	}
	if r.UserID == 0 {
		vo.Owner = "公共"
		return vo
	}
	vo.Owner = names[r.UserID]
	if vo.Owner == "" {
		// 账号被删除时不让列表出现空白，退化成账号 ID
		if u, err := s.users.FindUserByID(tenantID, r.UserID); err == nil && u != nil {
			vo.Owner = u.Name
		}
		if vo.Owner == "" {
			vo.Owner = "客服 " + strconv.FormatUint(uint64(r.UserID), 10)
		}
	}
	names[r.UserID] = vo.Owner
	return vo
}

// CreateQuickReply 新增快捷回复。
// 客服建的归属固定为当前账号（忽略请求里的 user_id）；管理员可指定归属，缺省为公共（0）。
func (s *QuickReplyService) CreateQuickReply(tenantID, actorID uint, isAdmin bool, req QuickReplyRequest) (*QuickReplyVO, error) {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" {
		return nil, errors.New("标题不能为空")
	}
	if content == "" {
		return nil, errors.New("回复内容不能为空")
	}
	owner := actorID
	if isAdmin {
		owner = 0
		if req.UserID != nil {
			owner = *req.UserID
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	r := &model.QuickReply{
		TenantID: tenantID, UserID: owner, Title: title, Content: content,
		Enabled: true,
	}
	if req.Sort != nil {
		r.Sort = *req.Sort
	}
	if err := s.repo.InsertQuickReply(r); err != nil {
		return nil, err
	}
	// bool 的零值会被 gorm 的 `default:true` 覆盖（关闭状态插不进去），落库后再显式写回一次
	if !enabled {
		r.Enabled = false
		if err := s.repo.SaveQuickReply(r); err != nil {
			return nil, err
		}
	}
	vo := s.toQuickReplyVO(tenantID, r, actorID, isAdmin, map[uint]string{})
	return &vo, nil
}

// UpdateQuickReply 修改快捷回复：客服只能改自己的，共享项与别人的私有项一律拒绝。
func (s *QuickReplyService) UpdateQuickReply(tenantID, id, actorID uint, isAdmin bool, req QuickReplyRequest) error {
	r, err := s.repo.FindQuickReplyByID(tenantID, id)
	if err != nil {
		return errors.New("快捷回复不存在")
	}
	if !quickReplyMutable(r, actorID, isAdmin) {
		return errors.New("只能修改自己的快捷回复")
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		r.Title = title
	}
	if content := strings.TrimSpace(req.Content); content != "" {
		r.Content = content
	}
	if req.Sort != nil {
		r.Sort = *req.Sort
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if isAdmin && req.UserID != nil {
		r.UserID = *req.UserID
	}
	return s.repo.SaveQuickReply(r)
}

// DeleteQuickReply 删除快捷回复：权限口径同修改。
func (s *QuickReplyService) DeleteQuickReply(tenantID, id, actorID uint, isAdmin bool) error {
	r, err := s.repo.FindQuickReplyByID(tenantID, id)
	if err != nil {
		return errors.New("快捷回复不存在")
	}
	if !quickReplyMutable(r, actorID, isAdmin) {
		return errors.New("只能删除自己的快捷回复")
	}
	return s.repo.RemoveQuickReply(tenantID, id)
}

// quickReplyMutable 判断当前账号能否改 / 删某条快捷回复：
// 管理员可改全部；客服只能改自己创建的私有项，租户共享项（user_id = 0）只读。
func quickReplyMutable(r *model.QuickReply, actorID uint, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	return r.UserID > 0 && r.UserID == actorID
}
