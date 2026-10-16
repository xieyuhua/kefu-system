package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/response"

	"gorm.io/gorm"
)

const TimeLayout = "2006-01-02 15:04:05"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(TimeLayout)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(TimeLayout)
}

// ---------------- Channel ----------------

func ListChannels(tenantID uint, agentID uint, isAdmin bool) ([]dto.ChannelVO, error) {
	var list []model.Channel
	q := gormDB().Where("tenant_id = ?", tenantID)
	if !isAdmin {
		q = q.Where("id IN (SELECT channel_id FROM agent_channels WHERE user_id = ?)", agentID)
	}
	if err := q.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]dto.ChannelVO, 0, len(list))
	for _, c := range list {
		var agentCount int64
		gormDB().Model(&model.AgentChannel{}).Where("channel_id = ?", c.ID).Count(&agentCount)
		var pending int64
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND channel_id = ? AND status = 'pending'", tenantID, c.ID).Count(&pending)
		out = append(out, dto.ChannelVO{
			ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
			Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
			HideHeader: c.HideHeader, HideDebug: c.HideDebug, HideMoreMenu: c.HideMoreMenu,
			CreatedAt: fmtTime(c.CreatedAt), AgentCount: agentCount, PendingCount: pending,
			// 始终用后端配置的 public_base 拼完整 URL，
			// 不要让前端用 window.location.origin，避免内网/反代场景下复制到错地址
			AccessPath: AccessPath(c),
			AccessURL:  AccessURL(c, ""),
		})
	}
	return out, nil
}

// GenerateAppID 生成一个随机、可读的渠道业务标识。
// 形如 kefu_a1b2c3d4（8 位十六进制），用于区分同一类型下的不同渠道实例。
func GenerateAppID(tenantID uint) string {
	var buf [4]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			break
		}
		id := fmt.Sprintf("kefu_%s", hex.EncodeToString(buf[:]))
		var cnt int64
		gormDB().Model(&model.Channel{}).
			Where("tenant_id = ? AND app_id = ?", tenantID, id).Count(&cnt)
		if cnt == 0 {
			return id
		}
	}
	// 极端情况下退回时间戳，保证一定能生成
	return fmt.Sprintf("kefu_%d", time.Now().UnixNano())
}

// AccessPath 渠道的用户端接入路径（相对路径）
// 使用 app_id 作为渠道标识：同一类型可配置多个渠道，只有 app_id 能唯一区分。
// 例如 /kefu/?channel=kefu_a1b2c3d4
// 旧的 /c 与 channel=<type|id> 仍可解析（向后兼容）。
func AccessPath(c model.Channel) string {
	return "/kefu/?channel=" + url.QueryEscape(ChannelKey(c))
}

// ChannelKey 返回渠道在接入链接中使用的标识：有 app_id 用 app_id，否则退回 id。
func ChannelKey(c model.Channel) string {
	if key := strings.TrimSpace(c.AppID); key != "" {
		return key
	}
	return strconv.FormatUint(uint64(c.ID), 10)
}

func CreateChannel(tenantID uint, req dto.ChannelRequest) (*dto.ChannelVO, error) {
	if req.Name == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	// 渠道类型不限制枚举，可任意自定义（如 wechat / wecom / web / 小红书 / 快手 ...）
	if req.Type == "" {
		req.Type = "custom"
	}
	c := model.Channel{
		TenantID:     tenantID,
		Type:         req.Type,
		Name:         req.Name,
		AppID:        strings.TrimSpace(req.AppID),
		Enabled:      true,
		RobotEnabled: req.RobotEnabled != nil && *req.RobotEnabled,
		HideHeader:   req.HideHeader != nil && *req.HideHeader,
		HideDebug:    req.HideDebug != nil && *req.HideDebug,
		HideMoreMenu: req.HideMoreMenu != nil && *req.HideMoreMenu,
	}
	// app_id 未指定时自动生成一个随机标识（可在渠道管理里重置）
	if c.AppID == "" {
		c.AppID = GenerateAppID(tenantID)
	} else if err := ensureAppIDUnique(tenantID, c.AppID, 0); err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if err := gormDB().Create(&c).Error; err != nil {
		return nil, err
	}
	return &dto.ChannelVO{
		ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
		Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
		HideHeader: c.HideHeader, HideDebug: c.HideDebug, HideMoreMenu: c.HideMoreMenu,
		CreatedAt: fmtTime(c.CreatedAt),
	}, nil
}

