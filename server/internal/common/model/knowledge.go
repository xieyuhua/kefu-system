package model

// KnowledgeDoc 知识库条目：机器人（AI/RAG）回答时检索的资料来源。
//
// 设计取舍：知识条目只是「纯文本 + 标签」，不引入向量库 / 外部检索服务 ——
// 检索在 common/service/rag.go 里用「关键词 + 中文 2-gram 重叠」打分完成，
// 因此本表不需要存 embedding，运维只需要一张普通业务表（可用 sqlite / mysql 直接备份）。
type KnowledgeDoc struct {
	Model
	TenantID uint   `gorm:"index;not null" json:"tenant_id"`
	Title    string `gorm:"size:128" json:"title"`
	Content  string `gorm:"type:text" json:"content"`
	Tags     string `gorm:"size:128" json:"tags"` // 逗号分隔，便于检索加权
	Enabled  bool   `gorm:"default:true" json:"enabled"`
}
