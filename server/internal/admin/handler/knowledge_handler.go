package handler

// 本文件是后台端的知识库 HTTP 层（AI / RAG 应答的资料来源）。
//
// 权限口径：只有管理员能读写 —— 知识条目直接决定机器人对客户说什么，
// 属于「对外话术」级别的内容，与 /admin/quick-replies 同样收敛在后台侧。
//
// 路由（见 internal/admin/route）：
//
//	GET    /api/admin/knowledge?keyword=&page=&page_size=
//	POST   /api/admin/knowledge
//	PUT    /api/admin/knowledge/:id
//	DELETE /api/admin/knowledge/:id

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ListKnowledgeHandler 分页列出知识条目（keyword 匹配标题 / 正文 / 标签）。
func (h *Handler) ListKnowledgeHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	paged, err := h.svc.Knowledge.ListKnowledge(
		authctx.TenantID(c), c.Query("keyword"),
		httpx.Default().QueryInt(c, "page", 1), httpx.Default().QueryInt(c, "page_size", 20),
	)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, paged)
}

// CreateKnowledgeHandler 新增知识条目。
func (h *Handler) CreateKnowledgeHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.KnowledgeRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Knowledge.CreateKnowledge(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

// UpdateKnowledgeHandler 修改知识条目。
func (h *Handler) UpdateKnowledgeHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.KnowledgeRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	err := h.svc.Knowledge.UpdateKnowledge(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

// DeleteKnowledgeHandler 删除知识条目。
func (h *Handler) DeleteKnowledgeHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.Knowledge.DeleteKnowledge(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