// ensureAppIDUnique 保证同一租户内 app_id 唯一（excludeID 用于更新场景排除自身）
func ensureAppIDUnique(tenantID uint, appID string, excludeID uint) error {
	var cnt int64
	q := gormDB().Model(&model.Channel{}).
		Where("tenant_id = ? AND app_id = ?", tenantID, appID)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	q.Count(&cnt)
	if cnt > 0 {
		return errors.New("该渠道标识（app_id）已被占用")
	}
	return nil
}

// ResetChannelAppID 重置渠道的 app_id（生成新的随机标识）
// 注意：重置后旧链接会失效，需要重新分发二维码/链接。
func ResetChannelAppID(tenantID, id uint) (string, error) {
	var c model.Channel
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return "", errors.New("渠道不存在")
	}
	for i := 0; i < 5; i++ {
		newID := GenerateAppID(tenantID)
		if newID == c.AppID {
			continue
		}
		c.AppID = newID
		if err := gormDB().Save(&c).Error; err != nil {
			return "", err
		}
		return newID, nil
	}
	return "", errors.New("生成渠道标识失败，请重试")
}

func UpdateChannel(tenantID, id uint, req dto.ChannelRequest) error {
	var c model.Channel
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return errors.New("渠道不存在")
	}
	if req.Name != "" {
		c.Name = req.Name
	}
	if req.Type != "" {
		c.Type = req.Type
	}
	if req.AppID != "" {
		appID := strings.TrimSpace(req.AppID)
		if appID != c.AppID {
			if err := ensureAppIDUnique(tenantID, appID, c.ID); err != nil {
				return err
			}
			c.AppID = appID
		}
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if req.RobotEnabled != nil {
		c.RobotEnabled = *req.RobotEnabled
	}
	// 用户端展示控制（APP 内嵌场景）
	if req.HideHeader != nil {
		c.HideHeader = *req.HideHeader
	}
	if req.HideDebug != nil {
		c.HideDebug = *req.HideDebug
	}
	if req.HideMoreMenu != nil {
		c.HideMoreMenu = *req.HideMoreMenu
	}
	return gormDB().Save(&c).Error
}

func DeleteChannel(tenantID, id uint) error {
	gormDB().Where("channel_id = ?", id).Delete(&model.AgentChannel{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Channel{}).Error
}

// ---------------- Agent ----------------

func ListAgents(tenantID uint) ([]dto.AgentVO, error) {
	var users []model.User
	if err := gormDB().Where("tenant_id = ?", tenantID).Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]dto.AgentVO, 0, len(users))
	for _, u := range users {
		vo := dto.AgentVO{
			ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
			Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
			Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
		}
		vo.Status = "offline"
		if u.Online {
			vo.Status = "online"
		}
		var acs []model.AgentChannel
		gormDB().Where("user_id = ?", u.ID).Find(&acs)
		for _, ac := range acs {
			var ch model.Channel
			if gormDB().First(&ch, ac.ChannelID).Error == nil {
				vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
			}
		}
		var active int64
		gormDB().Model(&model.Conversation{}).
			Where("tenant_id = ? AND agent_id = ? AND status = 'open'", tenantID, u.ID).Count(&active)
		vo.ActiveCount = active
		out = append(out, vo)
	}
	return out, nil
}

