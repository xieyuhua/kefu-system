// Package channel 渠道内核：channels、channel_types、渠道二维码 / 接入地址、
// 渠道 ↔ 接待账号分配。模块内按 service / repository 分层，HTTP 层在各端模块。
//
// 依赖方向：core/channel → platform → pkg（不依赖任何业务端模块，也不依赖其它内核包）。
//
// 形态约定：业务编排都是 Service 的方法（指针接收者），Service 在构造时建好自己的
// Repository 依赖；只有无状态的纯函数（AccessPath / ChannelKey / AccessURL）保留为包级函数。
package channel

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"kefu-server/internal/platform/dto"
	"kefu-server/internal/platform/idgen"
	"kefu-server/internal/platform/model"
	"kefu-server/pkg/util"
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
