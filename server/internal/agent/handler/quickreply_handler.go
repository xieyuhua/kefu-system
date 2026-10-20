package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 快捷回复（常用语，客服自己的话术） ----------------
//
// 客服端分组（登录即可用）：
//   - 列表 = 自己的私有 + 租户共享（user_id = 0）；
//   - 新增归属当前登录账号（忽略请求里的 user_id）；
//   - 改 / 删只能动自己的，共享项与别人的私有项返回中文业务错误。
//
// 注意：这里一律按「客服视角」（isAdmin = false）处理，管理员在工作台也是「自己 + 共享」；
// 维护全租户的话术请走后台端的 /api/admin/quick-replies（见 admin/handler）。

func (h *Handler) ListQuickRepliesHandler(c *gin.Context) {
	list, err := h.svc.QuickReply.ListQuickReplies(
		authctx.TenantID(c), authctx.UserID(c), false, c.Query("keyword"),
	)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateQuickReplyHandler(c *gin.Context) {
	var req commonsvc.QuickReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	// 客服端不接受前端传的归属：一律落到当前登录账号（管理员落到自己，等同私有）
	req.UserID = nil
	vo, err := h.svc.QuickReply.CreateQuickReply(
		authctx.TenantID(c), authctx.UserID(c), false, req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateQuickReplyHandler(c *gin.Context) {
	var req commonsvc.QuickReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	req.UserID = nil
	err := h.svc.QuickReply.UpdateQuickReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		authctx.UserID(c), false, req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteQuickReplyHandler(c *gin.Context) {
	err := h.svc.QuickReply.DeleteQuickReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		authctx.UserID(c), false,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
