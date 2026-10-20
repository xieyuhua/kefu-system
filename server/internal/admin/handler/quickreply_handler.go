package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 快捷回复：管理员维护租户全部（含公共话术 user_id = 0） ----------------
//
// 与客服端那组端点的区别只有范围：这里是「租户全部」视角 ——
//   - 列表：含所有客服的私有项，便于管理员统一维护 / 清理；
//   - 新增：默认建为公共话术（user_id = 0），也可传 user_id 指定归属某个客服；
//   - 改 / 删：租户内任意一条都可以（管理员口径）。

func (h *Handler) ListTenantQuickRepliesHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	list, err := h.svc.QuickReply.ListQuickReplies(
		authctx.TenantID(c), authctx.UserID(c), true, c.Query("keyword"),
	)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateTenantQuickReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.QuickReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.QuickReply.CreateQuickReply(
		authctx.TenantID(c), authctx.UserID(c), true, req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateTenantQuickReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req commonsvc.QuickReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	err := h.svc.QuickReply.UpdateQuickReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		authctx.UserID(c), true, req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteTenantQuickReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	err := h.svc.QuickReply.DeleteQuickReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		authctx.UserID(c), true,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
