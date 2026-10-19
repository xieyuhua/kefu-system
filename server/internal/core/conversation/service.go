// Package conversation 会话内核：conversations、messages、WebSocket 实时通道。
// 模块内按 service / repository 分层，ws.go 放 WebSocket 升级器等共用件，
// HTTP 层在各端模块（客服端 modules/agent、用户端 modules/client）。
//
// 依赖方向：core/conversation → core/{account,channel,customer} → platform → pkg。
// 内核层内不允许出现环：本包可以依赖上述三个内核包，反之不行。
// 跨内核调用约定：
//   - 业务编排走对方导出的 Service 方法（如 s.customer.GetCustomerTags）；
//   - 组装 VO 所需的单表读取同样走对方的 Service 门面（s.customer.FindCustomerByID /
//     s.customer.CountOrdersOfCustomer / s.channel.FindChannelByID / s.channel.HasAgentChannel /
//     s.account.FindUserBriefByID / s.account.FindUserByID / s.account.CountOpenConversations），
//     这些方法只是一次主键查询，SQL 仍留在对方 repository 里；
//   - 唯一一处跨内核写入是「刷新客户最近活跃时间」（s.customer.UpdateCustomerColumns），
//     消息落库与会话摘要必须保持原有先后顺序，因此就近调用对方 service 门面。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），跨内核依赖在构造时建好；
// 只有无状态的纯函数（messagePreview / IsCustomerOnline / orDefault）保留为包级函数。
package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"kefu-server/internal/core/account"
	"kefu-server/internal/core/channel"
	"kefu-server/internal/core/customer"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/ipinfo"
	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/realtime"
	"kefu-server/pkg/response"
	"kefu-server/pkg/util"
)

// Service 会话内核的业务编排层，持有本模块仓储与三个跨内核依赖。
type Service struct {
	repo     *Repository
	account  *account.Service
	channel  *channel.Service
	customer *customer.Service
}

// NewService 创建业务层实例（自带数据访问层与跨内核依赖）。
func NewService() *Service {
	return &Service{
		repo:     NewRepository(),
		account:  account.NewService(),
		channel:  channel.NewService(),
		customer: customer.NewService(),
	}
}

// ---------------- 基础设施层回调注册 ----------------
// platform 下的包不反向依赖业务内核，涉及业务逻辑的部分在这里注册回调：
//   - realtime：客服最后一个连接断开 → 释放其接待中的会话
// 注册点落在会话内核的 init 里（route 会 import 本包，保证注册被执行）。
// 另一个回调 authctx.SetUserLoader 归属账号内核，见 core/account/service.go。

func init() {
	realtime.SetAgentOfflineHook(NewService().releaseAgentConversations)
}

// releaseAgentConversations 把某客服正在接待的会话全部置回「待接待」。
// 用于客服（含管理员）离线后自动释放，避免会话被已离线的人长期占用。
func (s *Service) releaseAgentConversations(tenantID, agentID uint) {
	convs, err := s.repo.ListOpenConversationsByAgent(tenantID, agentID)
	if err != nil || len(convs) == 0 {
		return
	}
	now := time.Now()
	for _, c := range convs {
		c.AgentID = 0
		c.Status = "pending"
		c.LastMessageAt = now
		_ = s.repo.SaveConversation(c)
		s.addSystemMessage(tenantID, c.ID, s.agentName(agentID)+" 已离线，会话回到待接待")
		vo := s.buildConversationVO(c)
		realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.updated", Data: vo})
		if m := s.lastSystemMessage(tenantID, c.ID); m != nil {
			realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "message.new", Data: *m})
		}
	}
	log.Printf("[hub] 客服 #%d 离线，释放 %d 个会话回待接待", agentID, len(convs))
}

// ---------------- Conversation ----------------

type ConvQuery struct {
	ChannelID       uint
	Status          string
	Scope           string
	Keyword         string
	Page            int
	PageSize        int
	IncludeArchived bool // 是否包含已归档会话
}

