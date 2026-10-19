package channel

// 本文件是 channel 模块的 HTTP 层（原 handler/base.go 的渠道部分 + handler/misc.go 的
// 渠道二维码 / 接待账号 / 渠道类型 / 重置 app_id 部分）。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。

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

func ListChannelsHandler(c *gin.Context) {
	list, err := ListChannels(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := CreateChannel(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := UpdateChannel(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := DeleteChannel(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ResetChannelAppIDHandler 重置渠道标识（app_id），生成新的随机值
func ResetChannelAppIDHandler(c *gin.Context) {
	appID, err := ResetChannelAppID(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"app_id":     appID,
		"access_url": AccessURLByAppID(authctx.TenantID(c), httpx.UintParam(c, "id"), ""),
	})
}

// ---------------- 二维码 / 接入地址 ----------------

// ChannelQRPNGHandler 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func ChannelQRPNGHandler(c *gin.Context) {
	vo, err := ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
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
func ChannelQRInfoHandler(c *gin.Context) {
	vo, err := ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
		c.Query("base"), httpx.QueryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func ListChannelAgentsHandler(c *gin.Context) {
	list, err := ListChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

func SetChannelAgentsHandler(c *gin.Context) {
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
	list, err := SetChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func ListChannelTypesHandler(c *gin.Context) {
	list, err := ListChannelTypes(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateChannelTypeHandler(c *gin.Context) {
	var req ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := CreateChannelType(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateChannelTypeHandler(c *gin.Context) {
	var req ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := UpdateChannelType(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteChannelTypeHandler(c *gin.Context) {
	if err := DeleteChannelType(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}
