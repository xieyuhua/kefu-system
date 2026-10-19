// Package channel 渠道内核：channels、channel_types、渠道二维码 / 接入地址、
// 渠道 ↔ 接待账号分配。模块内按 service / repository 分层，HTTP 层在各端模块。
//
// 依赖方向：core/channel → platform → pkg（不依赖任何业务端模块，也不依赖其它内核包）。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），Service 在构造时建好自己的
// Repository 依赖；只有无状态的纯函数（AccessPath / ChannelKey / AccessURL）保留为包级函数。
package channel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"kefu-server/internal/platform/config"
	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/idgen"
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/util"

	qrcode "github.com/skip2/go-qrcode"
)

// Service 渠道内核的业务编排层。
type Service struct {
	repo *Repository
}

// NewService 创建业务层实例（自带数据访问层依赖）。
func NewService() *Service { return &Service{repo: NewRepository()} }

// ---------------- Channel ----------------

func (s *Service) ListChannels(tenantID uint, agentID uint, isAdmin bool) ([]dto.ChannelVO, error) {
	list, err := s.repo.ListChannelsByTenant(tenantID, agentID, isAdmin)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ChannelVO, 0, len(list))
	for _, c := range list {
		// 角标统计失败不阻断列表展示，与原逻辑一致
		agentCount, _ := s.repo.CountAgentsOfChannel(c.ID)
		pending, _ := s.repo.CountPendingByChannel(tenantID, c.ID)
		out = append(out, dto.ChannelVO{
			ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
			Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
			HideHeader: c.HideHeader, HideDebug: c.HideDebug, HideMoreMenu: c.HideMoreMenu,
			CreatedAt: util.FmtTime(c.CreatedAt), AgentCount: agentCount, PendingCount: pending,
			// 始终用后端配置的 public_base 拼完整 URL，
			// 不要让前端用 window.location.origin，避免内网/反代场景下复制到错地址
			AccessPath: AccessPath(c),
			AccessURL:  AccessURL(c, ""),
		})
	}
	return out, nil
}

// AccessPath 渠道的用户端接入路径（相对路径）
// 使用 app_id 作为渠道标识：同一类型可配置多个渠道，只有 app_id 能唯一区分。
// 例如 /kefu/?channel=kefu_a1b2c3d4
// 旧的 /c 与 channel=<type|id> 仍可解析（向后兼容）。
func AccessPath(c *model.Channel) string {
	return "/kefu/?channel=" + url.QueryEscape(ChannelKey(c))
}

// ChannelKey 返回渠道在接入链接中使用的标识：有 app_id 用 app_id，否则退回 id。
func ChannelKey(c *model.Channel) string {
	if key := strings.TrimSpace(c.AppID); key != "" {
		return key
	}
	return strconv.FormatUint(uint64(c.ID), 10)
}

func (s *Service) CreateChannel(tenantID uint, req dto.ChannelRequest) (*dto.ChannelVO, error) {
	if req.Name == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	// 渠道类型不限制枚举，可任意自定义（如 wechat / wecom / web / 小红书 / 快手 ...）
	if req.Type == "" {
		req.Type = "custom"
	}
	c := &model.Channel{
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
		c.AppID = idgen.GenerateAppID(tenantID)
	} else if err := s.ensureAppIDUnique(tenantID, c.AppID, 0); err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if err := s.repo.InsertChannel(c); err != nil {
		return nil, err
	}
	return &dto.ChannelVO{
		ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
		Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
		HideHeader: c.HideHeader, HideDebug: c.HideDebug, HideMoreMenu: c.HideMoreMenu,
		CreatedAt: util.FmtTime(c.CreatedAt),
	}, nil
}

// ensureAppIDUnique 保证同一租户内 app_id 唯一（excludeID 用于更新场景排除自身）
func (s *Service) ensureAppIDUnique(tenantID uint, appID string, excludeID uint) error {
	// 统计失败按「未被占用」处理，与原逻辑一致
	cnt, _ := s.repo.CountChannelByAppID(tenantID, appID, excludeID)
	if cnt > 0 {
		return errors.New("该渠道标识（app_id）已被占用")
	}
	return nil
}

