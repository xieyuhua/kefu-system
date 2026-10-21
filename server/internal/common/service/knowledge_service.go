package service

// ---------------- 知识库（AI / RAG 资料来源）域 ----------------
//
// 知识条目是「标题 + 正文 + 标签」的纯文本，由管理员在后台维护；
// 机器人未命中关键词规则时，检索内核（rag.go）会从中挑出最相关的几条拼进提示词。
//
// 权限口径：只有管理员能增删改（后台页面 /api/admin/knowledge），客服端不暴露该接口 ——
// 知识条目直接决定机器人对客户说什么，属于「对外话术」级别的内容，收敛到后台维护。
//
// 形态约定：业务编排都是 KnowledgeService 的方法（指针接收者），
// 数据访问走本域仓储（repository.KnowledgeRepository）。

import (
	"errors"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/response"
	"kefu-server/pkg/storage"
	"kefu-server/pkg/util"
)

// 知识条目的字段上限：与 model.KnowledgeDoc 的 gorm size 保持一致，
// 后台超长输入在业务层就报中文错误，而不是等数据库截断 / 报 1406。
const (
	knowledgeTitleMaxLen = 128
	knowledgeTagsMaxLen  = 128
	knowledgeContentMax  = 5000
)

// KnowledgeService 知识库的业务编排层，持有本域仓储。
type KnowledgeService struct {
	repo *repository.KnowledgeRepository
}

// NewKnowledgeService 创建业务层实例（自带数据访问层依赖）。
func NewKnowledgeService() *KnowledgeService {
	return &KnowledgeService{repo: repository.NewKnowledgeRepository()}
}

// KnowledgeVO 知识条目的对外视图。
type KnowledgeVO struct {
	ID        uint   `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Tags      string `json:"tags"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// KnowledgeRequest 新增 / 修改知识条目的请求体。
// enabled 用指针：不传表示「保留原值」（新增时默认启用）。
type KnowledgeRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Tags    string `json:"tags"`
	Enabled *bool  `json:"enabled"`
}

// ListKnowledge 分页列出租户的知识条目（keyword 匹配标题 / 正文 / 标签）。
func (s *KnowledgeService) ListKnowledge(tenantID uint, keyword string, page, pageSize int) (*response.Paged, error) {
	// 统一夹取并回显夹取后的值，口径与其它分页接口一致
	page, pageSize = storage.Default().ClampPage(page, pageSize)
	list, total, err := s.repo.ListKnowledgePaged(tenantID, keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	vos := make([]KnowledgeVO, 0, len(list))
	for _, doc := range list {
		vos = append(vos, toKnowledgeVO(doc))
	}
	return &response.Paged{List: vos, Total: total, Page: page, PageSize: pageSize}, nil
}

// CreateKnowledge 新增知识条目。
func (s *KnowledgeService) CreateKnowledge(tenantID uint, req KnowledgeRequest) (*KnowledgeVO, error) {
	title, content, tags, err := normalizeKnowledgeInput(req)
	if err != nil {
		return nil, err
	}
	doc := &model.KnowledgeDoc{TenantID: tenantID, Title: title, Content: content, Tags: tags}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := s.repo.InsertKnowledge(doc); err != nil {
		return nil, err
	}
	// bool 的零值会被 gorm 的 `default:true` 覆盖（关闭状态插不进去），落库后再显式写回一次
	if !enabled {
		doc.Enabled = false
		if err := s.repo.SaveKnowledge(doc); err != nil {
			return nil, err
		}
	}
	vo := toKnowledgeVO(doc)
	return &vo, nil
}

// UpdateKnowledge 修改知识条目：只覆盖请求里给出的字段（空串视为「清空该字段」，
// 与快捷回复「空串不改」的口径不同 —— 知识的标签本来就允许被清空）。
func (s *KnowledgeService) UpdateKnowledge(tenantID, id uint, req KnowledgeRequest) error {
	doc, err := s.repo.FindKnowledgeByID(tenantID, id)
	if err != nil {
		return errors.New("知识条目不存在")
	}
	title, content, tags, verr := normalizeKnowledgeInput(req)
	if verr != nil {
		return verr
	}
	doc.Title = title
	doc.Content = content
	doc.Tags = tags
	if req.Enabled != nil {
		doc.Enabled = *req.Enabled
	}
	return s.repo.SaveKnowledge(doc)
}

// DeleteKnowledge 删除知识条目。
func (s *KnowledgeService) DeleteKnowledge(tenantID, id uint) error {
	if _, err := s.repo.FindKnowledgeByID(tenantID, id); err != nil {
		return errors.New("知识条目不存在")
	}
	return s.repo.RemoveKnowledge(tenantID, id)
}

// toKnowledgeVO 组装视图（时间统一按 "2006-01-02 15:04:05" 格式化，与其它列表一致）。
func toKnowledgeVO(doc *model.KnowledgeDoc) KnowledgeVO {
	return KnowledgeVO{
		ID:        doc.ID,
		Title:     doc.Title,
		Content:   doc.Content,
		Tags:      doc.Tags,
		Enabled:   doc.Enabled,
		CreatedAt: util.Default().FmtTime(doc.CreatedAt),
		UpdatedAt: util.Default().FmtTime(doc.UpdatedAt),
	}
}

// normalizeKnowledgeInput 规整并校验新增 / 修改的入参（中文错误文案直接回给后台）。
func normalizeKnowledgeInput(req KnowledgeRequest) (title, content, tags string, err error) {
	title = strings.TrimSpace(req.Title)
	content = strings.TrimSpace(req.Content)
	tags = strings.TrimSpace(req.Tags)
	if title == "" {
		return "", "", "", errors.New("标题不能为空")
	}
	if content == "" {
		return "", "", "", errors.New("知识内容不能为空")
	}
	if n := len([]rune(title)); n > knowledgeTitleMaxLen {
		return "", "", "", errors.New("标题过长（最多 128 个字符）")
	}
	if n := len([]rune(tags)); n > knowledgeTagsMaxLen {
		return "", "", "", errors.New("标签过长（最多 128 个字符，逗号分隔）")
	}
	if n := len([]rune(content)); n > knowledgeContentMax {
		return "", "", "", errors.New("知识内容过长（最多 5000 个字符）")
	}
	return title, content, tags, nil
}
