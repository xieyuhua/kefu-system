package handler

import (
	"kefu-server/internal/common/authctx"
	"kefu-server/internal/common/setting"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 系统配置（默认图片等） ----------------
// 实现完全落在 platform/setting（account 的登录响应也要用），本模块只做 HTTP 适配。

// ListSettingsHandler 读取全部配置项
func (h *Handler) ListSettingsHandler(c *gin.Context) {
	list, err := setting.ListSettings(authctx.TenantID(c))
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, list)
}

// SaveSettingHandler 保存单个配置项
func (h *Handler) SaveSettingHandler(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := setting.SaveSetting(authctx.TenantID(c), req.Key, req.Value); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}
