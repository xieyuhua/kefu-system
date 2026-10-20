package service

// ---------------- 会话域（Conversation） ----------------
// conversations、messages、会话可见范围与实时推送；ws.go 放 WebSocket 升级器等共用件。
//
// 跨域调用约定（同包内直接持有对方 Service，不再有 import 关系）：
//   - 业务编排走对方的 Service 方法（如 s.customer.GetCustomerTags）；
//   - 组装 VO 所需的单表读取同样走对方 Service 门面（s.customer.FindCustomerByID /
//     s.customer.CountOrdersOfCustomer / s.channel.FindChannelByID / s.channel.HasAgentChannel /
//     s.account.FindUserBriefByID / s.account.FindUserByID / s.account.CountOpenConversations），
//     这些方法只是一次主键查询，SQL 仍留在对方仓储里；
//   - 唯一一处跨域写入是「刷新客户最近活跃时间」（s.customer.UpdateCustomerColumns），
//     消息落库与会话摘要必须保持原有先后顺序，因此就近调用对方 Service 门面。
//
// 形态约定：业务编排都是 ConversationService 的方法（指针接收者），依赖在构造时建好；
// 只有无状态的纯函数（messagePreview / IsCustomerOnline）保留为包级函数。
import (
	"errors"
	"fmt"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/response"
	"kefu-server/pkg/util"
)

// ConversationService 会话内核的业务编排层，持有本模块仓储与三个跨内核依赖。
// conversations / 可见范围 / messages 三张表的读写分属三个仓储（合并成一个包后按子域拆开）。
type ConversationService struct {
	repo     *repository.ConversationRepository
	vis      *repository.ConversationVisibilityRepository
	messages *repository.MessageRepository
	account  *AccountService
	channel  *ChannelService
	customer *CustomerService
}

