package admin

import (
	"fmt"
	"net/http"
	"net/url"

	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 渠道 ----------------

func (h *Handler) ListChannelsHandler(c *gin.Context) {
	list, err := h.svc.ch.ListChannels(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.ch.CreateChannel(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.ch.UpdateChannel(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.ch.DeleteChannel(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ResetChannelAppIDHandler 重置渠道标识（app_id），生成新的随机值
func (h *Handler) ResetChannelAppIDHandler(c *gin.Context) {
	appID, err := h.svc.ch.ResetChannelAppID(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"app_id":     appID,
		"access_url": h.svc.ch.AccessURLByAppID(authctx.TenantID(c), httpx.UintParam(c, "id"), ""),
	})
}

// ---------------- 二维码 / 接入地址 ----------------

// ChannelQRPNGHandler 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func (h *Handler) ChannelQRPNGHandler(c *gin.Context) {
	vo, err := h.svc.ch.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
		c.Query("base"), httpx.QueryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(vo.FileName)))
	c.Data(http.StatusOK, "image/png", vo.PNG)
}

// ChannelQRInfoHandler 返回 base64 + 地址，便于页面内联展示与复制
func (h *Handler) ChannelQRInfoHandler(c *gin.Context) {
	vo, err := h.svc.ch.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
		c.Query("base"), httpx.QueryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func (h *Handler) ListChannelAgentsHandler(c *gin.Context) {
	list, err := h.svc.ch.ListChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) SetChannelAgentsHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if req.UserIDs == nil {
		req.UserIDs = []uint{}
	}
	list, err := h.svc.ch.SetChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}
