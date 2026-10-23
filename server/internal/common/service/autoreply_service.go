package service

// ---------------- 自动回复域（Automation） ----------------
// 关键词规则 → 机器人回复 / 转人工。
// 跨域调用全部走 ConversationService 的方法，不直接读写对方的表。
//
// 形态约定：业务编排都是 AutomationService 的方法（指针接收者），跨域依赖在构造时建好；
// 只有无状态的纯函数（matchKeywords / splitKeywords / defaultMatchType / orDefaultStr）保留为包级函数。
// 关键词话术不再写死在代码里：内置的那几组话术由 pkg/seed 灌成自动回复规则，后台可编辑。
import (
	"errors"
	"fmt"
	"kefu-server/pkg/setting"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/util"
)

// AutomationService 自动回复内核的业务编排层，持有本模块仓储、会话内核依赖与账号只读依赖。
type AutomationService struct {
	repo    *repository.AutoReplyRepository
	conv    *ConversationService
	users   *repository.UserRepository
	account *AccountService
	// ai AI 应答内核：未命中关键词规则时按「检索渠道自动回复内容 → 大模型 → 兜底话术」逐级降级
	ai *AIService
}

// NewAutomationService 创建业务层实例（自带数据访问层与会话内核依赖）。
func NewAutomationService() *AutomationService {
	return &AutomationService{
		repo:    repository.NewAutoReplyRepository(),
		conv:    NewConversationService(),
		users:   repository.NewUserRepository(),
		account: NewAccountService(),
		ai:      NewAIService(),
	}
}

// ---------------- 自动回复规则 ----------------
// 关键词命中 → 机器人自动回复；若该规则标记为"转人工"，则直接把会话转为待人工接待。
//
// 归属与权限（归属字段 user_id）：
//   - user_id = 0：管理员建的公共规则，全租户客服可见、可用，但只有管理员能改 / 删；
//   - user_id > 0：创建该规则的客服，只有本人（和管理员）能改 / 删。
//
// 渠道范围（多选题）：
//   - ChannelIDs 非空：规则只对列表里的渠道生效；
//   - ChannelIDs 为空：回退旧的单渠道语义（ChannelID = 0 全渠道，>0 只对该渠道生效）。
//   - 命中判定统一走纯函数 MatchChannel（见 match.go），匹配 / 转人工 / 命中计数链路不再自带渠道判断。
//
// 匹配 / 触发链路（MatchRule / ReplyNow / hit_count）除渠道判定外与改造前完全一致，归属只影响写权限。

