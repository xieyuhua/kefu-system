package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"
)

// ---------------- Channel ----------------

func ListChannels(tenantID uint, agentID uint, isAdmin bool) ([]dto.ChannelVO, error) {
	list, err := repository.ListChannelsByTenant(tenantID, agentID, isAdmin)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ChannelVO, 0, len(list))
	for _, c := range list {
		// 角标统计失败不阻断列表展示，与原逻辑一致
		agentCount, _ := repository.CountAgentsOfChannel(c.ID)
		pending, _ := repository.CountPendingByChannel(tenantID, c.ID)
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
		// 查询失败按「未占用」处理，保证一定能返回标识（与原逻辑一致）
		cnt, _ := repository.CountChannelByAppID(tenantID, id, 0)
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
	if err := repository.CreateChannel(&c); err != nil {
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
	// 统计失败按「未被占用」处理，与原逻辑一致
	cnt, _ := repository.CountChannelByAppID(tenantID, appID, excludeID)
	if cnt > 0 {
		return errors.New("该渠道标识（app_id）已被占用")
	}
	return nil
}

// ResetChannelAppID 重置渠道的 app_id（生成新的随机标识）
// 注意：重置后旧链接会失效，需要重新分发二维码/链接。
func ResetChannelAppID(tenantID, id uint) (string, error) {
	c, err := repository.FindChannelByID(tenantID, id)
	if err != nil {
		return "", errors.New("渠道不存在")
	}
	for i := 0; i < 5; i++ {
		newID := GenerateAppID(tenantID)
		if newID == c.AppID {
			continue
		}
		c.AppID = newID
		if err := repository.SaveChannel(c); err != nil {
			return "", err
		}
		return newID, nil
	}
	return "", errors.New("生成渠道标识失败，请重试")
}

func UpdateChannel(tenantID, id uint, req dto.ChannelRequest) error {
	c, err := repository.FindChannelByID(tenantID, id)
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
	return repository.SaveChannel(c)
}

func DeleteChannel(tenantID, id uint) error {
	return repository.DeleteChannelWithBindings(tenantID, id)
}