func (s *Service) ListConversations(tenantID, userID uint, isAdmin bool, q ConvQuery) (*response.Paged, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	// 可见范围（scope=mine / 管理员全量 / 普通客服看自己接待的 + 自己渠道待接待的）
	// 与默认排除已归档的过滤都在 repository 内完成，保证与统计口径一致。
	list, total, err := s.repo.QueryConversations(tenantID, userID, isAdmin, ConversationFilter{
		ChannelID:       q.ChannelID,
		Status:          q.Status,
		Scope:           q.Scope,
		Keyword:         q.Keyword,
		IncludeArchived: q.IncludeArchived,
		Page:            q.Page,
		PageSize:        q.PageSize,
	})
	if err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		vos = append(vos, s.buildConversationVO(c))
	}
	return &response.Paged{List: vos, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) GetConversation(tenantID, id uint) (*dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	vo := s.buildConversationVO(c)
	return &vo, nil
}

func (s *Service) buildConversationVO(c *model.Conversation) dto.ConversationVO {
	vo := dto.ConversationVO{
		ID: c.ID, ChannelID: c.ChannelID, Status: c.Status,
		Unread: c.Unread, LastMessage: c.LastMessage,
		LastMessageAt: util.FmtTime(c.LastMessageAt), CreatedAt: util.FmtTime(c.CreatedAt),
		ClosedAt: util.FmtTimePtr(c.ClosedAt), MessageCount: c.MessageCount, Summary: c.Summary,
		Archived: c.Archived, ArchivedAt: util.FmtTimePtr(c.ArchivedAt),
	}
	// 渠道 / 客户 / 客服的展示信息查不到时留空，不阻断 VO 组装（与原逻辑一致）
	if ch, err := s.channel.FindChannelByID(c.TenantID, c.ChannelID); err == nil {
		vo.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}
	if cu, err := s.customer.FindCustomerByID(c.CustomerID); err == nil {
		oc, _ := s.customer.CountOrdersOfCustomer(cu.ID)
		vo.User = &dto.CustomerRef{
			ID: cu.ID, ExternalID: cu.ExternalID, Nickname: cu.Nickname,
			Avatar: cu.Avatar, Phone: cu.Phone, OrderCount: oc, ChannelID: cu.ChannelID,
			Tags:   s.customer.GetCustomerTags(c.TenantID, cu.ID),
			Remark: cu.Remark,
			IP:     cu.IP,
			City:   ipinfo.CityOrLookup(cu),
			// 客户在线状态：按最近活跃时间推断（2 分钟内活跃即在线）
			Online:     IsCustomerOnline(cu),
			LastSeenAt: util.FmtTimePtr(cu.LastSeenAt),
		}
		vo.OrderCount = oc
	}
	if c.AgentID > 0 {
		if u, err := s.account.FindUserBriefByID(c.AgentID); err == nil {
			vo.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role, Online: u.Online}
		}
	}
	return vo
}

// isAdminUser 判断用户是否为管理员（管理员享有接管/查看全部会话的特权）
func (s *Service) isAdminUser(userID uint) bool {
	if userID == 0 {
		return false
	}
	u, err := s.account.FindUserBriefByID(userID)
	if err != nil {
		return false
	}
	return u.Role == "admin"
}

// canServeChannel 判断客服是否配置了接待该渠道。
func (s *Service) canServeChannel(userID, channelID uint) bool {
	ok, err := s.channel.HasAgentChannel(userID, channelID)
	return err == nil && ok
}

// countOpenConversations 统计某客服当前进行中的会话数（status=open）。
func (s *Service) countOpenConversations(tenantID, agentID uint) int64 {
	n, _ := s.account.CountOpenConversations(tenantID, agentID)
	return n
}

