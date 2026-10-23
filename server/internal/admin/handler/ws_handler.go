package handler

// 「连接管理」：查看当前 WebSocket 长连接（客服 / 访客）并支持单个或批量断开。

import (
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ListWSConnectionsHandler 返回当前所有 WebSocket 连接快照。
func (h *Handler) ListWSConnectionsHandler(c *gin.Context) {
	kind := c.Query("kind")
	list := realtime.HubInstance().Snapshot()
	if kind != "" {
		filtered := make([]realtime.ConnInfo, 0, len(list))
		for _, it := range list {
			if it.Kind == kind {
				filtered = append(filtered, it)
			}
		}
		list = filtered
	}
	agents, guests := 0, 0
	for _, it := range list {
		if it.Kind == "guest" {
			guests++
		} else {
			agents++
		}
	}
	response.Default().OK(c, gin.H{"list": list, "total": len(list), "agents": agents, "guests": guests})
}

// KickWSConnectionsHandler 断开指定连接；ids 为空时按 kind 批量断开。
func (h *Handler) KickWSConnectionsHandler(c *gin.Context) {
	var req struct {
		IDs  []uint64 `json:"ids"`
		Kind string   `json:"kind"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if len(req.IDs) == 0 && req.Kind == "" {
		response.Default().Param(c, "请指定要断开的连接或类型")
		return
	}
	n := realtime.HubInstance().KickConnections(req.Kind, req.IDs)
	response.Default().OK(c, gin.H{"closed": n})
}
