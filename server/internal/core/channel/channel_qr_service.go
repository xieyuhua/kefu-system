package channel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"kefu-server/internal/platform/config"
	"kefu-server/internal/platform/model"

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