// checkConcurrentLimit 校验客服是否已达「最大同时接待数」上限。
// 返回 nil 表示可继续接待；返回 error 表示已满。
// 规则：MaxConcurrent <= 0 视为不限；查不到用户时放行（避免误伤）。
func (s *Service) checkConcurrentLimit(tenantID, agentID uint) error {
	u, err := s.account.FindUserBriefByID(agentID)
	if err != nil {
		return nil
	}
	if u.MaxConcurrent <= 0 {
		return nil
	}
	if active := s.countOpenConversations(tenantID, agentID); active >= int64(u.MaxConcurrent) {
		return fmt.Errorf("已达最大同时接待数（%d），请先处理现有会话", u.MaxConcurrent)
	}
	return nil
}

func (s *Service) ClaimConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.Status == "closed" {
		return nil, errors.New("会话已结束")
	}
	// 渠道绑定校验：该渠道只接待账号限制了名单时，未绑定的客服不能接。
	// 管理员不受限制（可接待任意渠道）。
	if !s.isAdminUser(agentID) && !s.canServeChannel(agentID, c.ChannelID) {
		return nil, errors.New("您未配置接待该渠道")
	}
	// 互斥接待：已被其他客服领取的会话不能再领。
	// 管理员例外——可以接管任何会话（含其他客服正在接待的）。
	if c.AgentID != 0 && c.AgentID != agentID && !s.isAdminUser(agentID) {
		return nil, errors.New("会话已被其他客服接待")
	}
	// 并发上限校验：仅当本次领取会让该客服的 open 会话数 +1 时才校验。
	// 自己已在 open 接待的会话重复领取属幂等操作，不拦截。
	if !(c.AgentID == agentID && c.Status == "open") {
		if err := s.checkConcurrentLimit(tenantID, agentID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	c.AgentID = agentID
	c.Status = "open"
	c.LastMessageAt = now
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, err
	}
	s.addSystemMessage(tenantID, c.ID, s.agentName(agentID)+" 已接入会话")
	vo := s.buildConversationVO(c)
	// 通知用户端：已有人工客服接入
	if m := s.lastSystemMessage(tenantID, c.ID); m != nil {
		realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "message.new", Data: *m})
	}
	realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

// lastSystemMessage 取会话最后一条 system 消息（VO）
func (s *Service) lastSystemMessage(tenantID, convID uint) *dto.MessageVO {
	m, err := s.repo.FindLastSystemMessage(tenantID, convID)
	if err != nil {
		return nil
	}
	vo := s.buildMessageVO(m)
	return &vo
}

func (s *Service) TransferConversation(tenantID, id, fromAgentID, toAgentID uint) (*dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.AgentID != fromAgentID {
		return nil, errors.New("只能转接自己接待的会话")
	}
	target, err := s.account.FindUserByID(tenantID, toAgentID)
	if err != nil {
		return nil, errors.New("目标客服不存在")
	}
	// 转接目标必须配置了接待该渠道，否则转过去他根本看不到这个会话
	if !s.isAdminUser(toAgentID) && !s.canServeChannel(toAgentID, c.ChannelID) {
		return nil, errors.New("该客服未配置接待此渠道，无法转接")
	}
	// 目标客服并发上限校验（转给自己则无变化，跳过）
	if toAgentID != fromAgentID {
		if err := s.checkConcurrentLimit(tenantID, toAgentID); err != nil {
			return nil, errors.New("转接失败：" + err.Error())
		}
	}
	c.AgentID = toAgentID
	c.Status = "open"
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, err
	}
	s.addSystemMessage(tenantID, c.ID, fmt.Sprintf("会话已转接给 %s", target.Name))
	realtime.Notify(tenantID, toAgentID, "conversation.transferred", "会话转接", "有一条会话被转接给你", c.ID)
	vo := s.buildConversationVO(c)
	if m := s.lastSystemMessage(tenantID, c.ID); m != nil {
		realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "message.new", Data: *m})
	}
	return &vo, nil
}

