package chat

// 本文件是 chat 模块的 HTTP 层（原 handler/conversation.go 的会话与消息部分）。
// handler 层不持有数据库连接：参数绑定 → 调本模块 service → 写响应。
// WebSocket 处理器见 ws.go。

import (
	"strings"

	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 会话 ----------------

func ListConversationsHandler(c *gin.Context) {
	q := ConvQuery{
		ChannelID:       httpx.QueryUint(c, "channel_id"),
		Status:          c.Query("status"),
		Scope:           c.Query("scope"),
		Keyword:         c.Query("keyword"),
		Page:            httpx.QueryInt(c, "page", 1),
		PageSize:        httpx.QueryInt(c, "page_size", 20),
		IncludeArchived: c.Query("include_archived") == "1" || c.Query("include_archived") == "true",
	}
	paged, err := ListConversations(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func GetConversationHandler(c *gin.Context) {
	vo, err := GetConversation(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func ClaimConversationHandler(c *gin.Context) {
	vo, err := ClaimConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func TransferConversationHandler(c *gin.Context) {
	var req struct {
		AgentID uint `json:"agent_id"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := TransferConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req.AgentID)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func CloseConversationHandler(c *gin.Context) {
	vo, err := CloseConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.HubInstance().Publish(authctx.TenantID(c), nil, realtime.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func MarkReadHandler(c *gin.Context) {
	if err := MarkRead(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func ConversationStatsHandler(c *gin.Context) {
	s, err := ConversationStats(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, s)
}

// ArchiveConversationHandler 归档会话（客服清理工作台列表；客户再咨询会自动恢复）
func ArchiveConversationHandler(c *gin.Context) {
	if err := ArchiveConversation(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c)); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ArchiveConversationsBulkHandler 一键清空工作台列表（批量归档）
// POST /api/conversations/archive-bulk  body/query 同列表接口的过滤条件
// 只处理「当前筛选条件下可见」的会话，保证所见即所清。
func ArchiveConversationsBulkHandler(c *gin.Context) {
	q := ConvQuery{
		ChannelID: httpx.QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		Scope:     c.Query("scope"),
		Keyword:   c.Query("keyword"),
		Page:      httpx.QueryInt(c, "page", 1),
		PageSize:  httpx.QueryInt(c, "page_size", 20),
	}
	n, err := ArchiveConversationsBulk(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, gin.H{"archived": n})
}

// UnarchiveConversationHandler 取消归档
func UnarchiveConversationHandler(c *gin.Context) {
	if err := UnarchiveConversation(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// DeleteConversationHandler 彻底删除会话与消息（不可恢复）
func DeleteConversationHandler(c *gin.Context) {
	if err := DeleteConversation(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// SearchConversationMessagesHandler 在会话内按关键词搜索历史消息
func SearchConversationMessagesHandler(c *gin.Context) {
	convID := httpx.UintParam(c, "id")
	keyword := c.Query("keyword")
	if strings.TrimSpace(keyword) == "" {
		response.OK(c, []interface{}{})
		return
	}
	rows, err := SearchMessages(authctx.TenantID(c), convID, keyword, httpx.QueryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	out := make([]interface{}, 0, len(rows))
	for _, m := range rows {
		out = append(out, BuildMessageVO(m))
	}
	response.OK(c, out)
}

// SearchConversationRecordsHandler 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=
func SearchConversationRecordsHandler(c *gin.Context) {
	q := ConversationRecordsQuery{
		Keyword:   c.Query("keyword"),
		ChannelID: httpx.QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		AgentID:   httpx.QueryUint(c, "agent_id"),
		Start:     c.Query("start"),
		End:       c.Query("end"),
		Page:      httpx.QueryInt(c, "page", 1),
		PageSize:  httpx.QueryInt(c, "page_size", 20),
	}
	list, total, err := SearchConversations(authctx.TenantID(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if list == nil {
		list = []ConversationRecord{}
	}
	response.OKPaged(c, list, total, q.Page, q.PageSize)
}

// GetConversationMessagesHandler 管理后台查看某个会话的完整消息（只读）
func GetConversationMessagesHandler(c *gin.Context) {
	list, err := ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), 0, httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- 消息 ----------------

func ListMessagesHandler(c *gin.Context) {
	list, err := ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), httpx.QueryUint(c, "before_id"), httpx.QueryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func SendMessageHandler(c *gin.Context) {
	var req dto.SendMessageRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	mvo, cvo, err := SendMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), authctx.UserID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	realtime.PublishMessage(authctx.TenantID(c), httpx.UintParam(c, "id"), mvo, cvo)
	response.OK(c, mvo)
}
