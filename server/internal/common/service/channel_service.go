package service

// ---------------- 渠道域（Channel） ----------------
// channels、channel_types、渠道二维码 / 接入地址、渠道 ↔ 接待账号分配。
//
// 形态约定：业务编排都是 ChannelService 的方法（指针接收者），构造时建好渠道与渠道类型两个仓储；
// 只有无状态的纯函数（AccessPath / ChannelKey / AccessURL）保留为包级函数。
import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
	"kefu-server/pkg/dto"
	"kefu-server/pkg/idgen"
	"kefu-server/pkg/util"
)

// ChannelService 渠道内核的业务编排层。
// 同包内渠道表与渠道类型库分属两个仓储：repo 管 channels / agent_channels，
// types 管 channel_types（原 channel 包只有一个 Repository 类型，合并后按子域拆开）。
type ChannelService struct {
	repo  *repository.ChannelRepository
	types *repository.ChannelTypeRepository
}

// NewChannelService 创建业务层实例（自带数据访问层依赖）。
func NewChannelService() *ChannelService {
	return &ChannelService{
		repo:  repository.NewChannelRepository(),
		types: repository.NewChannelTypeRepository(),
	}
}

// ---------------- Channel ----------------

func (s *ChannelService) ListChannels(tenantID uint, agentID uint, isAdmin bool) ([]dto.ChannelVO, error) {
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
			SupportOrderCard: c.SupportOrderCard, SupportProductCard: c.SupportProductCard,
			CreatedAt: util.Default().FmtTime(c.CreatedAt), AgentCount: agentCount, PendingCount: pending,
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

// ChannelRefOf 把渠道实体映射成会话 / 客户 VO 里的渠道精简对象（纯函数）。
// 卡片能力开关随会话一起下发，客服工作台据此判断「发送订单 / 发送商品」入口是否可用。
func ChannelRefOf(c *model.Channel) *dto.ChannelRef {
	if c == nil {
		return nil
	}
	return &dto.ChannelRef{
		ID: c.ID, Type: c.Type, Name: c.Name,
		SupportOrderCard: c.SupportOrderCard, SupportProductCard: c.SupportProductCard,
	}
}

func (s *ChannelService) CreateChannel(tenantID uint, req dto.ChannelRequest) (*dto.ChannelVO, error) {
	if req.Name == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	// 渠道类型不限制枚举，可任意自定义（如 wechat / wecom / web / 小红书 / 快手 ...）
	if req.Type == "" {
		req.Type = "custom"
	}
	// 卡片能力开关：创建时未传（nil）视为开启，与存量渠道默认行为一致
	wantOrderCard := req.SupportOrderCard == nil || *req.SupportOrderCard
	wantProductCard := req.SupportProductCard == nil || *req.SupportProductCard
	c := &model.Channel{
		TenantID:           tenantID,
		Type:               req.Type,
		Name:               req.Name,
		AppID:              strings.TrimSpace(req.AppID),
		Enabled:            true,
		RobotEnabled:       req.RobotEnabled != nil && *req.RobotEnabled,
		HideHeader:         req.HideHeader != nil && *req.HideHeader,
		HideDebug:          req.HideDebug != nil && *req.HideDebug,
		HideMoreMenu:       req.HideMoreMenu != nil && *req.HideMoreMenu,
		SupportOrderCard:   wantOrderCard,
		SupportProductCard: wantProductCard,
	}
	// app_id 未指定时自动生成一个随机标识（可在渠道管理里重置）
	if c.AppID == "" {
		c.AppID = idgen.Default().GenerateAppID(s.appIDExists(tenantID))
	} else if err := s.ensureAppIDUnique(tenantID, c.AppID, 0); err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if err := s.repo.InsertChannel(c); err != nil {
		return nil, err
	}
	// 两个开关的列带 default:true：GORM 在 Create 时会把零值（false）替换成标签默认值
	// 并回写到结构体，导致「新建渠道时就关闭某类卡片」被悄悄改成开启。
	// 这里按业务意图校正并补一次全字段保存（仅在需要关闭时才多这一条 UPDATE）。
	if c.SupportOrderCard != wantOrderCard || c.SupportProductCard != wantProductCard {
		c.SupportOrderCard = wantOrderCard
		c.SupportProductCard = wantProductCard
		if err := s.repo.SaveChannel(c); err != nil {
			return nil, err
		}
	}
	return &dto.ChannelVO{
		ID: c.ID, Type: c.Type, Name: c.Name, AppID: c.AppID,
		Enabled: c.Enabled, RobotEnabled: c.RobotEnabled,
		HideHeader: c.HideHeader, HideDebug: c.HideDebug, HideMoreMenu: c.HideMoreMenu,
		SupportOrderCard: c.SupportOrderCard, SupportProductCard: c.SupportProductCard,
		CreatedAt: util.Default().FmtTime(c.CreatedAt),
	}, nil
}

// appIDExists 返回「同租户内 app_id 是否已被占用」的判定闭包。
// pkg/idgen 是可复用 SDK、不认识 channels 表，唯一性校验由业务层以闭包注入。
func (s *ChannelService) appIDExists(tenantID uint) func(id string) bool {
	return func(id string) bool {
		// 统计失败按「未被占用」处理，与原逻辑一致
		cnt, _ := s.repo.CountChannelByAppID(tenantID, id, 0)
		return cnt > 0
	}
}

// ensureAppIDUnique 保证同一租户内 app_id 唯一（excludeID 用于更新场景排除自身）
func (s *ChannelService) ensureAppIDUnique(tenantID uint, appID string, excludeID uint) error {
	// 统计失败按「未被占用」处理，与原逻辑一致
	cnt, _ := s.repo.CountChannelByAppID(tenantID, appID, excludeID)
	if cnt > 0 {
		return errors.New("该渠道标识（app_id）已被占用")
	}
	return nil
}

// ResetChannelAppID 重置渠道的 app_id（生成新的随机标识）
// 注意：重置后旧链接会失效，需要重新分发二维码/链接。
func (s *ChannelService) ResetChannelAppID(tenantID, id uint) (string, error) {
	c, err := s.repo.FindChannelByID(tenantID, id)
	if err != nil {
		return "", errors.New("渠道不存在")
	}
	for i := 0; i < 5; i++ {
		newID := idgen.Default().GenerateAppID(s.appIDExists(tenantID))
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

func (s *ChannelService) UpdateChannel(tenantID, id uint, req dto.ChannelRequest) error {
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
	// 卡片能力开关：按指针语义写库，nil 表示本次不修改（可单独切换其中一项）
	if req.SupportOrderCard != nil {
		c.SupportOrderCard = *req.SupportOrderCard
	}
	if req.SupportProductCard != nil {
		c.SupportProductCard = *req.SupportProductCard
	}
	return s.repo.SaveChannel(c)
}

func (s *ChannelService) DeleteChannel(tenantID, id uint) error {
	return s.repo.DeleteChannelWithBindings(tenantID, id)
}

// ---------------- 供其它域 / 端复用的只读门面 ----------------
// 同包内的会话域与 client / agent 两端需要「按主键取一条渠道 / 判断接待绑定」这类单表读取，
// 跨包调用只能走 Service：这里的每个方法都只是把调用转给本包仓储，SQL 与原来逐字一致。

// FindChannelByID 按租户 + 主键取渠道（跨包只读门面）。
func (s *ChannelService) FindChannelByID(tenantID, id uint) (*model.Channel, error) {
	return s.repo.FindChannelByID(tenantID, id)
}

// ListChannelsByTenant 列出租户的渠道（跨包只读门面）。
func (s *ChannelService) ListChannelsByTenant(tenantID, agentID uint, isAdmin bool) ([]*model.Channel, error) {
	return s.repo.ListChannelsByTenant(tenantID, agentID, isAdmin)
}