func (s *Service) CloseConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, err
	}
	s.addSystemMessage(tenantID, c.ID, "会话已结束，感谢您的咨询")
	vo := s.buildConversationVO(c)
	if m := s.lastSystemMessage(tenantID, c.ID); m != nil {
		realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "message.new", Data: *m})
	}
	realtime.HubInstance().PublishToGuest(tenantID, c.ID, realtime.Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

func (s *Service) MarkRead(tenantID, id uint) error {
	// 会话未读清零 + 消息置读；失败不阻断（与原逻辑一致，下次打开会话会再次置读）
	_ = s.repo.ClearConversationUnread(tenantID, id)
	_ = s.repo.MarkMessagesRead(tenantID, id)
	return nil
}

func (s *Service) ConversationStats(tenantID, userID uint, isAdmin bool) (*dto.StatsVO, error) {
	st := &dto.StatsVO{}

	// 统计口径必须与 ListConversations 完全一致：同一套可见范围 + 默认排除已归档会话。
	// 否则会出现「侧边栏显示未接待 1，点进去却是空的」——归档会话被算进了统计，
	// 但列表接口过滤掉了 archived = true 的记录。
	// 三项计数各自新建查询（repository 内部每次独立构建条件），避免 GORM 条件互相污染。
	st.Pending, _ = s.repo.CountVisibleConversations(tenantID, userID, isAdmin, "pending")
	st.Open, _ = s.repo.CountVisibleConversations(tenantID, userID, isAdmin, "open")
	st.Closed, _ = s.repo.CountVisibleConversations(tenantID, userID, isAdmin, "closed")
	// 未读消息总数：把各会话的 unread 求和（会话 unread 字段 = 该会话未读消息条数）
	st.Unread, _ = s.repo.SumVisibleConversationUnread(tenantID, userID, isAdmin)
	return st, nil
}

// FindOrCreateConversation 定位或创建会话。
//
// 核心约定：同一个 uid + 同一渠道 = 同一个客户，同一个客户 = **始终复用最近一个会话**。
// 即使该会话已「结束」（closed）或被「归档」（archived），也会自动重新激活为「待接待」，
// 从而保证聊天记录**自然合并**，不会因为客服关闭 / 归档而分裂成多个会话。
//
// 供 GuestInit / GuestSend / WebSocket 访客连接三处统一使用，避免各自写不一致的过滤条件。
func (s *Service) FindOrCreateConversation(tenantID, customerID, channelID uint) (*model.Conversation, error) {
	if conv, err := s.repo.FindLatestConversationByCustomer(tenantID, customerID); err == nil {
		// 复用最近一个会话；已关闭 / 已归档的重新激活
		needSave := false
		if conv.Status == "closed" {
			conv.Status = "pending"
			conv.ClosedAt = nil
			needSave = true
		}
		if conv.Archived {
			conv.Archived = false
			conv.ArchivedAt = nil
			conv.Status = "pending"
			needSave = true
		}
		if needSave {
			if err := s.repo.SaveConversation(conv); err != nil {
				return nil, err
			}
		}
		return conv, nil
	}

	// 首次咨询：新建会话
	now := time.Now()
	conv := &model.Conversation{
		TenantID: tenantID, ChannelID: channelID, CustomerID: customerID,
		Status: "pending", LastMessageAt: now,
	}
	if err := s.repo.CreateConversation(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// ---------------- 会话归档 / 删除 ----------------

// ArchiveConversation 归档会话（客服主动清理工作台列表）。
// 仅隐藏，不删除数据；客户再次发消息会自动恢复（见 ReopenConversation）。
func (s *Service) ArchiveConversation(tenantID, convID, operatorID uint) error {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return errors.New("会话不存在")
	}
	now := time.Now()
	updates := map[string]interface{}{
		"archived":    true,
		"archived_at": &now,
		"unread":      0,
	}
	// 归档时若该客服不再接待，释放归属（重新进入待接待池）
	if c.AgentID == operatorID {
		updates["agent_id"] = 0
		updates["status"] = "pending"
	}
	return s.repo.UpdateConversationByID(c.ID, updates)
}

// ArchiveConversationsBulk 批量归档会话（一键清空工作台列表）。
// 复用单条归档语义：仅隐藏不删数据，客户再次咨询会自动恢复。
// q 使用与列表查询完全相同的过滤条件，保证「所见即所清」。
func (s *Service) ArchiveConversationsBulk(tenantID, userID uint, isAdmin bool, q ConvQuery) (int, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	// 清空时不按分页截断，但设上限防止误操作拖垮服务
	q.PageSize = 500

	paged, err := s.ListConversations(tenantID, userID, isAdmin, q)
	if err != nil {
		return 0, err
	}
	// ListConversations 返回的 List 是 []interface{}，逐项取出会话 ID。
	items, ok := paged.List.([]interface{})
	if !ok || len(items) == 0 {
		return 0, nil
	}

	ids := make([]uint, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case map[string]interface{}:
			// VO 以 map 形态进入（JSON 往返）
			if f, ok := v["id"].(float64); ok {
				ids = append(ids, uint(f))
			}
		case dto.ConversationVO:
			ids = append(ids, v.ID)
		}
	}
	if len(ids) == 0 {
		log.Printf("[bulk] 未从列表解析出会话 id，items=%d", len(items))
		return 0, nil
	}

	now := time.Now()
	// 先查出归属，用于判断哪些需要释放回待接待池
	rows, err := s.repo.ListConversationAgentRefs(tenantID, ids)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, c := range rows {
		updates := map[string]interface{}{
			"archived":    true,
			"archived_at": &now,
			"unread":      0,
		}
		// 归档时若该客服不再接待，释放归属（重新进入待接待池）
		if c.AgentID == userID {
			updates["agent_id"] = 0
			updates["status"] = "pending"
		}
		if err := s.repo.UpdateConversationByID(c.ID, updates); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// UnarchiveConversation 取消归档，恢复到工作台列表
func (s *Service) UnarchiveConversation(tenantID, convID uint) error {
	return s.repo.SetConversationUnarchived(tenantID, convID)
}

// DeleteConversation 彻底删除会话及其消息（不可恢复）
func (s *Service) DeleteConversation(tenantID, convID uint) error {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return errors.New("会话不存在")
	}
	return s.repo.DeleteConversationWithMessages(tenantID, c.ID)
}

// ReopenConversation 客户重新发起咨询时调用：
// 若会话被归档，则取消归档并回到「未接待」状态（消息不丢）。
func (s *Service) ReopenConversation(tenantID, convID uint) {
	// 仅命中已归档的会话，重复调用无副作用；失败不阻断（与原逻辑一致）
	_ = s.repo.ReopenArchivedConversation(tenantID, convID)
}

// ---------------- 客户在线状态（活跃度推断） ----------------

// onlineWindow 活跃判定窗口：超过此时长无活动视为离线（微信式）
const onlineWindow = 2 * time.Minute

// IsCustomerOnline 按最近活跃时间推断客户是否在线
func IsCustomerOnline(c *model.Customer) bool {
	if c.LastSeenAt == nil {
		return false
	}
	return time.Since(*c.LastSeenAt) <= onlineWindow
}

// ---------------- 消息搜索 ----------------

// SearchMessages 在指定会话内按内容关键词搜索消息，返回升序结果
func (s *Service) SearchMessages(tenantID, convID uint, keyword string, limit int) ([]*model.Message, error) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, errors.New("请输入搜索关键词")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// repository 内部按 id 倒序取最近 limit 条后反转为升序，与消息流顺序一致
	return s.repo.SearchMessagesByContent(tenantID, convID, kw, limit)
}

// ---------------- 全局对话记录查询（管理后台） ----------------

// ConversationRecord 对话记录查询项
type ConversationRecord struct {
	ID             uint   `json:"id"`
	CustomerID     uint   `json:"customer_id"`
	CustomerName   string `json:"customer_name"`
	CustomerAvatar string `json:"customer_avatar"`
	ChannelID      uint   `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	ChannelType    string `json:"channel_type"`
	AgentID        uint   `json:"agent_id"`
	AgentName      string `json:"agent_name"`
	Status         string `json:"status"`
	Archived       bool   `json:"archived"`
	MessageCount   int    `json:"message_count"`
	LastMessage    string `json:"last_message"`
	LastMessageAt  string `json:"last_message_at"`
	CreatedAt      string `json:"created_at"`
	FirstMessage   string `json:"first_message"`
	FirstMessageAt string `json:"first_message_at"`
}

// ConversationRecordsQuery 对话记录查询条件
type ConversationRecordsQuery struct {
	Keyword   string // 搜昵称 / 消息内容
	ChannelID uint
	Status    string
	AgentID   uint
	Start     string // YYYY-MM-DD
	End       string // YYYY-MM-DD
	Page      int
	PageSize  int
}

// SearchConversations 全局查询对话记录（支持按消息内容全文匹配）
func (s *Service) SearchConversations(tenantID uint, q ConversationRecordsQuery) ([]ConversationRecord, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 100 {
		q.PageSize = 20
	}
	f := ConversationRecordFilter{
		Keyword:   q.Keyword,
		ChannelID: q.ChannelID,
		Status:    q.Status,
		AgentID:   q.AgentID,
		Page:      q.Page,
		PageSize:  q.PageSize,
	}
	// 日期按天解析：起始含当天零点，结束取次日零点做开区间，避免漏掉当天最后一秒
	if q.Start != "" {
		if t, err := time.Parse("2006-01-02", q.Start); err == nil {
			f.From = &t
		}
	}
	if q.End != "" {
		if t, err := time.Parse("2006-01-02", q.End); err == nil {
			next := t.AddDate(0, 0, 1)
			f.To = &next
		}
	}

	list, total, err := s.repo.SearchConversationRecords(tenantID, f)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ConversationRecord, 0, len(list))
	for _, c := range list {
		rec := ConversationRecord{
			ID: c.ID, CustomerID: c.CustomerID, ChannelID: c.ChannelID,
			AgentID: c.AgentID, Status: c.Status, Archived: c.Archived,
			MessageCount: c.MessageCount, LastMessage: c.LastMessage,
			LastMessageAt: util.FmtTime(c.LastMessageAt), CreatedAt: util.FmtTime(c.CreatedAt),
			FirstMessageAt: "",
		}
		if cu, err := s.customer.FindCustomerByID(c.CustomerID); err == nil {
			rec.CustomerName = cu.Nickname
			rec.CustomerAvatar = cu.Avatar
		}
		if ch, err := s.channel.FindChannelByID(c.TenantID, c.ChannelID); err == nil {
			rec.ChannelName = ch.Name
			rec.ChannelType = ch.Type
		}
		rec.AgentName = "未分配"
		if c.AgentID > 0 {
			if u, err := s.account.FindUserBriefByID(c.AgentID); err == nil {
				rec.AgentName = u.Name
			}
		}
		// 首条消息（列表里预览对话开头）
		if first, err := s.repo.FindFirstUserMessage(c.ID); err == nil {
			rec.FirstMessage = messagePreview(first)
			rec.FirstMessageAt = util.FmtTime(first.CreatedAt)
		}
		out = append(out, rec)
	}
	return out, total, nil
}

// ---------------- Message ----------------

// ListMessagesSince 正向增量拉取：返回 id > afterID 的消息（按 id 升序）。
// 供用户端 / APP 离线同步使用 —— 与 ListMessages 的「向前翻页」语义相反，勿混用。
func (s *Service) ListMessagesSince(tenantID, convID uint, afterID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	list, err := s.repo.ListMessagesAfter(tenantID, convID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, s.buildMessageVO(m))
	}
	return out, nil
}

// ListMessages 分页向前翻页：返回 id < beforeID 的消息（按 id 升序）
func (s *Service) ListMessages(tenantID, convID uint, beforeID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 {
		limit = 50
	}
	list, err := s.repo.ListMessagesBefore(tenantID, convID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, s.buildMessageVO(m))
	}
	return out, nil
}

// BuildMessageVO 导出方法，供其他模块（如消息搜索、离线同步）复用
func (s *Service) BuildMessageVO(m *model.Message) dto.MessageVO { return s.buildMessageVO(m) }

func (s *Service) buildMessageVO(m *model.Message) dto.MessageVO {
	vo := dto.MessageVO{
		ID: m.ID, ConversationID: m.ConversationID, Type: m.Type,
		Direction: m.Direction, Content: m.Content,
		CreatedAt: util.FmtTime(m.CreatedAt),
	}
	switch m.SenderRole {
	case "user":
		u, err := s.account.FindUserBriefByID(m.SenderID)
		if m.SenderID > 0 && err == nil {
			vo.Sender = dto.SenderRef{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: "agent"}
		} else {
			vo.Sender = dto.SenderRef{Name: "智能助手", Role: "robot"}
		}
	case "customer":
		if c, err := s.customer.FindCustomerByID(m.SenderID); err == nil {
			vo.Sender = dto.SenderRef{ID: c.ID, Name: c.Nickname, Avatar: c.Avatar, Role: "user"}
		}
	default:
		vo.Sender = dto.SenderRef{Role: "system"}
	}
	return vo
}

// SendMessage 客服发送消息
func (s *Service) SendMessage(tenantID, convID, senderID uint, req dto.SendMessageRequest) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Content == "" {
		return nil, nil, errors.New("消息内容不能为空")
	}
	// 卡片与媒体消息的 content 必须是合法 JSON（{url,kind,name,size}）
	if req.Type == "order" || req.Type == "product" || req.Type == "image" ||
		req.Type == "video" || req.Type == "file" {
		var probe map[string]interface{}
		if err := json.Unmarshal([]byte(req.Content), &probe); err != nil {
			return nil, nil, errors.New("消息内容格式不正确（需为合法 JSON）")
		}
	}
	if c.AgentID == 0 {
		// 未分配时由发送者接管；必须先确认他有权接待该渠道，
		// 否则未绑定的客服也能通过直接发消息「抢」走别人的待接待会话。
		if !s.isAdminUser(senderID) && !s.canServeChannel(senderID, c.ChannelID) {
			return nil, nil, errors.New("您未配置接待该渠道")
		}
		// 自动接管也受「最大同时接待数」限制
		if err := s.checkConcurrentLimit(tenantID, senderID); err != nil {
			return nil, nil, err
		}
		c.AgentID = senderID
		c.Status = "open"
	} else if c.AgentID != senderID && !s.isAdminUser(senderID) {
		// 会话已被他人接待，禁止越权发言
		return nil, nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       senderID,
		SenderRole:     "user",
		Type:           req.Type,
		Direction:      "out",
		Content:        req.Content,
		Read:           true,
	}
	if err := s.repo.CreateMessage(m); err != nil {
		return nil, nil, err
	}
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	mvo := s.buildMessageVO(m)
	cvo := s.buildConversationVO(c)
	return &mvo, &cvo, nil
}

func messagePreview(m *model.Message) string {
	switch m.Type {
	case "image":
		return "[图片]"
	case "video":
		return "[视频]"
	case "file":
		return "[文件]"
	case "order":
		return "[订单]"
	case "product":
		return "[商品]"
	case "system":
		return m.Content
	default:
		return m.Content
	}
}

func (s *Service) addSystemMessage(tenantID, convID uint, content string) *model.Message {
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderRole:     "system",
		Type:           "system",
		Direction:      "system",
		Content:        content,
		Read:           true,
	}
	// 落库失败不阻断主流程（与原逻辑一致）：系统提示语缺失不影响会话状态流转
	_ = s.repo.AddSystemMessage(m)
	return m
}

// AppendSystemMessage 导出方法：给其它模块（automation 的自动回复转人工）落系统提示语。
// 内部实现仍是 addSystemMessage，保持「落库失败不阻断」的原语义。
func (s *Service) AppendSystemMessage(tenantID, convID uint, content string) *model.Message {
	return s.addSystemMessage(tenantID, convID, content)
}

// ReceiveMessage 客户侧发消息（演示入口 / 渠道回调共用）
func (s *Service) ReceiveMessage(tenantID, convID, customerID uint, msgType, content string) (*dto.MessageVO, *dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if msgType == "" {
		msgType = "text"
	}
	m := &model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       customerID,
		SenderRole:     "customer",
		Type:           msgType,
		Direction:      "in",
		Content:        content,
	}
	if err := s.repo.CreateMessage(m); err != nil {
		return nil, nil, err
	}
	now := time.Now()
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	c.Unread++
	if c.Status == "closed" {
		c.Status = "pending"
		c.ClosedAt = nil
	}
	// 客户重新咨询时，若会话曾被客服归档 → 自动恢复，重新出现在工作台待接待列表
	if c.Archived {
		c.Archived = false
		c.ArchivedAt = nil
		if c.Status == "closed" {
			c.Status = "pending"
			c.ClosedAt = nil
		}
		// 归档释放过客服归属，这里保持 agent_id = 0 进入待接待池
		if c.AgentID == 0 {
			c.Status = "pending"
		}
	}
	if err := s.repo.SaveConversation(c); err != nil {
		return nil, nil, err
	}
	// 活跃时间刷新失败不阻断发消息（与原逻辑一致）
	_ = s.customer.UpdateCustomerColumns(customerID, map[string]interface{}{"last_seen_at": now})

	mvo := s.buildMessageVO(m)
	cvo := s.buildConversationVO(c)
	return &mvo, &cvo, nil
}

func (s *Service) agentName(id uint) string {
	if u, err := s.account.FindUserBriefByID(id); err == nil && u.Name != "" {
		return u.Name
	}
	return "客服"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// ---------------- 供其它内核复用的通知辅助 ----------------

// NotifyChannelAgents 通知该渠道下所有在线客服（转人工场景）。
// core/automation 的 ReplyNow 需要它，而内核层不允许 automation 反向依赖端模块，
// 故留在会话内核（本包已具备渠道 / 账号读取能力）。
func (s *Service) NotifyChannelAgents(tenantID, channelID, convID uint, title, body string) {
	acs, err := s.channel.ListAgentChannelsByChannel(channelID)
	if err != nil {
		return
	}
	for _, ac := range acs {
		u, err := s.account.FindUserBriefByID(ac.UserID)
		if err != nil {
			continue
		}
		realtime.Notify(tenantID, u.ID, "conversation.pending", title, body, convID)
	}
}

// BroadcastConversation 把会话最新状态推给客服端与用户端（原 service/demo.go 的 broadcastConversation）。
func (s *Service) BroadcastConversation(tenantID, convID uint) {
	c, err := s.repo.FindConversationByID(tenantID, convID)
	if err != nil {
		return
	}
	vo := s.buildConversationVO(c)
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "conversation.updated", Data: vo})
	realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{Event: "conversation.updated", Data: vo})
}

// ---------------- 供其它内核 / 端复用的只读门面 ----------------
// 端模块（modules/client、modules/agent）与 automation 需要「按主键取一条会话」这类单表读取，
// 跨包只能走 Service：这里的每个方法都只是把调用转给本包 repository，SQL 与原来逐字一致。

// FindConversationByID 按租户 + 主键读取会话（跨包只读门面）。
func (s *Service) FindConversationByID(tenantID, id uint) (*model.Conversation, error) {
	return s.repo.FindConversationByID(tenantID, id)
}

// FindLatestConversationByCustomer 取某客户最近一条会话（跨包只读门面）。
func (s *Service) FindLatestConversationByCustomer(tenantID, customerID uint) (*model.Conversation, error) {
	return s.repo.FindLatestConversationByCustomer(tenantID, customerID)
}

// CreateConversation 新增会话（跨包写入门面，供演示入口新建会话）。
func (s *Service) CreateConversation(c *model.Conversation) error {
	return s.repo.CreateConversation(c)
}
