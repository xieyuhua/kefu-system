package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// ---------------- 自动回复规则（关键词 → 机器人回复 / 转人工） ----------------
//
// 这组端点是「团队共用」的：后台与客服工作台都打同一组 method+path。
// 权限口径在公共业务层（common/service.AutomationService）里统一实现：
//   - 列表：全租户可见（规则本来就是团队共用的），每条带 editable 供前端置灰；
//   - 新增：客服建的归属自己，管理员建的是公共规则（user_id = 0）；
//   - 改 / 删：管理员可动全部，客服只能动自己创建的，公共规则只读（中文业务错误）。
//
// 之所以不再按 admin / agent 各注册一份：gin 同一个 engine 上重复注册同一 method+path 会
// panic（handlers are already registered for path ...），而两端共用同一个 /api 分组。

// autoReplyActor 当前操作者：账号 ID + 是否管理员（归属与写权限都由业务层据此判定）。
func autoReplyActor(c *gin.Context) (actorID uint, isAdmin bool) {
	return authctx.UserID(c), authctx.IsAdmin(c)
}

func (h *Handler) ListAutoRepliesHandler(c *gin.Context) {
	list, err := h.svc.AutoReply.ListAutoReplies(
		authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c),
	)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateAutoReplyHandler(c *gin.Context) {
	var req commonsvc.AutoReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	actorID, isAdmin := autoReplyActor(c)
	vo, err := h.svc.AutoReply.CreateAutoReply(authctx.TenantID(c), actorID, isAdmin, req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateAutoReplyHandler(c *gin.Context) {
	var req commonsvc.AutoReplyRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	actorID, isAdmin := autoReplyActor(c)
	err := h.svc.AutoReply.UpdateAutoReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"), actorID, isAdmin, req,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteAutoReplyHandler(c *gin.Context) {
	actorID, isAdmin := autoReplyActor(c)
	err := h.svc.AutoReply.DeleteAutoReply(
		authctx.TenantID(c), httpx.Default().UintParam(c, "id"), actorID, isAdmin,
	)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
