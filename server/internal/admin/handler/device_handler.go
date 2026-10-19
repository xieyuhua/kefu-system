package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// TestDevicePushHandler 手动触发一次推送，用于验证厂商通道是否接对。
// 只会推给「当前登录账号」已注册的设备，便于 APP 端自查。

// ---------------- 设备自检（推送通道验证，仅管理员） ----------------

func (h *Handler) TestDevicePushHandler(c *gin.Context) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Title == "" {
		body.Title = "推送自检"
	}
	if body.Body == "" {
		body.Body = "这是一条测试推送，收到说明厂商通道已接通"
	}
	realtime.Notify(authctx.TenantID(c), authctx.UserID(c), "test", body.Title, body.Body, 0)
	response.Default().OK(c, map[string]interface{}{"sent": true, "title": body.Title})
}