// ResetChannelAppID 重置渠道的 app_id（生成新的随机标识）
// 注意：重置后旧链接会失效，需要重新分发二维码/链接。
func (s *Service) ResetChannelAppID(tenantID, id uint) (string, error) {
	c, err := s.repo.FindChannelByID(tenantID, id)
	if err != nil {
		return "", errors.New("渠道不存在")
	}
	for i := 0; i < 5; i++ {
		newID := idgen.GenerateAppID(tenantID)
		if newID == c.AppID {
			continue
		}
		c.AppID = newID
		if err := s.repo.SaveChannel(c); err != nil {
			return "", err
		}
		return newID, nil
	}
	return "", errors.New("生成渠道标识失败，请重试")
}

func (s *Service) UpdateChannel(tenantID, id uint, req dto.ChannelRequest) error {
	c, err := s.repo.FindChannelByID(tenantID, id)
	if err != nil {
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
			if err := s.ensureAppIDUnique(tenantID, appID, c.ID); err != nil {
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
	return s.repo.SaveChannel(c)
}

func (s *Service) DeleteChannel(tenantID, id uint) error {
	return s.repo.DeleteChannelWithBindings(tenantID, id)
}

// ---------------- 渠道接入地址与二维码 ----------------

// AccessURL 生成渠道的完整用户端接入地址。
// publicBase 为空时用配置的 public_base；两者都为空则回退成相对路径 /kefu/?channel=xxx。
func AccessURL(c *model.Channel, publicBase string) string {
	if publicBase == "" {
		publicBase = config.C.Server.PublicBase
	}
	path := AccessPath(c)
	if publicBase == "" {
		return path
	}
	return strings.TrimRight(publicBase, "/") + path
}

// AccessURLByAppID 按渠道 ID 取完整接入地址（重置 app_id 后返回新地址）
func (s *Service) AccessURLByAppID(tenantID, channelID uint, publicBase string) string {
	c, err := s.repo.FindChannelByID(tenantID, channelID)
	if err != nil {
		return ""
	}
	return AccessURL(c, publicBase)
}

// ChannelQR 生成渠道接入二维码（PNG bytes / base64）。
// size 为图片边长像素，建议 256~512。
func (s *Service) ChannelQR(tenantID, channelID uint, publicBase string, size int) (*ChannelQRVO, error) {
	c, err := s.repo.FindChannelByID(tenantID, channelID)
	if err != nil {
		return nil, errors.New("渠道不存在")
	}
	if size <= 0 {
		size = 320
	}
	if size > 1024 {
		size = 1024
	}
	target := AccessURL(c, publicBase)
	png, err := qrcode.Encode(target, qrcode.Medium, size)
	if err != nil {
		return nil, err
	}
	return &ChannelQRVO{
		ChannelID:   c.ID,
		ChannelName: c.Name,
		ChannelType: c.Type,
		URL:         target,
		Path:        AccessPath(c),
		PNG:         png,
		Base64:      "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		Size:        size,
		FileName:    fmt.Sprintf("channel-%s-%s.png", c.Type, c.Name),
	}, nil
}

type ChannelQRVO struct {
	ChannelID   uint   `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelType string `json:"channel_type"`
	URL         string `json:"url"`
	Path        string `json:"path"`
	PNG         []byte `json:"-"`
	Base64      string `json:"base64"` // data:image/png;base64,... 可直接放进 img src
	Size        int    `json:"size"`
	FileName    string `json:"file_name"` // 下载用的文件名
}

// ---------------- 渠道 ↔ 接待账号快速设置 ----------------

// ListChannelAgents 列出某渠道可分配的客服账号及当前是否已分配
func (s *Service) ListChannelAgents(tenantID, channelID uint) ([]dto.ChannelAgentVO, error) {
	if _, err := s.repo.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	users, err := s.repo.ListTenantAgents(tenantID)
	if err != nil {
		return nil, err
	}
	selected := map[uint]bool{}
	acs, _ := s.repo.ListAgentChannelsByChannel(channelID)
	for _, ac := range acs {
		selected[ac.UserID] = true
	}
	out := make([]dto.ChannelAgentVO, 0, len(users))
	for _, u := range users {
		out = append(out, dto.ChannelAgentVO{
			ID: u.ID, Name: u.Name, Account: u.Account, Avatar: u.Avatar,
			Online: u.Online, Enabled: u.Enabled, Selected: selected[u.ID],
		})
	}
	return out, nil
}

// SetChannelAgents 批量设置某渠道的接待账号（全量覆盖）
func (s *Service) SetChannelAgents(tenantID, channelID uint, userIDs []uint) ([]dto.ChannelAgentVO, error) {
	if _, err := s.repo.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	// 校验账号归属本租户
	valid := map[uint]bool{}
	if len(userIDs) > 0 {
		// 校验失败按「无账号通过校验」处理，与原逻辑一致
		users, _ := s.repo.FindAgentUsersByIDs(tenantID, userIDs)
		for _, u := range users {
			valid[u.ID] = true
		}
	}
	// 覆盖式写入：清空旧分配后逐个重建（写入失败由后续列表查询体现，与原逻辑一致）
	_ = s.repo.DeleteAgentChannels(channelID)
	for _, id := range userIDs {
		if !valid[id] {
			continue
		}
		_ = s.repo.CreateAgentChannel(&model.AgentChannel{UserID: id, ChannelID: channelID})
	}
	return s.ListChannelAgents(tenantID, channelID)
}

// ---------------- 渠道类型库 ----------------
// 渠道类型完全自定义、不限制固定枚举：租户可新增任意类型并指定名称/颜色/图标。

type ChannelTypeVO struct {
	ID        uint   `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Icon      string `json:"icon"`
	Sort      int    `json:"sort"`
	BuiltIn   bool   `json:"built_in"`
	UsedCount int64  `json:"used_count"`
	CreatedAt string `json:"created_at"`
}

type ChannelTypeRequest struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
	Sort  int    `json:"sort"`
}

// 内置建议类型，首次使用时自动写入租户类型库作为可选预设
var builtinChannelTypes = []struct {
	Code, Name, Color, Icon string
	Sort                    int
}{
	{"wechat", "微信客服", "#07C160", "💬", 1},
	{"wecom", "企业微信", "#2E6BE6", "🏢", 2},
	{"web", "网页客服", "#0BB2D4", "🌐", 3},
	{"app", "APP 客服", "#FF7D41", "📱", 4},
	{"weibo", "微博", "#F5576C", "📕", 5},
	{"douyin", "抖音", "#191919", "🎵", 6},
	{"xiaohongshu", "小红书", "#FF2442", "📒", 7},
	{"kuaishou", "快手", "#FF6600", "⚡", 8},
	{"email", "邮件", "#8B5CF6", "✉️", 9},
	{"robot", "机器人助手", "#8B5CF6", "🤖", 10},
}

// EnsureBuiltinChannelTypes 为租户补齐内置类型（幂等）
func (s *Service) EnsureBuiltinChannelTypes(tenantID uint) {
	for _, b := range builtinChannelTypes {
		if _, err := s.repo.FindChannelTypeByCode(tenantID, b.Code); err == nil {
			continue
		}
		s.repo.InsertChannelType(&model.ChannelType{
			TenantID: tenantID, Code: b.Code, Name: b.Name,
			Color: b.Color, Icon: b.Icon, Sort: b.Sort, BuiltIn: true,
		})
	}
}

func (s *Service) ListChannelTypes(tenantID uint) ([]ChannelTypeVO, error) {
	s.EnsureBuiltinChannelTypes(tenantID)
	list, err := s.repo.ListChannelTypesByTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelTypeVO, 0, len(list))
	for _, t := range list {
		// 使用数统计失败不阻断列表展示，与原逻辑一致
		used, _ := s.repo.CountChannelsOfType(tenantID, t.Code)
		out = append(out, ChannelTypeVO{
			ID: t.ID, Code: t.Code, Name: t.Name, Color: t.Color,
			Icon: t.Icon, Sort: t.Sort, BuiltIn: t.BuiltIn,
			UsedCount: used, CreatedAt: util.FmtTime(t.CreatedAt),
		})
	}
	return out, nil
}