func CreateAgent(tenantID uint, req dto.AgentRequest) (*dto.AgentVO, error) {
	if req.Account == "" || req.Password == "" {
		return nil, errors.New("账号与密码不能为空")
	}
	var exist model.User
	if gormDB().Where("account = ?", req.Account).First(&exist).Error == nil {
		return nil, errors.New("账号已存在")
	}
	hash, err := hashPwd(req.Password)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role != "admin" {
		role = "agent"
	}
	maxC := req.MaxConcurrent
	if maxC <= 0 {
		maxC = 5
	}
	u := model.User{
		TenantID:      tenantID,
		Account:       req.Account,
		PasswordHash:  hash,
		Name:          orDefault(req.Name, req.Account),
		Role:          role,
		Avatar:        "/demo/avatar-2.svg",
		MaxConcurrent: maxC,
		Enabled:       true,
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := gormDB().Create(&u).Error; err != nil {
		return nil, err
	}
	setAgentChannels(tenantID, u.ID, req.ChannelIDs)
	return getAgentVO(tenantID, u.ID)
}

func UpdateAgent(tenantID, id uint, req dto.AgentRequest) error {
	var u model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&u).Error; err != nil {
		return errors.New("账号不存在")
	}
	if req.Name != "" {
		u.Name = req.Name
	}
	if req.Role != "" {
		u.Role = req.Role
	}
	if req.MaxConcurrent > 0 {
		u.MaxConcurrent = req.MaxConcurrent
	}
	if req.Password != "" {
		h, err := hashPwd(req.Password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := gormDB().Save(&u).Error; err != nil {
		return err
	}
	if req.ChannelIDs != nil {
		setAgentChannels(tenantID, id, req.ChannelIDs)
	}
	return nil
}

func DeleteAgent(tenantID, id uint) error {
	gormDB().Where("user_id = ?", id).Delete(&model.AgentChannel{})
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.User{}).Error
}

func SetOnline(tenantID, id uint, online bool) error {
	return gormDB().Model(&model.User{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Update("online", online).Error
}

func setAgentChannels(tenantID, userID uint, ids []uint) {
	gormDB().Where("user_id = ?", userID).Delete(&model.AgentChannel{})
	var valid []model.Channel
	gormDB().Where("tenant_id = ?", tenantID).Find(&valid)
	allowed := map[uint]bool{}
	for _, c := range valid {
		allowed[c.ID] = true
	}
	for _, id := range ids {
		if !allowed[id] {
			continue
		}
		gormDB().Create(&model.AgentChannel{UserID: userID, ChannelID: id})
	}
}

func getAgentVO(tenantID, id uint) (*dto.AgentVO, error) {
	var u model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&u).Error; err != nil {
		return nil, err
	}
	vo := &dto.AgentVO{
		ID: u.ID, Account: u.Account, Name: u.Name, Role: u.Role,
		Avatar: u.Avatar, Online: u.Online, MaxConcurrent: u.MaxConcurrent,
		Enabled: u.Enabled, Channels: []dto.ChannelSimple{},
	}
	vo.Status = "offline"
	if u.Online {
		vo.Status = "online"
	}
	var acs []model.AgentChannel
	gormDB().Where("user_id = ?", id).Find(&acs)
	for _, ac := range acs {
		var ch model.Channel
		if gormDB().First(&ch, ac.ChannelID).Error == nil {
			vo.Channels = append(vo.Channels, dto.ChannelSimple{ID: ch.ID, Type: ch.Type, Name: ch.Name})
		}
	}
	return vo, nil
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

func ListConversations(tenantID, userID uint, isAdmin bool, q ConvQuery) (*response.Paged, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	dbq := gormDB().Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	// 默认不展示已归档会话；IncludeArchived 时一并返回（历史档案 / 统计用）
	if !q.IncludeArchived {
		dbq = dbq.Where("archived = ?", false)
	}
	if q.ChannelID > 0 {
		dbq = dbq.Where("channel_id = ?", q.ChannelID)
	}
	if q.Status != "" && q.Status != "all" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	// 可见范围（按角色区分）：
	//   - 管理员：默认看全部；scope=mine 只看自己接待的
	//   - 普通客服：只能看「自己接待的」+「自己可接待渠道下、尚未被别人领取的」
	//     已被同渠道其他客服领取的会话对其隐藏（实现互斥接待）
	if q.Scope == "mine" {
		dbq = dbq.Where("agent_id = ?", userID)
	} else if isAdmin {
		// 管理员全量可见，不做额外过滤
	} else {
		var chIDs []uint
		gormDB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
		if len(chIDs) > 0 {
			// 自己接待的，或（未接待 且 在自己渠道内）——别人已领取的自然被排除
			dbq = dbq.Where("agent_id = ? OR (agent_id = 0 AND channel_id IN ?)", userID, chIDs)
		} else {
			dbq = dbq.Where("agent_id = ?", userID)
		}
	}
	if q.Keyword != "" {
		kw := "%" + strings.TrimSpace(q.Keyword) + "%"
		dbq = dbq.Where(
			"last_message LIKE ? OR customer_id IN (SELECT id FROM customers WHERE nickname LIKE ?)",
			kw, kw)
	}

	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Conversation
	if err := dbq.Order("last_message_at desc").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	vos := make([]interface{}, 0, len(list))
	for _, c := range list {
		vos = append(vos, buildConversationVO(c))
	}
	return &response.Paged{List: vos, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func GetConversation(tenantID, id uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	vo := buildConversationVO(c)
	return &vo, nil
}

func buildConversationVO(c model.Conversation) dto.ConversationVO {
	vo := dto.ConversationVO{
		ID: c.ID, ChannelID: c.ChannelID, Status: c.Status,
		Unread: c.Unread, LastMessage: c.LastMessage,
		LastMessageAt: fmtTime(c.LastMessageAt), CreatedAt: fmtTime(c.CreatedAt),
		ClosedAt: fmtTimePtr(c.ClosedAt), MessageCount: c.MessageCount, Summary: c.Summary,
		Archived: c.Archived, ArchivedAt: fmtTimePtr(c.ArchivedAt),
	}
	var ch model.Channel
	if gormDB().First(&ch, c.ChannelID).Error == nil {
		vo.Channel = &dto.ChannelRef{ID: ch.ID, Type: ch.Type, Name: ch.Name}
	}
	var cu model.Customer
	if gormDB().First(&cu, c.CustomerID).Error == nil {
		var oc int64
		gormDB().Model(&model.Order{}).Where("customer_id = ?", cu.ID).Count(&oc)
		vo.User = &dto.CustomerRef{
			ID: cu.ID, ExternalID: cu.ExternalID, Nickname: cu.Nickname,
			Avatar: cu.Avatar, Phone: cu.Phone, OrderCount: oc, ChannelID: cu.ChannelID,
			Tags:   GetCustomerTags(c.TenantID, cu.ID),
			Remark: cu.Remark,
			IP:     cu.IP,
			City:   cityOrLookup(cu),
			// 客户在线状态：按最近活跃时间推断（2 分钟内活跃即在线）
			Online:     IsCustomerOnline(cu),
			LastSeenAt: fmtTimePtr(cu.LastSeenAt),
		}
		vo.OrderCount = oc
	}
	if c.AgentID > 0 {
		var u model.User
		if gormDB().First(&u, c.AgentID).Error == nil {
			vo.Agent = &dto.UserBrief{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: u.Role, Online: u.Online}
		}
	}
	return vo
}

// isAdminUser 判断用户是否为管理员（管理员享有接管/查看全部会话的特权）
func isAdminUser(userID uint) bool {
	if userID == 0 {
		return false
	}
	var u model.User
	if err := gormDB().Select("id", "role").Where("id = ?", userID).First(&u).Error; err != nil {
		return false
	}
	return u.Role == "admin"
}

// canServeChannel 判断客服是否配置了接待该渠道。
func canServeChannel(userID, channelID uint) bool {
	var n int64
	gormDB().Model(&model.AgentChannel{}).
		Where("user_id = ? AND channel_id = ?", userID, channelID).Count(&n)
	return n > 0
}

func ClaimConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.Status == "closed" {
		return nil, errors.New("会话已结束")
	}
	// 渠道绑定校验：该渠道只接待账号限制了名单时，未绑定的客服不能接。
	// 管理员不受限制（可接待任意渠道）。
	if !isAdminUser(agentID) && !canServeChannel(agentID, c.ChannelID) {
		return nil, errors.New("您未配置接待该渠道")
	}
	// 互斥接待：已被其他客服领取的会话不能再领。
	// 管理员例外——可以接管任何会话（含其他客服正在接待的）。
	if c.AgentID != 0 && c.AgentID != agentID && !isAdminUser(agentID) {
		return nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	c.AgentID = agentID
	c.Status = "open"
	c.LastMessageAt = now
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, agentName(agentID)+" 已接入会话")
	vo := buildConversationVO(c)
	// 通知用户端：已有人工客服接入
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

// lastSystemMessage 取会话最后一条 system 消息（VO）
func lastSystemMessage(tenantID, convID uint) *dto.MessageVO {
	var m model.Message
	if err := gormDB().Where("tenant_id = ? AND conversation_id = ? AND type = 'system'",
		tenantID, convID).Order("id desc").First(&m).Error; err != nil {
		return nil
	}
	vo := buildMessageVO(m)
	return &vo
}

func TransferConversation(tenantID, id, fromAgentID, toAgentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	if c.AgentID != fromAgentID {
		return nil, errors.New("只能转接自己接待的会话")
	}
	var target model.User
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, toAgentID).First(&target).Error; err != nil {
		return nil, errors.New("目标客服不存在")
	}
	// 转接目标必须配置了接待该渠道，否则转过去他根本看不到这个会话
	if !isAdminUser(toAgentID) && !canServeChannel(toAgentID, c.ChannelID) {
		return nil, errors.New("该客服未配置接待此渠道，无法转接")
	}
	c.AgentID = toAgentID
	c.Status = "open"
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, fmt.Sprintf("会话已转接给 %s", target.Name))
	Notify(tenantID, toAgentID, "conversation.transferred", "会话转接", "有一条会话被转接给你", c.ID)
	vo := buildConversationVO(c)
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	return &vo, nil
}

func CloseConversation(tenantID, id, agentID uint) (*dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error; err != nil {
		return nil, errors.New("会话不存在")
	}
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, err
	}
	addSystemMessage(tenantID, c.ID, "会话已结束，感谢您的咨询")
	vo := buildConversationVO(c)
	if m := lastSystemMessage(tenantID, c.ID); m != nil {
		HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "message.new", Data: *m})
	}
	HubInstance().PublishToGuest(tenantID, c.ID, Event{Event: "conversation.updated", Data: vo})
	return &vo, nil
}

