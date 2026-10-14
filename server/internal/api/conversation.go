package api

import (
	"kefu-server/internal/dto"
	"kefu-server/internal/response"
	"kefu-server/internal/service"

	"github.com/gin-gonic/gin"
)

// ---------------- Conversation ----------------

func ListConversations(c *gin.Context) {
	q := service.ConvQuery{
		ChannelID: queryUint(c, "channel_id"),
		Status:    c.Query("status"),
		Scope:     c.Query("scope"),
		Keyword:   c.Query("keyword"),
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 20),
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
