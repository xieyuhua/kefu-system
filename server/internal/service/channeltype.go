package service

import (
	"errors"
	"strings"

	"kefu-server/internal/model"
)

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
func EnsureBuiltinChannelTypes(tenantID uint) {
	for _, b := range builtinChannelTypes {
		var exist model.ChannelType
		if err := gormDB().Where("tenant_id = ? AND code = ?", tenantID, b.Code).First(&exist).Error; err == nil {
			continue
		}
		gormDB().Create(&model.ChannelType{
			TenantID: tenantID, Code: b.Code, Name: b.Name,
			Color: b.Color, Icon: b.Icon, Sort: b.Sort, BuiltIn: true,
		})
	}
}

func ListChannelTypes(tenantID uint) ([]ChannelTypeVO, error) {
	EnsureBuiltinChannelTypes(tenantID)
	var list []model.ChannelType
	if err := gormDB().Where("tenant_id = ?", tenantID).
		Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]ChannelTypeVO, 0, len(list))
	for _, t := range list {
		var used int64
		gormDB().Model(&model.Channel{}).
			Where("tenant_id = ? AND type = ?", tenantID, t.Code).Count(&used)
		out = append(out, ChannelTypeVO{
			ID: t.ID, Code: t.Code, Name: t.Name, Color: t.Color,
			Icon: t.Icon, Sort: t.Sort, BuiltIn: t.BuiltIn,
			UsedCount: used, CreatedAt: fmtTime(t.CreatedAt),
		})
	}
	return out, nil
}

func CreateChannelType(tenantID uint, req ChannelTypeRequest) (*ChannelTypeVO, error) {
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
	var exist model.ChannelType
	if gormDB().Where("tenant_id = ? AND code = ?", tenantID, code).First(&exist).Error == nil {
		return nil, errors.New("该渠道类型已存在")
	}
	color := req.Color
	if color == "" {
		color = "#07C160"
	}
	t := model.ChannelType{
		TenantID: tenantID, Code: code, Name: name,
		Color: color, Icon: req.Icon, Sort: req.Sort,
	}
	if err := gormDB().Create(&t).Error; err != nil {
		return nil, err
	}
	return &ChannelTypeVO{
		ID: t.ID, Code: t.Code, Name: t.Name, Color: t.Color,
		Icon: t.Icon, Sort: t.Sort, CreatedAt: fmtTime(t.CreatedAt),
	}, nil
}

func UpdateChannelType(tenantID, id uint, req ChannelTypeRequest) error {
	var t model.ChannelType
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&t).Error; err != nil {
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
	return gormDB().Save(&t).Error
}

func DeleteChannelType(tenantID, id uint) error {
	var t model.ChannelType
	if err := gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).First(&t).Error; err != nil {
		return errors.New("渠道类型不存在")
	}
	var used int64
	gormDB().Model(&model.Channel{}).
		Where("tenant_id = ? AND type = ?", tenantID, t.Code).Count(&used)
	if used > 0 {
		return errors.New("该类型下还有渠道，无法删除")
	}
	return gormDB().Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&model.ChannelType{}).Error
}