func MarkRead(tenantID, id uint) error {
	gormDB().Model(&model.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{"unread": 0})
	gormDB().Model(&model.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, id).
		Update("read", true)
	return nil
}

func ConversationStats(tenantID, userID uint, isAdmin bool) (*dto.StatsVO, error) {
	s := &dto.StatsVO{}

	// 可见范围条件（会话级），每项统计都基于它新建查询，避免 GORM 条件互相污染
	var scopeCond string
	var scopeArgs []interface{}
	if !isAdmin {
		var chIDs []uint
		gormDB().Model(&model.AgentChannel{}).Where("user_id = ?", userID).Pluck("channel_id", &chIDs)
		if len(chIDs) > 0 {
			scopeCond = " AND (agent_id = ? OR (agent_id = 0 AND channel_id IN ?))"
			scopeArgs = []interface{}{userID, chIDs}
		} else {
			scopeCond = " AND agent_id = ?"
			scopeArgs = []interface{}{userID}
		}
	}

	// 统计口径必须与 ListConversations 完全一致：默认排除已归档会话。
	// 否则会出现「侧边栏显示未接待 1，点进去却是空的」——归档会话被算进了统计，
	// 但列表接口过滤掉了 archived = true 的记录。
	const archivedCond = " AND archived = ?"
	const archivedArg = false

	countBy := func(extra string, extraArgs ...interface{}) int64 {
		args := append([]interface{}{tenantID}, scopeArgs...)
		args = append(args, archivedArg)
		if extra != "" {
			args = append(args, extraArgs...)
		}
		where := "tenant_id = ?" + scopeCond + archivedCond + extra
		var n int64
		gormDB().Model(&model.Conversation{}).Where(where, args...).Count(&n)
		return n
	}

	// 未读消息总数：把各会话的 unread 求和（会话 unread 字段 = 该会话未读消息条数）
	sumUnread := func() int64 {
		args := append([]interface{}{tenantID}, scopeArgs...)
		args = append(args, archivedArg)
		where := "tenant_id = ?" + scopeCond + archivedCond
		var total sql.NullInt64
		row := gormDB().Model(&model.Conversation{}).
			Select("SUM(unread)").Where(where, args...).Row()
		if err := row.Scan(&total); err == nil && total.Valid {
			return total.Int64
		}
		return 0
	}

	s.Pending = countBy(" AND status = ?", "pending")
	s.Open = countBy(" AND status = ?", "open")
	s.Closed = countBy(" AND status = ?", "closed")
	s.Unread = sumUnread()
	return s, nil
}