func (s *Service) CreateChannelType(tenantID uint, req ChannelTypeRequest) (*ChannelTypeVO, error) {
	code := strings.TrimSpace(req.Code)
	name := strings.TrimSpace(req.Name)
	if code == "" {
		// 未填 code 时用名称拼音/原名兜底，保证可自定义任意中文类型
		code = name
	}
	if name == "" {
		name = code
	}
	if code == "" {
		return nil, errors.New("渠道类型不能为空")
	}
	if _, err := s.repo.FindChannelTypeByCode(tenantID, code); err == nil {
		return nil, errors.New("该渠道类型已存在")
	}
	color := req.Color
	if color == "" {
		color = "#07C160"
	}
	t := &model.ChannelType{
		TenantID: tenantID, Code: code, Name: name,
		Color: color, Icon: req.Icon, Sort: req.Sort,
	}
	if err := s.repo.InsertChannelType(t); err != nil {
		return nil, err
	}
	return &ChannelTypeVO{
		ID: t.ID, Code: t.Code, Name: t.Name, Color: t.Color,
		Icon: t.Icon, Sort: t.Sort, CreatedAt: util.FmtTime(t.CreatedAt),
	}, nil
}

func (s *Service) UpdateChannelType(tenantID, id uint, req ChannelTypeRequest) error {
	t, err := s.repo.FindChannelTypeByID(tenantID, id)
	if err != nil {
		return errors.New("渠道类型不存在")
	}
	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Code != "" {
		t.Code = req.Code
	}
	if req.Color != "" {
		t.Color = req.Color
	}
	if req.Icon != "" {
		t.Icon = req.Icon
	}
	t.Sort = req.Sort
	return s.repo.SaveChannelType(t)
}

