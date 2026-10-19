package service

// 会话域的 WebSocket 公共件：升级器 Upgrader。
// 客服端连接处理器在 agent/handler（WSHandler），用户端（访客）连接处理器在
// client/handler（GuestWSHandler），两端共用本文件的 Upgrader，避免两处配置漂移。

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// Upgrader WebSocket 升级器。客服端与用户端共用同一套参数，
// 因此导出给两端 handler 使用，避免两处配置漂移。
var Upgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   4096,
	CheckOrigin:       func(r *http.Request) bool { return true },
	EnableCompression: true,
}