// ---------------- Message ----------------

// ListMessagesSince 正向增量拉取：返回 id > afterID 的消息（按 id 升序）。
// 供用户端 / APP 离线同步使用 —— 与 ListMessages 的「向前翻页」语义相反，勿混用。
func ListMessagesSince(tenantID, convID uint, afterID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := gormDB().Where("tenant_id = ? AND conversation_id = ? AND id > ?", tenantID, convID, afterID)
	var list []model.Message
	if err := q.Order("id asc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]dto.MessageVO, 0, len(list))
	for _, m := range list {
		out = append(out, buildMessageVO(m))
	}
	return out, nil
}

// ListMessages 分页向前翻页：返回 id < beforeID 的消息（按 id 升序）
func ListMessages(tenantID, convID uint, beforeID uint, limit int) ([]dto.MessageVO, error) {
	if limit <= 0 {
		limit = 50
	}
	q := gormDB().Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []model.Message
	if err := q.Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	// 反转为升序返回
	out := make([]dto.MessageVO, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, buildMessageVO(list[i]))
	}
	return out, nil
}

// BuildMessageVO 导出版本，供其他包（如消息搜索）复用
func BuildMessageVO(m model.Message) dto.MessageVO { return buildMessageVO(m) }

func buildMessageVO(m model.Message) dto.MessageVO {
	vo := dto.MessageVO{
		ID: m.ID, ConversationID: m.ConversationID, Type: m.Type,
		Direction: m.Direction, Content: m.Content,
		CreatedAt: fmtTime(m.CreatedAt),
	}
	switch m.SenderRole {
	case "user":
		var u model.User
		if m.SenderID > 0 && gormDB().First(&u, m.SenderID).Error == nil {
			vo.Sender = dto.SenderRef{ID: u.ID, Name: u.Name, Avatar: u.Avatar, Role: "agent"}
		} else {
			vo.Sender = dto.SenderRef{Name: "智能助手", Role: "robot"}
		}
	case "customer":
		var c model.Customer
		if gormDB().First(&c, m.SenderID).Error == nil {
			vo.Sender = dto.SenderRef{ID: c.ID, Name: c.Nickname, Avatar: c.Avatar, Role: "user"}
		}
	default:
		vo.Sender = dto.SenderRef{Role: "system"}
	}
	return vo
}