func (s *Service) DeleteChannelType(tenantID, id uint) error {
	t, err := s.repo.FindChannelTypeByID(tenantID, id)
	if err != nil {
		return errors.New("渠道类型不存在")
	}
	// 使用数统计失败按「未被引用」处理，与原逻辑一致
	used, _ := s.repo.CountChannelsOfType(tenantID, t.Code)
	if used > 0 {
		return errors.New("该类型下还有渠道，无法删除")
	}
	return s.repo.RemoveChannelType(tenantID, id)
}

// ---------------- 供其它内核 / 端复用的只读门面 ----------------
// core/conversation 与 modules/client、modules/agent 需要「按主键取一条渠道 / 判断接待绑定」这类单表读取，
// 跨包只能走 Service：这里的每个方法都只是把调用转给本包 repository，SQL 与原来逐字一致。

// FindChannelByID 按租户 + 主键取渠道（跨包只读门面）。
func (s *Service) FindChannelByID(tenantID, id uint) (*model.Channel, error) {
	return s.repo.FindChannelByID(tenantID, id)
}

// ListChannelsByTenant 列出租户的渠道（跨包只读门面）。
func (s *Service) ListChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]*model.Channel, error) {
	return s.repo.ListChannelsByTenant(tenantID, agentID, isAdmin)
}

// HasAgentChannel 判断客服是否配置了接待该渠道（跨包只读门面）。
func (s *Service) HasAgentChannel(userID, channelID uint) (bool, error) {
	return s.repo.HasAgentChannel(userID, channelID)
}

// ListAgentChannelsByChannel 列出某渠道已分配的账号关联（跨包只读门面）。
func (s *Service) ListAgentChannelsByChannel(channelID uint) ([]*model.AgentChannel, error) {
	return s.repo.ListAgentChannelsByChannel(channelID)
}
