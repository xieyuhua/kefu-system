package handler

import (
	"strings"

	"kefu-server/internal/dto"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// ---------------- Conversation ----------------

func ListConversations(c *gin.Context) {
	q := service.ConvQuery{
		ChannelID:       queryUint(c, "channel_id"),
		Status:          c.Query("status"),
		Scope:           c.Query("scope"),
		Keyword:         c.Query("keyword"),
		Page:            queryInt(c, "page", 1),
		PageSize:        queryInt(c, "page_size", 20),
		IncludeArchived: c.Query("include_archived") == "1" || c.Query("include_archived") == "true",
	}
	paged, err := service.ListConversations(tenantID(c), userID(c), isAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

func GetConversation(c *gin.Context) {
	vo, err := service.GetConversation(tenantID(c), uintParam(c, "id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func ClaimConversation(c *gin.Context) {
	vo, err := service.ClaimConversation(tenantID(c), uintParam(c, "id"), userID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	service.HubInstance().Publish(tenantID(c), nil, service.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func TransferConversation(c *gin.Context) {
	var req struct {
		AgentID uint `json:"agent_id"`
	}
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.TransferConversation(tenantID(c), uintParam(c, "id"), userID(c), req.AgentID)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	service.HubInstance().Publish(tenantID(c), nil, service.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func CloseConversation(c *gin.Context) {
	vo, err := service.CloseConversation(tenantID(c), uintParam(c, "id"), userID(c))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	service.HubInstance().Publish(tenantID(c), nil, service.Event{Event: "conversation.updated", Data: vo})
	response.OK(c, vo)
}

func MarkRead(c *gin.Context) {
	if err := service.MarkRead(tenantID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func ConversationStats(c *gin.Context) {
	s, err := service.ConversationStats(tenantID(c), userID(c), isAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, s)
}

// ArchiveConversation 归档会话（客服清理工作台列表；客户再咨询会自动恢复）
func ArchiveConversation(c *gin.Context) {
	if err := service.ArchiveConversation(tenantID(c), uintParam(c, "id"), userID(c)); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ArchiveConversationsBulk 一键清空工作台列表（批量归档）
// POST /api/conversations/archive-bulk  body/query 同列表接口的过滤条件
// 只处理「当前筛选条件下可见」的会话，保证所见即所清。
func ArchiveConversationsBulk(c *gin.Context) {
	q := service.ConvQuery{
		ChannelID: queryUint(c, "channel_id"),
		Status:    c.Query("status"),
		Scope:     c.Query("scope"),
		Keyword:   c.Query("keyword"),
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 20),
	}
	n, err := service.ArchiveConversationsBulk(tenantID(c), userID(c), isAdmin(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, gin.H{"archived": n})
}

// UnarchiveConversation 取消归档
func UnarchiveConversation(c *gin.Context) {
	if err := service.UnarchiveConversation(tenantID(c), uintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// DeleteConversation 彻底删除会话与消息（不可恢复）
func DeleteConversation(c *gin.Context) {
	if err := service.DeleteConversation(tenantID(c), uintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// SearchConversationMessages 在会话内按关键词搜索历史消息
func SearchConversationMessages(c *gin.Context) {
	convID := uintParam(c, "id")
	keyword := c.Query("keyword")
	if strings.TrimSpace(keyword) == "" {
		response.OK(c, []interface{}{})
		return
	}
	rows, err := service.SearchMessages(tenantID(c), convID, keyword, queryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	out := make([]interface{}, 0, len(rows))
	for _, m := range rows {
		out = append(out, service.BuildMessageVO(m))
	}
	response.OK(c, out)
}

// SearchConversationRecords 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=
func SearchConversationRecords(c *gin.Context) {
	q := service.ConversationRecordsQuery{
		Keyword:   c.Query("keyword"),
		ChannelID: queryUint(c, "channel_id"),
		Status:    c.Query("status"),
		AgentID:   queryUint(c, "agent_id"),
		Start:     c.Query("start"),
		End:       c.Query("end"),
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 20),
	}
	list, total, err := service.SearchConversations(tenantID(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if list == nil {
		list = []service.ConversationRecord{}
	}
	response.OKPaged(c, list, total, q.Page, q.PageSize)
}

// GetConversationMessages 管理后台查看某个会话的完整消息（只读）
func GetConversationMessages(c *gin.Context) {
	list, err := service.ListMessages(tenantID(c), uintParam(c, "id"), 0, queryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- Message ----------------

func ListMessages(c *gin.Context) {
	list, err := service.ListMessages(tenantID(c), uintParam(c, "id"), queryUint(c, "before_id"), queryInt(c, "limit", 50))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func SendMessage(c *gin.Context) {
	var req dto.SendMessageRequest
	if !bindJSON(c, &req) {
		return
	}
	mvo, cvo, err := service.SendMessage(tenantID(c), uintParam(c, "id"), userID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	service.PublishMessage(tenantID(c), uintParam(c, "id"), mvo, cvo)
	response.OK(c, mvo)
}

// ---------------- 客户标签 ----------------

func ListTags(c *gin.Context) {
	list, err := service.ListTags(tenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateTag(c *gin.Context) {
	var req service.TagRequest
	if !bindJSON(c, &req) {
		return
	}
	vo, err := service.CreateTag(tenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func DeleteTag(c *gin.Context) {
	if err := service.DeleteTag(tenantID(c), uintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// SetCustomerTags 给客户打/改标签（接待会话中即可操作，其他客服可见）
func SetCustomerTags(c *gin.Context) {
	var req struct {
		TagIDs []uint `json:"tag_ids"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.TagIDs == nil {
		req.TagIDs = []uint{}
	}
	tags, err := service.SetCustomerTags(tenantID(c), uintParam(c, "id"), userID(c), req.TagIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, tags)
}

func UpdateCustomerRemark(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := service.UpdateCustomerRemark(tenantID(c), uintParam(c, "id"), req.Remark); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}
