package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"kefu-server/internal/config"
	"kefu-server/internal/dto"
	"kefu-server/internal/model"
	"kefu-server/internal/repository"

	qrcode "github.com/skip2/go-qrcode"
)

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
func AccessURLByAppID(tenantID, channelID uint, publicBase string) string {
	c, err := repository.FindChannelByID(tenantID, channelID)
	if err != nil {
		return ""
	}
	return AccessURL(c, publicBase)
}

// ChannelQR 生成渠道接入二维码（PNG bytes / base64）。
// size 为图片边长像素，建议 256~512。
func ChannelQR(tenantID, channelID uint, publicBase string, size int) (*ChannelQRVO, error) {
	c, err := repository.FindChannelByID(tenantID, channelID)
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
func ListChannelAgents(tenantID, channelID uint) ([]dto.ChannelAgentVO, error) {
	if _, err := repository.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	users, err := repository.ListTenantAgents(tenantID)
	if err != nil {
		return nil, err
	}
	selected := map[uint]bool{}
	acs, _ := repository.ListAgentChannelsByChannel(channelID)
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
func SetChannelAgents(tenantID, channelID uint, userIDs []uint) ([]dto.ChannelAgentVO, error) {
	if _, err := repository.FindChannelByID(tenantID, channelID); err != nil {
		return nil, errors.New("渠道不存在")
	}
	// 校验账号归属本租户
	valid := map[uint]bool{}
	if len(userIDs) > 0 {
		// 校验失败按「无账号通过校验」处理，与原逻辑一致
		users, _ := repository.FindAgentUsersByIDs(tenantID, userIDs)
		for _, u := range users {
			valid[u.ID] = true
		}
	}
	// 覆盖式写入：清空旧分配后逐个重建（写入失败由后续列表查询体现，与原逻辑一致）
	_ = repository.DeleteAgentChannels(channelID)
	for _, id := range userIDs {
		if !valid[id] {
			continue
		}
		_ = repository.CreateAgentChannel(&model.AgentChannel{UserID: id, ChannelID: channelID})
	}
	return ListChannelAgents(tenantID, channelID)
}
