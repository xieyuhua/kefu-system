package admin

// 本文件是后台端（modules/admin）的 HTTP 层：渠道与渠道类型、子账号、客户与标签、
// 自动回复、设置与统计、设备自检、对话记录。
// handler 层不持有数据库连接：参数绑定 → 调 core/* 的 service（或本端 service）→ 写响应。
//
// 依赖方向：modules/admin → core/* → platform → pkg。
// 本包不 import modules/client、modules/agent，也不被 core/* import。
//
// 说明：渠道、子账号、客户标签、自动回复这些能力的 service 全在 core/*，
// 后台端只负责「管理员视角的入参与鉴权分支」；概览统计是后台专属口径，见 service.go。

import (
	"fmt"
	"net/http"
	"net/url"

	"kefu-server/internal/core/account"
	"kefu-server/internal/core/automation"
	"kefu-server/internal/core/channel"
	"kefu-server/internal/core/conversation"
	"kefu-server/internal/core/customer"
	"kefu-server/internal/platform/authctx"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/realtime"
	"kefu-server/internal/platform/setting"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ---------------- 渠道 ----------------

func ListChannelsHandler(c *gin.Context) {
	list, err := channel.ListChannels(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
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
	vo, err := channel.CreateChannel(authctx.TenantID(c), req)
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
	if err := channel.UpdateChannel(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
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
	if err := channel.DeleteChannel(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ResetChannelAppIDHandler 重置渠道标识（app_id），生成新的随机值
func ResetChannelAppIDHandler(c *gin.Context) {
	appID, err := channel.ResetChannelAppID(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"app_id":     appID,
		"access_url": channel.AccessURLByAppID(authctx.TenantID(c), httpx.UintParam(c, "id"), ""),
	})
}

// ---------------- 二维码 / 接入地址 ----------------

// ChannelQRPNGHandler 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func ChannelQRPNGHandler(c *gin.Context) {
	vo, err := channel.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
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
	vo, err := channel.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
		c.Query("base"), httpx.QueryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func ListChannelAgentsHandler(c *gin.Context) {
	list, err := channel.ListChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"))
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
	list, err := channel.SetChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func ListChannelTypesHandler(c *gin.Context) {
	list, err := channel.ListChannelTypes(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateChannelTypeHandler(c *gin.Context) {
	var req channel.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := channel.CreateChannelType(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateChannelTypeHandler(c *gin.Context) {
	var req channel.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := channel.UpdateChannelType(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteChannelTypeHandler(c *gin.Context) {
	if err := channel.DeleteChannelType(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 子账号 ----------------

// ListAgentsHandler 客服列表。
//   - admin：看全部
//   - agent：带 ?scope=transfer 时可看「与自己接待渠道有交集」的在线客服（用于会话转接）
func ListAgentsHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) && c.Query("scope") != "transfer" {
		response.Forbid(c, "仅管理员可查看")
		return
	}
	list, err := account.ListAgents(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if !authctx.IsAdmin(c) {
		// 非管理员只返回在线、且未停用的同事
		filtered := make([]dto.AgentVO, 0, len(list))
		for _, a := range list {
			if a.Online && a.Enabled && a.ID != authctx.UserID(c) {
				filtered = append(filtered, a)
			}
		}
		list = filtered
	}
	response.OK(c, list)
}

func CreateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := account.CreateAgent(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := account.UpdateAgent(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := account.DeleteAgent(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func SetOnlineHandler(c *gin.Context) {
	var body struct {
		Online bool `json:"online"`
	}
	_ = c.ShouldBindJSON(&body)
	target := httpx.UintParam(c, "id")
	cl := authctx.ClaimsFromContext(c)
	if cl == nil {
		return
	}
	if target != cl.UserID && cl.Role != "admin" {
		response.Forbid(c, "只能修改自己的在线状态")
		return
	}
	if err := account.SetOnline(authctx.TenantID(c), target, body.Online); err != nil {
		response.Server(c, err.Error())
		return
	}
	status := "offline"
	if body.Online {
		status = "online"
	}
	response.OK(c, map[string]interface{}{"online": body.Online, "status": status})
}

// ---------------- 客户 / 历史档案 ----------------

func ListCustomersHandler(c *gin.Context) {
	paged, err := customer.ListCustomers(authctx.TenantID(c), c.Query("keyword"), httpx.QueryUint(c, "channel_id"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

// ---------------- 客户标签 ----------------

func ListTagsHandler(c *gin.Context) {
	list, err := customer.ListTags(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateTagHandler(c *gin.Context) {
	var req customer.TagRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := customer.CreateTag(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func DeleteTagHandler(c *gin.Context) {
	if err := customer.DeleteTag(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func UpdateCustomerRemarkHandler(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := customer.UpdateCustomerRemark(authctx.TenantID(c), httpx.UintParam(c, "id"), req.Remark); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 自动回复规则（关键词 → 机器人回复 / 转人工） ----------------

func ListAutoRepliesHandler(c *gin.Context) {
	list, err := automation.ListAutoReplies(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func CreateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := automation.CreateAutoReply(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func UpdateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := automation.UpdateAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func DeleteAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := automation.DeleteAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 系统配置（默认图片等） ----------------
// 实现完全落在 platform/setting（account 的登录响应也要用），本模块只做 HTTP 适配。

// ListSettingsHandler 读取全部配置项
func ListSettingsHandler(c *gin.Context) {
	list, err := setting.ListSettings(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// SaveSettingHandler 保存单个配置项
func SaveSettingHandler(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := setting.SaveSetting(authctx.TenantID(c), req.Key, req.Value); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 数据统计 ----------------

func StatsOverviewHandler(c *gin.Context) {
	days := httpx.QueryInt(c, "days", 7)
	data, err := StatsOverviewData(authctx.TenantID(c), days)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, data)
}

// TestDevicePushHandler 手动触发一次推送，用于验证厂商通道是否接对。
// 只会推给「当前登录账号」已注册的设备，便于 APP 端自查。

// ---------------- 设备自检（推送通道验证，仅管理员） ----------------

func TestDevicePushHandler(c *gin.Context) {
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
	response.OK(c, map[string]interface{}{"sent": true, "title": body.Title})
}

// SearchConversationRecordsHandler 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=

// ---------------- 对话记录（全局检索 / 只读回放） ----------------

func SearchConversationRecordsHandler(c *gin.Context) {
	q := conversation.ConversationRecordsQuery{
		Keyword:   c.Query("keyword"),
		ChannelID: httpx.QueryUint(c, "channel_id"),
		Status:    c.Query("status"),
		AgentID:   httpx.QueryUint(c, "agent_id"),
		Start:     c.Query("start"),
		End:       c.Query("end"),
		Page:      httpx.QueryInt(c, "page", 1),
		PageSize:  httpx.QueryInt(c, "page_size", 20),
	}
	list, total, err := conversation.SearchConversations(authctx.TenantID(c), q)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	if list == nil {
		list = []conversation.ConversationRecord{}
	}
	response.OKPaged(c, list, total, q.Page, q.PageSize)
}

// GetConversationMessagesHandler 管理后台查看某个会话的完整消息（只读）
func GetConversationMessagesHandler(c *gin.Context) {
	list, err := conversation.ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), 0, httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}
