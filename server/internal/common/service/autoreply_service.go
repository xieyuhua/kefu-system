package service

// ---------------- 自动回复域（Automation） ----------------
// 关键词规则 → 机器人回复 / 转人工。
// 跨域调用全部走 ConversationService 的方法，不直接读写对方的表。
//
// 形态约定：业务编排都是 AutomationService 的方法（指针接收者），跨域依赖在构造时建好；
// 只有无状态的纯函数（matchKeywords / splitKeywords / defaultMatchType / orDefaultStr / robotAnswer）保留为包级函数。
import (
	"errors"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/realtime"
	"kefu-server/pkg/util"
)

// AutomationService 自动回复内核的业务编排层，持有本模块仓储、会话内核依赖与账号只读依赖。
type AutomationService struct {
	repo  *repository.AutoReplyRepository
	conv  *ConversationService
	users *repository.UserRepository
}

// NewAutomationService 创建业务层实例（自带数据访问层与会话内核依赖）。
func NewAutomationService() *AutomationService {
	return &AutomationService{
		repo:  repository.NewAutoReplyRepository(),
		conv:  NewConversationService(),
		users: repository.NewUserRepository(),
	}
}

// ---------------- 自动回复规则 ----------------
// 关键词命中 → 机器人自动回复；若该规则标记为"转人工"，则直接把会话转为待人工接待。
//
// 归属与权限（归属字段 user_id）：
//   - user_id = 0：管理员建的公共规则，全租户客服可见、可用，但只有管理员能改 / 删；
//   - user_id > 0：创建该规则的客服，只有本人（和管理员）能改 / 删。
//
// 匹配 / 触发链路（MatchRule / ReplyNow / hit_count）与改造前完全一致，归属只影响写权限。

type AutoReplyVO struct {
	ID              uint   `json:"id"`
	UserID          uint   `json:"user_id"`
	Owner           string `json:"owner"`    // 「公共」或创建人昵称，前端直接展示
	Shared          bool   `json:"shared"`   // user_id = 0：公共规则（客服只读）
	Editable        bool   `json:"editable"` // 当前登录账号能否改 / 删
	Name            string `json:"name"`
	ChannelID       uint   `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	MatchType       string `json:"match_type"` // contain | equal | prefix
	Keywords        string `json:"keywords"`   // 逗号分隔
	Reply           string `json:"reply"`
	ReplyType       string `json:"reply_type"` // text | product
	TransferToHuman bool   `json:"transfer_to_human"`
	Enabled         bool   `json:"enabled"`
	Priority        int    `json:"priority"`
	HitCount        int64  `json:"hit_count"`
	CreatedAt       string `json:"created_at"`
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
	out := make([]AutoReplyVO, 0, len(list))
	for _, r := range list {
		out = append(out, s.toAutoReplyVO(tenantID, r, viewerID, isAdmin, names))
	}
	return out, nil
}

func (s *AutomationService) toAutoReplyVO(tenantID uint, r *model.AutoReply, viewerID uint, isAdmin bool, names map[uint]string) AutoReplyVO {
	vo := AutoReplyVO{
		ID: r.ID, UserID: r.UserID, Name: r.Name, ChannelID: r.ChannelID,
		MatchType: r.MatchType, Keywords: r.Keywords, Reply: r.Reply,
		ReplyType: r.ReplyType, TransferToHuman: r.TransferToHuman,
		Enabled: r.Enabled, Priority: r.Priority, HitCount: r.HitCount,
		CreatedAt: util.Default().FmtTime(r.CreatedAt), ChannelName: "全渠道",
		Shared:   r.UserID == 0,
		Editable: autoReplyMutable(r, viewerID, isAdmin),
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
	if r.ChannelID > 0 {
		// 渠道名查不到时保留"全渠道"兜底文案，不影响规则列表展示（与原逻辑一致）
		if ch, err := s.repo.FindAutoReplyChannel(r.ChannelID); err == nil {
			vo.ChannelName = ch.Name
		}
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
func (s *AutomationService) CreateAutoReply(tenantID, actorID uint, isAdmin bool, req AutoReplyRequest) (*AutoReplyVO, error) {
	if strings.TrimSpace(req.Keywords) == "" {
		return nil, errors.New("关键词不能为空")
	}
	if !req.HasTransfer() && strings.TrimSpace(req.Reply) == "" {
		return nil, errors.New("回复内容不能为空（转人工规则可留空）")
	}
	owner := actorID
	if isAdmin {
		owner = 0
	}
	r := &model.AutoReply{
		TenantID: tenantID, UserID: owner, Name: req.Name, ChannelID: req.ChannelID,
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
	vo := s.toAutoReplyVO(tenantID, r, actorID, isAdmin, map[uint]string{})
	return &vo, nil
}

// UpdateAutoReply 修改规则：管理员可改全部；客服只能改自己创建的。
func (s *AutomationService) UpdateAutoReply(tenantID, id, actorID uint, isAdmin bool, req AutoReplyRequest) error {
	r, err := s.repo.FindAutoReplyByID(tenantID, id)
	if err != nil {
		return errors.New("规则不存在")
	}
	if !autoReplyMutable(r, actorID, isAdmin) {
		return autoReplyReadonlyErr(r)
	}
	if req.Name != "" {
		r.Name = req.Name
	}
	if req.ChannelID > 0 || req.ChannelID == 0 && req.Name == "" {
		r.ChannelID = req.ChannelID
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

	// 先渠道专属，再全渠道
	for _, scope := range []uint{channelID, 0} {
		for _, r := range rules {
			if r.ChannelID != scope {
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

// ReplyNow 同步执行一次自动回复判定（用户端接口即时返回用）
func (s *AutomationService) ReplyNow(tenantID, convID, customerID, channelID uint, question string) *ReplyResult {
	conv, err := s.repo.FindAutoReplyConversation(tenantID, convID)
	if err != nil {
		return nil
	}
	if conv.AgentID > 0 && conv.Status == "open" {
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
		if ch, err := s.repo.FindAutoReplyChannel(channelID); err == nil && ch.RobotEnabled {
			reply = robotAnswer(question)
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
