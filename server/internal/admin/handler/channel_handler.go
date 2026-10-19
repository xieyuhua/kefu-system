package handler

import (
	"fmt"
	"net/http"
	"net/url"

	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 渠道 ----------------

func (h *Handler) ListChannelsHandler(c *gin.Context) {
	list, err := h.svc.Channels.ListChannels(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) CreateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.Channels.CreateChannel(authctx.TenantID(c), req)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

func (h *Handler) UpdateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.Channels.UpdateChannel(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

func (h *Handler) DeleteChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.Channels.DeleteChannel(authctx.TenantID(c), httpx.Default().UintParam(c, "id")); err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

// ResetChannelAppIDHandler 重置渠道标识（app_id），生成新的随机值
func (h *Handler) ResetChannelAppIDHandler(c *gin.Context) {
	appID, err := h.svc.Channels.ResetChannelAppID(authctx.TenantID(c), httpx.Default().UintParam(c, "id"))
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, map[string]interface{}{
		"app_id":     appID,
		"access_url": h.svc.Channels.AccessURLByAppID(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), ""),
	})
}

// ---------------- 二维码 / 接入地址 ----------------

// ChannelQRPNGHandler 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func (h *Handler) ChannelQRPNGHandler(c *gin.Context) {
	vo, err := h.svc.Channels.ChannelQR(authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		c.Query("base"), httpx.Default().QueryInt(c, "size", 320))
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(vo.FileName)))
	c.Data(http.StatusOK, "image/png", vo.PNG)
}

// ChannelQRInfoHandler 返回 base64 + 地址，便于页面内联展示与复制
func (h *Handler) ChannelQRInfoHandler(c *gin.Context) {
	vo, err := h.svc.Channels.ChannelQR(authctx.TenantID(c), httpx.Default().UintParam(c, "id"),
		c.Query("base"), httpx.Default().QueryInt(c, "size", 320))
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func (h *Handler) ListChannelAgentsHandler(c *gin.Context) {
	list, err := h.svc.Channels.ListChannelAgents(authctx.TenantID(c), httpx.Default().UintParam(c, "id"))
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

func (h *Handler) SetChannelAgentsHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可操作")
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if req.UserIDs == nil {
		req.UserIDs = []uint{}
	}
	list, err := h.svc.Channels.SetChannelAgents(authctx.TenantID(c), httpx.Default().UintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}