type AutoReplyVO struct {
	ID       uint   `json:"id"`
	UserID   uint   `json:"user_id"`
	Owner    string `json:"owner"`    // 「公共」或创建人昵称，前端直接展示
	Shared   bool   `json:"shared"`   // user_id = 0：公共规则（客服只读）
	Editable bool   `json:"editable"` // 当前登录账号能否改 / 删
	Name     string `json:"name"`
	// ChannelID / ChannelName 是改造前的单渠道字段（历史兼容，0 = 全渠道）
	ChannelID   uint   `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	// ChannelIDs 多选渠道（已解析成数组，空 = 全渠道）；ChannelNames 是对应的渠道名，供前端直接展示
	ChannelIDs      []uint   `json:"channel_ids"`
	ChannelNames    []string `json:"channel_names"`
	MatchType       string   `json:"match_type"` // contain | equal | prefix
	Keywords        string   `json:"keywords"`   // 逗号分隔
	Reply           string   `json:"reply"`
	ReplyType       string   `json:"reply_type"` // text | product
	TransferToHuman bool     `json:"transfer_to_human"`
	Enabled         bool     `json:"enabled"`
	Priority        int      `json:"priority"`
	HitCount        int64    `json:"hit_count"`
	CreatedAt       string   `json:"created_at"`
}

// AutoReplyRequest 复用 dto 定义（含 HasTransfer 判定）
type AutoReplyRequest = dto.AutoReplyRequest

// ListAutoReplies 列出租户的自动回复规则（含其它客服建的：规则是团队共用的）。
// viewerID / isAdmin 只用于计算每条的 editable，不影响可见范围。
func (s *AutomationService) ListAutoReplies(tenantID, viewerID uint, isAdmin bool) ([]AutoReplyVO, error) {
	list, err := s.repo.ListAutoRepliesByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	names := make(map[uint]string, 4)
	chNames := s.channelNamesOf(list)
	out := make([]AutoReplyVO, 0, len(list))
	for _, r := range list {
		out = append(out, s.toAutoReplyVO(tenantID, r, viewerID, isAdmin, names, chNames))
	}
	return out, nil
}

// channelNamesOf 批量查规则用到的渠道名：一次查询覆盖整个列表，避免逐条回查（N+1）。
// 查询失败只影响名字回显，不阻断列表（与改造前的兜底口径一致）。
func (s *AutomationService) channelNamesOf(list []*model.AutoReply) map[uint]string {
	names := make(map[uint]string, len(list))
	ids := make([]uint, 0, len(list))
	seen := make(map[uint]bool, len(list))
	for _, r := range list {
		for _, id := range RuleChannelIDs(r) {
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return names
	}
	chs, err := s.repo.FindAutoReplyChannelsByIDs(ids)
	if err != nil {
		return names
	}
	for _, ch := range chs {
		names[ch.ID] = ch.Name
	}
	return names
}

func (s *AutomationService) toAutoReplyVO(tenantID uint, r *model.AutoReply, viewerID uint, isAdmin bool, names map[uint]string, chNames map[uint]string) AutoReplyVO {
	ids := RuleChannelIDs(r)
	vo := AutoReplyVO{
		ID: r.ID, UserID: r.UserID, Name: r.Name, ChannelID: r.ChannelID,
		ChannelIDs: ids, ChannelNames: make([]string, 0, len(ids)),
		MatchType: r.MatchType, Keywords: r.Keywords, Reply: r.Reply,
		ReplyType: r.ReplyType, TransferToHuman: r.TransferToHuman,
		Enabled: r.Enabled, Priority: r.Priority, HitCount: r.HitCount,
		CreatedAt: util.Default().FmtTime(r.CreatedAt), ChannelName: "全渠道",
		Shared:   r.UserID == 0,
		Editable: autoReplyMutable(r, viewerID, isAdmin),
	}
	for _, id := range ids {
		if name := chNames[id]; name != "" {
			vo.ChannelNames = append(vo.ChannelNames, name)
			continue
		}
		// 渠道被删除时不留空位，退化成渠道 ID（与 owner 的兜底口径一致）
		vo.ChannelNames = append(vo.ChannelNames, "渠道 #"+strconv.FormatUint(uint64(id), 10))
	}
	// 旧字段 ChannelName 跟着多渠道走（多个用「、」连接），未配置渠道时保留「全渠道」兜底文案：
	// 老客户端即使只读 channel_name 也不会把多选规则错看成「全渠道」。
	if len(vo.ChannelNames) > 0 {
		vo.ChannelName = strings.Join(vo.ChannelNames, "、")
	}
	if r.UserID == 0 {
		vo.Owner = "公共"
	} else {
		vo.Owner = names[r.UserID]
		if vo.Owner == "" {
			if u, err := s.users.FindUserByID(tenantID, r.UserID); err == nil && u != nil {
				vo.Owner = u.Name
			}
			if vo.Owner == "" {
				// 账号被删除时不让列表出现空白，退化成账号 ID
				vo.Owner = "客服 " + strconv.FormatUint(uint64(r.UserID), 10)
			}
		}
		names[r.UserID] = vo.Owner
	}
	if vo.ReplyType == "" {
		vo.ReplyType = "text"
	}
	if vo.MatchType == "" {
		vo.MatchType = "contain"
	}
	return vo
}

// CreateAutoReply 新增规则：客服建的归属自己，管理员建的是公共规则（user_id = 0）。
// 渠道必须至少选 1 个且是当前账号已授权的渠道（管理员例外，可选任意渠道或「全渠道」）。
func (s *AutomationService) CreateAutoReply(tenantID, actorID uint, isAdmin bool, req AutoReplyRequest) (*AutoReplyVO, error) {
	if strings.TrimSpace(req.Keywords) == "" {
		return nil, errors.New("关键词不能为空")
	}
	if !req.HasTransfer() && strings.TrimSpace(req.Reply) == "" {
		return nil, errors.New("回复内容不能为空（转人工规则可留空）")
	}
	ids, _, err := s.resolveAutoReplyChannels(tenantID, actorID, isAdmin, req)
	if err != nil {
		return nil, err
	}
	owner := actorID
	if isAdmin {
		owner = 0
	}
	r := &model.AutoReply{
		TenantID: tenantID, UserID: owner, Name: req.Name,
		// 渠道统一落多选字段；旧的单渠道字段只服务历史数据，新建时保持 0（= 兼容旧读法）
		ChannelIDs: FormatChannelIDs(ids), ChannelID: 0,
		MatchType: defaultMatchType(req.MatchType), Keywords: req.Keywords,
		Reply: req.Reply, ReplyType: orDefaultStr(req.ReplyType, "text"),
		Enabled: true, Priority: req.Priority,
	}
	if req.TransferToHuman != nil {
		r.TransferToHuman = *req.TransferToHuman
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if err := s.repo.InsertAutoReply(r); err != nil {
		return nil, err
	}
	vo := s.toAutoReplyVO(tenantID, r, actorID, isAdmin, map[uint]string{}, s.channelNamesOf([]*model.AutoReply{r}))
	return &vo, nil
}

// UpdateAutoReply 修改规则：管理员可改全部；客服只能改自己创建的。
// 渠道字段同样校验（防止「先建后改」把渠道改成未授权的）。
func (s *AutomationService) UpdateAutoReply(tenantID, id, actorID uint, isAdmin bool, req AutoReplyRequest) error {
	r, err := s.repo.FindAutoReplyByID(tenantID, id)
	if err != nil {
		return errors.New("规则不存在")
	}
	if !autoReplyMutable(r, actorID, isAdmin) {
		return autoReplyReadonlyErr(r)
	}
	ids, channelsProvided, err := s.resolveAutoReplyChannels(tenantID, actorID, isAdmin, req)
	if err != nil {
		return err
	}
	if req.Name != "" {
		r.Name = req.Name
	}
	if channelsProvided {
		r.ChannelIDs = FormatChannelIDs(ids)
		r.ChannelID = 0
	}
	if req.MatchType != "" {
		r.MatchType = req.MatchType
	}
	if req.Keywords != "" {
		r.Keywords = req.Keywords
	}
	if req.Reply != "" {
		r.Reply = req.Reply
	}
	if req.ReplyType != "" {
		r.ReplyType = req.ReplyType
	}
	if req.TransferToHuman != nil {
		r.TransferToHuman = *req.TransferToHuman
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	r.Priority = req.Priority
	return s.repo.SaveAutoReply(r)
}

// ---------------- 渠道归属校验 ----------------

// resolveAutoReplyChannels 校验并规整本次请求的渠道配置（新增 / 修改共用）。
//
// 返回：
//   - ids：生效的渠道 ID 列表（空 = 全渠道，仅管理员允许）；
//   - provided：本次请求是否涉及渠道字段（修改时 false 表示保持原值不动）；
//   - err：越权 / 参数错误的业务错误（中文文案，由 handler 统一包成 code 400）。
//
// 规则：
//   - 管理员可任选渠道，空列表 = 全渠道；
//   - 客服至少要选 1 个渠道，且每个渠道都必须在其已授权渠道内（AccountService.UserChannels）。
func (s *AutomationService) resolveAutoReplyChannels(tenantID, actorID uint, isAdmin bool, req AutoReplyRequest) (ids []uint, provided bool, err error) {
	// 两个渠道字段都没传 = 不涉及渠道配置（修改接口用；新增接口调用方会再落成「全渠道」语义）
	if req.ChannelIDs == nil && req.ChannelID == nil {
		return []uint{}, false, nil
	}
	provided = true
	ids = normalizeChannelIDs(req.ChannelIDs)
	// 兼容旧客户端：channel_id > 0 等价于只选这一个渠道
	if len(ids) == 0 && req.ChannelID != nil && *req.ChannelID > 0 {
		ids = []uint{*req.ChannelID}
	}
	if isAdmin {
		return ids, true, nil
	}
	if len(ids) == 0 {
		// 空列表 + channel_id = 0（或不传）= 「全部渠道」，客服没有这个权限
		return nil, true, errors.New("请选择你已授权的渠道")
	}
	allowed := s.authorizedChannelIDs(tenantID, actorID)
	for _, id := range ids {
		if !allowed[id] {
			return nil, true, fmt.Errorf("无权在该渠道配置自动回复（渠道 #%d 未授权给你）", id)
		}
	}
	return ids, true, nil
}

// authorizedChannelIDs 账号已授权的渠道集合：管理员 = 租户全部渠道，客服 = 被分配的渠道
// （口径完全复用 AccountService.UserChannels，避免两处各算一套）。
func (s *AutomationService) authorizedChannelIDs(tenantID, actorID uint) map[uint]bool {
	allowed := make(map[uint]bool, 8)
	u, err := s.users.FindUserByID(tenantID, actorID)
	if err != nil || u == nil {
		return allowed
	}
	for _, id := range s.account.UserChannels(u) {
		allowed[id] = true
	}
	return allowed
}

// normalizeChannelIDs 规整前端传来的渠道 ID 列表：丢弃 0（0 不是真实渠道）与重复项。
func normalizeChannelIDs(ids []uint) []uint {
	out := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// DeleteAutoReply 删除规则：权限口径同修改。
func (s *AutomationService) DeleteAutoReply(tenantID, id, actorID uint, isAdmin bool) error {
	r, err := s.repo.FindAutoReplyByID(tenantID, id)
	if err != nil {
		return errors.New("规则不存在")
	}
	if !autoReplyMutable(r, actorID, isAdmin) {
		return autoReplyReadonlyErr(r)
	}
	return s.repo.RemoveAutoReply(tenantID, id)
}

// autoReplyMutable 判断当前账号能否改 / 删某条规则：
// 管理员可改全部；客服只能改自己创建的（user_id = 自己），公共规则（user_id = 0）只读。
func autoReplyMutable(r *model.AutoReply, actorID uint, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	return r.UserID > 0 && r.UserID == actorID
}

// autoReplyReadonlyErr 统一的权限拒绝文案：公共规则与新造的「别人的规则」分开说明。
func autoReplyReadonlyErr(r *model.AutoReply) error {
	if r.UserID == 0 {
		return errors.New("公共规则仅管理员可修改")
	}
	return errors.New("只能修改自己创建的规则")
}

// MatchRule 按优先级找出命中的规则（优先匹配指定渠道的规则，其次全渠道规则）
func (s *AutomationService) MatchRule(tenantID, channelID uint, question string) (*model.AutoReply, bool) {
	if strings.TrimSpace(question) == "" {
		return nil, false
	}
	// 规则读取失败按「没有规则」处理，不影响消息主流程（与原逻辑一致）
	rules, _ := s.repo.ListEnabledAutoReplies(tenantID)
	q := strings.ToLower(strings.TrimSpace(question))

	// 先渠道专属，再全渠道：渠道作用域统一由 MatchChannel 判定（含多选列表与旧单渠道两种语义）
	for _, global := range []bool{false, true} {
		for _, r := range rules {
			if IsGlobalChannelRule(r) != global {
				continue
			}
			if !MatchChannel(r, channelID) {
				continue
			}
			if matchKeywords(r, q) {
				r.HitCount++
				// 命中次数回写失败不阻断自动回复（与原逻辑一致）
				_ = s.repo.UpdateAutoReplyHitCount(r.ID, r.HitCount)
				return r, true
			}
		}
	}
	return nil, false
}

// ReplyResult 自动回复判定结果
type ReplyResult struct {
	Transferred bool        // 是否命中转人工
	Message     interface{} // 机器人回复的消息 VO（可能为 nil）
}

// aiHistoryLimit 拼进 AI 提示词的最近对话条数（不含当前这条客户消息）。
const aiHistoryLimit = 6

// recentHistory 取会话最近 limit 条历史消息（按 id 升序，不含刚落库的当前客户消息）。
// 查询失败返回空切片：AI 仍可单轮回答，只是少了多轮上下文。
func (s *AutomationService) recentHistory(tenantID, convID uint, limit int) []dto.MessageVO {
	list, err := s.conv.ListMessages(tenantID, convID, 0, limit+1)
	if err != nil || len(list) == 0 {
		return []dto.MessageVO{}
	}
	// ListMessages 已按 id 升序：最后一条是刚写入的当前客户消息，历史里去掉它
	return list[:len(list)-1]
}

// ReplyNow 同步执行一次自动回复判定（用户端接口即时返回用）
func (s *AutomationService) ReplyNow(tenantID, convID, customerID, channelID uint, question string) *ReplyResult {
	conv, err := s.repo.FindAutoReplyConversation(tenantID, convID)
	if err != nil {
		return nil
	}
	if conv.AgentID > 0 && conv.Status == "open" {
		return &ReplyResult{Transferred: false}
	}
	// 已被人工接待过的会话（客服接入或回复过）：机器人不再抢答。
	// 客服离线后会话会被释放回「待接待」，但这是同一个咨询，仍应由人接手；
	// 只有用户重新发起新一轮咨询（结束/归档后再来）才恢复自动回复。
	if conv.HandledByHuman && setting.RobotSkipHandled(tenantID) {
		return &ReplyResult{Transferred: false}
	}

	// 机器人关闭的渠道：整段跳过（不查规则、不调模型），保持「不回复」。
	// 原来无论如何都会先查一遍自动回复规则，关闭机器人的渠道等于白跑一次查询——
	// 跨网访问 MySQL 时这就是每条消息多 20ms。
	ch, chOK := s.channelCached(channelID)
	if chOK && !ch.RobotEnabled {
		return &ReplyResult{Transferred: false}
	}

	rule, matched := s.MatchRule(tenantID, channelID, question)

	if matched && rule.TransferToHuman {
		desc := ""
		if rule.Name != "" {
			desc = "（命中：" + rule.Name + "）"
		}
		s.conv.AppendSystemMessage(tenantID, convID, "客户请求人工客服"+desc+"，正在为您转接，请稍候")
		// 转人工：置为待接待并清空接待人（失败不阻断，后续由客服端手动接入）
		_ = s.repo.UpdateAutoReplyConversation(convID, map[string]interface{}{
			"status": "pending", "agent_id": 0,
		})
		s.conv.NotifyChannelAgents(tenantID, channelID, convID, "待接待提醒", "有客户请求人工客服，请尽快接入")
		s.conv.BroadcastConversation(tenantID, convID)
		realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{
			Event: "conversation.updated",
			Data:  map[string]interface{}{"id": convID, "status": "pending", "transferred": true},
		})
		return &ReplyResult{Transferred: true}
	}

	var reply string
	if matched {
		reply = rule.Reply
	} else {
		if chOK && ch.RobotEnabled {
			// 未命中关键词规则时的兜底链路（渠道未开启机器人则整段跳过，保持「不回复」）：
			//   ① AI 应答：ai.enabled 且配置齐全 → 检索「该渠道的自动回复规则」+ 大模型生成；
			//   ② 兜底话术：AI 未启用 / 配置不全 / 检索无命中 / 调用失败 → 后台可配的 robot.fallback_reply。
			// 关键词话术不再写死在代码里（已由 pkg/seed 灌成自动回复规则，命中规则走上面的分支）。
			// AI 内部任何失败都返回空串并打日志，这里不会因此漏回复。
			history := s.recentHistory(tenantID, convID, aiHistoryLimit)
			reply = s.ai.Answer(tenantID, channelID, question, history)
			if reply == "" {
				reply = setting.RobotFallback(tenantID)
			}
		}
	}
	if reply == "" {
		return &ReplyResult{Transferred: false}
	}
	m := &model.Message{
		TenantID: tenantID, ConversationID: convID,
		SenderRole: "user", SenderID: 0,
		Type: "text", Direction: "out", Content: reply, Read: true,
	}
	if err := s.repo.CreateAutoReplyMessage(m); err != nil {
		return &ReplyResult{Transferred: false}
	}
	// 回写会话摘要（失败不阻断，消息本身已入库）
	_ = s.repo.UpdateAutoReplyConversation(convID, map[string]interface{}{
		"last_message": reply, "last_message_at": time.Now(),
	})
	vo := s.conv.BuildMessageVO(m)
	realtime.HubInstance().Publish(tenantID, nil, realtime.Event{Event: "message.new", Data: vo})
	realtime.HubInstance().PublishToGuest(tenantID, convID, realtime.Event{Event: "message.new", Data: vo})
	return &ReplyResult{Transferred: false, Message: vo}
}

// robotChannelCache 渠道机器人开关缓存：每条上行消息都要判断「是否开启机器人」，
// 缓存 5s 可省掉一次数据库查询。
var robotChannelCache = cache.NewTTL()

// channelCached 取渠道（带缓存）。第二个返回值表示是否取到了渠道：
// 查不到时调用方应按「继续走规则匹配」处理，不要误判成「机器人关闭」。
func (s *AutomationService) channelCached(channelID uint) (*model.Channel, bool) {
	if channelID == 0 {
		return nil, false
	}
	ck := "robot:" + strconv.FormatUint(uint64(channelID), 10)
	if v, ok := robotChannelCache.Get(ck); ok {
		if chv, ok2 := v.(model.Channel); ok2 {
			ch := chv
			return &ch, true
		}
	}
	ch, err := s.repo.FindAutoReplyChannel(channelID)
	if err != nil || ch == nil {
		return nil, false
	}
	robotChannelCache.Set(ck, *ch, 5*time.Second)
	return ch, true
}