// NewConversationService 创建业务层实例（自带数据访问层与跨内核依赖）。
func NewConversationService() *ConversationService {
	return &ConversationService{
		repo:     repository.NewConversationRepository(),
		vis:      repository.NewConversationVisibilityRepository(),
		messages: repository.NewMessageRepository(),
		account:  NewAccountService(),
		channel:  NewChannelService(),
		customer: NewCustomerService(),
	}
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

func (s *ConversationService) ListConversations(tenantID, userID uint, isAdmin bool, q ConvQuery) (*response.Paged, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	// 可见范围（scope=mine / 管理员全量 / 普通客服看自己接待的 + 自己渠道待接待的）
	// 与默认排除已归档的过滤都在 repository 内完成，保证与统计口径一致。
	list, total, err := s.vis.QueryConversations(tenantID, userID, isAdmin, repository.ConversationFilter{
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
	// 「未回复」判定要看每个会话的最后一条消息：整页批量取一次（1 条聚合 SQL），
	// 而不是在 VO 组装里逐会话查（N+1）。查询失败只影响标记，列表照常返回。
	ids := make([]uint, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	lasts, err := s.messages.FindLastMessagesOfConversations(tenantID, ids)
	if err != nil {
		lasts = nil
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		vo := s.assembleConversationVO(c)
		// 归档 / 已结束的会话同样按规则填充，是否展示由前端决定
		vo.UnrepliedTS = unrepliedTS(lasts[c.ID])
		vos = append(vos, vo)
	}
	return &response.Paged{List: vos, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *ConversationService) GetConversation(tenantID, id uint) (*dto.ConversationVO, error) {
	c, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return nil, errors.New("会话不存在")
	}
	vo := s.buildConversationVO(c)
	return &vo, nil
}

// ---------------- 「未回复」判定（会话列表用） ----------------

// IsCustomerMessage 判断一条消息是否由「客户」发出（即客服收到、还需要人工回复的消息）。
//
// 判据以 Direction 为主：in = 客户→客服，out = 客服/机器人→客户，system = 系统提示。
// 为什么不只看 SenderRole：本仓库里客服自己发出的消息 SenderRole 也是 "user"
// （见 SendMessage，seed 数据同此约定），单凭 role 会把「客服已回复」误判成客户消息，
// 导致回复后列表上的「未回复」永远不消失。SenderRole 只作兜底：
// "customer" / "client" 一定是客户消息；"system" 一定是系统提示。
func IsCustomerMessage(m *model.Message) bool {
	if m == nil {
		return false
	}
	if m.SenderRole == "system" || m.Type == "system" || m.Direction == "system" {
		return false
	}
	if m.Direction == "in" {
		return true
	}
	return m.SenderRole == "customer" || m.SenderRole == "client"
}

// unrepliedTS 计算「未回复起点」：会话最后一条消息是客户发的 → 返回它的时间（Unix 毫秒）；
// 客服已回复 / 只有系统提示 / 会话还没有消息 → 0。
// 前端据 UnrepliedTS > 0 决定是否在列表项上显示「未回复 N 秒」并以此为计时起点。
func unrepliedTS(last *model.Message) int64 {
	if !IsCustomerMessage(last) {
		return 0
	}
	return util.Default().Millis(last.CreatedAt)
}

// buildConversationVO 组装单条会话 VO，并顺带判定「未回复」状态。
// 多查一次最后一条消息（单会话场景只有 1 条 SQL）；批量场景请用
// assembleConversationVO + FindLastMessagesOfConversations，避免 N+1。
func (s *ConversationService) buildConversationVO(c *model.Conversation) dto.ConversationVO {
	vo := s.assembleConversationVO(c)
	if m, err := s.messages.FindLastMessageOfConversation(c.TenantID, c.ID); err == nil {
		vo.UnrepliedTS = unrepliedTS(m)
	}
	return vo
}

// assembleConversationVO 纯组装：只补齐会话自身字段与渠道 / 客户 / 客服的展示信息，
// 不做「最后一条消息」查询（未回复状态由调用方填充）。
func (s *ConversationService) assembleConversationVO(c *model.Conversation) dto.ConversationVO {
	vo := dto.ConversationVO{
		ID: c.ID, ChannelID: c.ChannelID, Status: c.Status,
		Unread: c.Unread, LastMessage: c.LastMessage,
		LastMessageAt: util.Default().FmtTime(c.LastMessageAt), CreatedAt: util.Default().FmtTime(c.CreatedAt),
		// 会话列表的「未回复时长」用毫秒时间戳计算；字符串 last_message_at 保持不变
		LastMessageTS: util.Default().Millis(c.LastMessageAt),
		ClosedAt:      util.Default().FmtTimePtr(c.ClosedAt), MessageCount: c.MessageCount, Summary: c.Summary,
		Archived: c.Archived, ArchivedAt: util.Default().FmtTimePtr(c.ArchivedAt),
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
			City:   s.customer.CityOrLookup(cu),
			// 客户在线状态：按最近活跃时间推断（2 分钟内活跃即在线）
			Online:     IsCustomerOnline(cu),
			LastSeenAt: util.Default().FmtTimePtr(cu.LastSeenAt),
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
func (s *ConversationService) isAdminUser(userID uint) bool {
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
func (s *ConversationService) canServeChannel(userID, channelID uint) bool {
	ok, err := s.channel.HasAgentChannel(userID, channelID)
	return err == nil && ok
}

// countOpenConversations 统计某客服当前进行中的会话数（status=open）。
func (s *ConversationService) countOpenConversations(tenantID, agentID uint) int64 {
	n, _ := s.account.CountOpenConversations(tenantID, agentID)
	return n
}

// checkConcurrentLimit 校验客服是否已达「最大同时接待数」上限。
// 返回 nil 表示可继续接待；返回 error 表示已满。
// 规则：MaxConcurrent <= 0 视为不限；查不到用户时放行（避免误伤）。
func (s *ConversationService) checkConcurrentLimit(tenantID, agentID uint) error {
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

func (s *ConversationService) ClaimConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
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
func (s *ConversationService) lastSystemMessage(tenantID, convID uint) *dto.MessageVO {
	m, err := s.messages.FindLastSystemMessage(tenantID, convID)
	if err != nil {
		return nil
	}
	vo := s.buildMessageVO(m)
	return &vo
}

func (s *ConversationService) TransferConversation(tenantID, id, fromAgentID, toAgentID uint) (*dto.ConversationVO, error) {
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

func (s *ConversationService) CloseConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
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

func (s *ConversationService) MarkRead(tenantID, id uint) (bool, error) {
	conv, err := s.repo.FindConversationByID(tenantID, id)
	if err != nil {
		return false, err
	}
	// 只有「已接管（open）」的会话才允许清未读。
	// 待接待（pending）时客服只是点开看一眼，属于「看消息但不接待」，
	// 此时不能改数据，否则未读信号消失、谁在等就看不出来了；已结束（closed）同理不处理。
	if conv.Status != "open" {
		return false, nil
	}
	// 会话未读清零 + 消息置读；失败不阻断（与原逻辑一致，下次打开会话会再次置读）
	_ = s.repo.ClearConversationUnread(tenantID, id)
	_ = s.messages.MarkMessagesRead(tenantID, id)
	return true, nil
}

func (s *ConversationService) ConversationStats(tenantID, userID uint, isAdmin bool) (*dto.StatsVO, error) {
	st := &dto.StatsVO{}

	// 统计口径必须与 ListConversations 完全一致：同一套可见范围 + 默认排除已归档会话。
	// 否则会出现「侧边栏显示未接待 1，点进去却是空的」——归档会话被算进了统计，
	// 但列表接口过滤掉了 archived = true 的记录。
	// 三项计数各自新建查询（repository 内部每次独立构建条件），避免 GORM 条件互相污染。
	st.Pending, _ = s.vis.CountVisibleConversations(tenantID, userID, isAdmin, "pending")
	st.Open, _ = s.vis.CountVisibleConversations(tenantID, userID, isAdmin, "open")
	st.Closed, _ = s.vis.CountVisibleConversations(tenantID, userID, isAdmin, "closed")
	// 未读消息总数：把各会话的 unread 求和（会话 unread 字段 = 该会话未读消息条数）
	st.Unread, _ = s.vis.SumVisibleConversationUnread(tenantID, userID, isAdmin)
	return st, nil
}
