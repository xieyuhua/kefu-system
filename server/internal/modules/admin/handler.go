package admin

// 本文件是后台端（modules/admin）的 HTTP 层：渠道与渠道类型、子账号、客户与标签、
// 自动回复、设置与统计、设备自检、对话记录。
// handler 层不持有数据库连接：参数绑定 → 调本端 service（其内已持有各内核 service）→ 写响应。
// 所有处理器都是 *Handler 的方法（指针接收者），实例由 route 在 Setup 里创建一次。
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

// Handler 后台端的 HTTP 层，持有本端 service（service 内已建好各内核依赖）。
type Handler struct {
	svc *Service
}

// NewHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewHandler() *Handler { return &Handler{svc: NewService()} }

// ---------------- 渠道 ----------------

func (h *Handler) ListChannelsHandler(c *gin.Context) {
	list, err := h.svc.ch.ListChannels(authctx.TenantID(c), authctx.UserID(c), authctx.IsAdmin(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.ch.CreateChannel(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.ChannelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.ch.UpdateChannel(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteChannelHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.ch.DeleteChannel(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ResetChannelAppIDHandler 重置渠道标识（app_id），生成新的随机值
func (h *Handler) ResetChannelAppIDHandler(c *gin.Context) {
	appID, err := h.svc.ch.ResetChannelAppID(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, map[string]interface{}{
		"app_id":     appID,
		"access_url": h.svc.ch.AccessURLByAppID(authctx.TenantID(c), httpx.UintParam(c, "id"), ""),
	})
}

// ---------------- 二维码 / 接入地址 ----------------

// ChannelQRPNGHandler 返回 PNG 图片本身，便于前端直接下载（GET /api/channels/:id/qr.png?size=320）
func (h *Handler) ChannelQRPNGHandler(c *gin.Context) {
	vo, err := h.svc.ch.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
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
func (h *Handler) ChannelQRInfoHandler(c *gin.Context) {
	vo, err := h.svc.ch.ChannelQR(authctx.TenantID(c), httpx.UintParam(c, "id"),
		c.Query("base"), httpx.QueryInt(c, "size", 320))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

// ---------------- 渠道接待账号快速设置 ----------------

func (h *Handler) ListChannelAgentsHandler(c *gin.Context) {
	list, err := h.svc.ch.ListChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"))
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) SetChannelAgentsHandler(c *gin.Context) {
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
	list, err := h.svc.ch.SetChannelAgents(authctx.TenantID(c), httpx.UintParam(c, "id"), req.UserIDs)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, list)
}

// ---------------- 渠道类型库（可自定义，不限制枚举） ----------------

func (h *Handler) ListChannelTypesHandler(c *gin.Context) {
	list, err := h.svc.ch.ListChannelTypes(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateChannelTypeHandler(c *gin.Context) {
	var req channel.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.ch.CreateChannelType(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateChannelTypeHandler(c *gin.Context) {
	var req channel.ChannelTypeRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.ch.UpdateChannelType(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteChannelTypeHandler(c *gin.Context) {
	if err := h.svc.ch.DeleteChannelType(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 子账号 ----------------

// ListAgentsHandler 客服列表。
//   - admin：看全部
//   - agent：带 ?scope=transfer 时可看「与自己接待渠道有交集」的在线客服（用于会话转接）
func (h *Handler) ListAgentsHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) && c.Query("scope") != "transfer" {
		response.Forbid(c, "仅管理员可查看")
		return
	}
	list, err := h.svc.acc.ListAgents(authctx.TenantID(c))
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

func (h *Handler) CreateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.acc.CreateAgent(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req dto.AgentRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.acc.UpdateAgent(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteAgentHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.acc.DeleteAgent(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) SetOnlineHandler(c *gin.Context) {
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
	if err := h.svc.acc.SetOnline(authctx.TenantID(c), target, body.Online); err != nil {
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

func (h *Handler) ListCustomersHandler(c *gin.Context) {
	paged, err := h.svc.cust.ListCustomers(authctx.TenantID(c), c.Query("keyword"), httpx.QueryUint(c, "channel_id"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "page_size", 20))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, paged)
}

// ---------------- 客户标签 ----------------

func (h *Handler) ListTagsHandler(c *gin.Context) {
	list, err := h.svc.cust.ListTags(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateTagHandler(c *gin.Context) {
	var req customer.TagRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.cust.CreateTag(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) DeleteTagHandler(c *gin.Context) {
	if err := h.svc.cust.DeleteTag(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) UpdateCustomerRemarkHandler(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.cust.UpdateCustomerRemark(authctx.TenantID(c), httpx.UintParam(c, "id"), req.Remark); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 自动回复规则（关键词 → 机器人回复 / 转人工） ----------------

func (h *Handler) ListAutoRepliesHandler(c *gin.Context) {
	list, err := h.svc.auto.ListAutoReplies(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

func (h *Handler) CreateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	vo, err := h.svc.auto.CreateAutoReply(authctx.TenantID(c), req)
	if err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, vo)
}

func (h *Handler) UpdateAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	var req automation.AutoReplyRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.auto.UpdateAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id"), req); err != nil {
		response.Param(c, err.Error())
		return
	}
	response.OK(c, true)
}

func (h *Handler) DeleteAutoReplyHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Forbid(c, "仅管理员可操作")
		return
	}
	if err := h.svc.auto.DeleteAutoReply(authctx.TenantID(c), httpx.UintParam(c, "id")); err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, true)
}

// ---------------- 系统配置（默认图片等） ----------------
// 实现完全落在 platform/setting（account 的登录响应也要用），本模块只做 HTTP 适配。

// ListSettingsHandler 读取全部配置项
func (h *Handler) ListSettingsHandler(c *gin.Context) {
	list, err := setting.ListSettings(authctx.TenantID(c))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}

// SaveSettingHandler 保存单个配置项
func (h *Handler) SaveSettingHandler(c *gin.Context) {
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

func (h *Handler) StatsOverviewHandler(c *gin.Context) {
	days := httpx.QueryInt(c, "days", 7)
	data, err := h.svc.StatsOverviewData(authctx.TenantID(c), days)
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, data)
}

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
	response.OK(c, map[string]interface{}{"sent": true, "title": body.Title})
}

// SearchConversationRecordsHandler 全局查询对话记录（管理后台）
// GET /api/conversation-records?keyword=&channel_id=&status=&agent_id=&start=&end=&page=&page_size=

// ---------------- 对话记录（全局检索 / 只读回放） ----------------

func (h *Handler) SearchConversationRecordsHandler(c *gin.Context) {
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
	list, total, err := h.svc.conv.SearchConversations(authctx.TenantID(c), q)
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
func (h *Handler) GetConversationMessagesHandler(c *gin.Context) {
	list, err := h.svc.conv.ListMessages(authctx.TenantID(c), httpx.UintParam(c, "id"), 0, httpx.QueryInt(c, "limit", 200))
	if err != nil {
		response.Server(c, err.Error())
		return
	}
	response.OK(c, list)
}