// SendMessage 客服发送消息
func SendMessage(tenantID, convID, senderID uint, req dto.SendMessageRequest) (*dto.MessageVO, *dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
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
		if !isAdminUser(senderID) && !canServeChannel(senderID, c.ChannelID) {
			return nil, nil, errors.New("您未配置接待该渠道")
		}
		c.AgentID = senderID
		c.Status = "open"
	} else if c.AgentID != senderID && !isAdminUser(senderID) {
		// 会话已被他人接待，禁止越权发言
		return nil, nil, errors.New("会话已被其他客服接待")
	}
	now := time.Now()
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       senderID,
		SenderRole:     "user",
		Type:           req.Type,
		Direction:      "out",
		Content:        req.Content,
		Read:           true,
	}
	if err := gormDB().Create(&m).Error; err != nil {
		return nil, nil, err
	}
	c.LastMessage = messagePreview(m)
	c.LastMessageAt = now
	c.MessageCount++
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, nil, err
	}
	mvo := buildMessageVO(m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func messagePreview(m model.Message) string {
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

func addSystemMessage(tenantID, convID uint, content string) *model.Message {
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderRole:     "system",
		Type:           "system",
		Direction:      "system",
		Content:        content,
		Read:           true,
	}
	gormDB().Create(&m)
	gormDB().Model(&model.Conversation{}).Where("id = ?", convID).
		Update("message_count", gorm.Expr("message_count + 1"))
	return &m
}

// ReceiveMessage 客户侧发消息（演示入口 / 渠道回调共用）
func ReceiveMessage(tenantID, convID, customerID uint, msgType, content string) (*dto.MessageVO, *dto.ConversationVO, error) {
	var c model.Conversation
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, convID).First(&c).Error; err != nil {
		return nil, nil, errors.New("会话不存在")
	}
	if msgType == "" {
		msgType = "text"
	}
	m := model.Message{
		TenantID:       tenantID,
		ConversationID: convID,
		SenderID:       customerID,
		SenderRole:     "customer",
		Type:           msgType,
		Direction:      "in",
		Content:        content,
	}
	if err := gormDB().Create(&m).Error; err != nil {
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
	if err := gormDB().Save(&c).Error; err != nil {
		return nil, nil, err
	}
	gormDB().Model(&model.Customer{}).Where("id = ?", customerID).Update("last_seen_at", now)

	mvo := buildMessageVO(m)
	cvo := buildConversationVO(c)
	return &mvo, &cvo, nil
}

func agentName(id uint) string {
	var u model.User
	if gormDB().First(&u, id).Error == nil && u.Name != "" {
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
