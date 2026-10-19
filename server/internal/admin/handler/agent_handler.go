package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 子账号 ----------------

// ListAgentsHandler 客服列表。
//   - admin：看全部
//   - agent：带 ?scope=transfer 时可看「与自己接待渠道有交集」的在线客服（用于会话转接）
func (h *Handler) ListAgentsHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) && c.Query("scope") != "transfer" {
		response.Default().Forbid(c, "仅管理员可查看")
		return
	}
	list, err := h.svc.Account.ListAgents(authctx.TenantID(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	if !authctx.IsAdmin(c) {
		// 非管理员只返回在线、且未停用的同事
		filtered := make([]dto.AgentVO, 0, len(list))
		for _, a := range list {
			if a.Online && a.Enabled && a.ID != authctx.UserID(c) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Account.CreateAgent(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.Account.UpdateAgent(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.Account.DeleteAgent(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) SetOnlineHandler(c *gin.Context) {
	var body struct {
		Online bool `json:"online"`
	}
	_ = c.ShouldBindJSON(&body)
	target := httpx.Default().UintParam(c, "id")
	cl := authctx.ClaimsFromContext(c)
	if cl == nil {
		return
	}
	if target != cl.UserID && cl.Role != "admin" {
		response.Default().Forbid(c, "只能修改自己的在线状态")
		return
	}
	if err := h.svc.Account.SetOnline(authctx.TenantID(c), target, body.Online); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	status := "offline"
	if body.Online {
		status = "online"
	}
	response.Default().OK(c, map[string]interface{}{"online": body.Online, "status": status})
}
