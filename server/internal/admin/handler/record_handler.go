package handler

import (
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	commonsvc "kefu-server/internal/common/service"
)

// SearchConversationRecordsHandler 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=

// ---------------- 对话记录（全局检索 / 只读回放） ----------------

func (h *Handler) SearchConversationRecordsHandler(c *gin.Context) {
	q := commonsvc.ConversationRecordsQuery{
		Keyword:   c.Query("keyword"),
		ChannelID: httpx.Default().QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		AgentID:   httpx.Default().QueryUint(c, "agent_id"),
		Start:     c.Query("start"),
		End:       c.Query("end"),
		Page:      httpx.Default().QueryInt(c, "page", 1),
		PageSize:  httpx.Default().QueryInt(c, "page_size", 20),
	}
	list, total, err := h.svc.Conv.SearchConversations(authctx.TenantID(c), q)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	if list == nil {
		list = []commonsvc.ConversationRecord{}
	}
	response.Default().OKPaged(c, list, total, q.Page, q.PageSize)
}

// GetConversationMessagesHandler 管理后台查看某个会话的完整消息（只读回放）
//
// GET /api/conversation-records/:id/messages?before_id=&limit=
//   - limit：单页条数，缺省 50、上限 200（超出按上限处理）；
//   - before_id：游标，返回 id < before_id 的更早消息；缺省 0 表示「最新一页」。
//
// 响应 data 为 { list, total, has_more }：
//   - total：该会话的消息总数（DB 实际条数，供抽屉显示「共 X 条」）；
//   - has_more：本次返回的最早一条之前是否还有更早的消息（true 时前端继续用首条 id 当 before_id 拉取）。
//
// 完整回放 = 首屏 before_id=0 取一页，之后循环拉取直到 has_more=false，累计条数即等于 total。
func (h *Handler) GetConversationMessagesHandler(c *gin.Context) {
	page, err := h.svc.Conv.ListConversationMessagesPage(
		authctx.TenantID(c),
		httpx.Default().UintParam(c, "id"),
		httpx.Default().QueryUint(c, "before_id"),
		httpx.Default().QueryInt(c, "limit", commonsvc.RecordMessagePageDefault),
	)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	response.Default().OK(c, page)
}
