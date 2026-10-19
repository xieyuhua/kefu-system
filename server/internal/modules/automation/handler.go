package automation

// 本文件是 automation 模块的 HTTP 层（原 handler/misc.go 的自动回复规则部分）。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。

import (
	"kefu-server/internal/platform/httpx"
	"kefu-server/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func ListAutoRepliesHandler(c *gin.Context) {
	list, err := ListAutoReplies(httpx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateAutoReplyHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := CreateAutoReply(httpx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAutoReplyHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := UpdateAutoReply(httpx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAutoReplyHandler(c *gin.Context) {
	if !httpx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := DeleteAutoReply(httpx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
