package service

import (
	"errors"
	"strings"

	"kefu-server/internal/model"
)

// ---------------- 租户系统配置（默认图片等） ----------------

// 设置项 Key
const (
	// SettingDefaultImage 统一默认图片：客户/商品/订单无图时使用
	SettingDefaultImage = "default_image"
	// SettingMessageSound 新消息提示音：on=内置提示音（默认），off=静音，或填音频 URL
	SettingMessageSound = "message_sound"
)

// defaultImageFallback 前端未配置时使用的内置兜底图
const defaultImageFallback = "/img/placeholder.svg"

var settingDefaults = map[string]struct {
	Value  string
	Remark string
}{
	SettingDefaultImage: {
		Value:  defaultImageFallback,
		Remark: "客户 / 商品 / 订单没有图片时展示的默认图片（留空则使用内置兜底图）",
	},
	SettingMessageSound: {
		Value:  "on",
		Remark: "新消息提示音：on=内置提示音（默认），off=静音，或填自定义音频 URL",
	},
}

// GetSetting 读取配置项，不存在时返回默认值
func GetSetting(tenantID uint, key string) string {
	var s model.Setting
	if err := gormDB().Where("tenant_id = ? AND key = ?", tenantID, key).First(&s).Error; err == nil {
		if v := strings.TrimSpace(s.Value); v != "" {
			return v
		}
	}
	if d, ok := settingDefaults[key]; ok {
		return d.Value
	}
	return ""
}

// ListSettings 返回全部配置项（含默认值，便于后台渲染表单）
func ListSettings(tenantID uint) ([]map[string]interface{}, error) {
	rows := map[string]model.Setting{}
	var list []model.Setting
	if err := gormDB().Where("tenant_id = ?", tenantID).Find(&list).Error; err != nil {
		return nil, err
	}
	for _, s := range list {
		rows[s.Key] = s
	}

	// 以默认项为基础，叠加已保存值，再补充自定义项
	keys := make([]string, 0, len(settingDefaults)+len(rows))
	seen := map[string]bool{}
	for k := range settingDefaults {
		keys = append(keys, k)
		seen[k] = true
	}
	for _, s := range list {
		if !seen[s.Key] {
			keys = append(keys, s.Key)
			seen[s.Key] = true
		}
	}

	out := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		val := GetSetting(tenantID, k)
		remark := ""
		if d, ok := settingDefaults[k]; ok {
			remark = d.Remark
		} else if s, ok := rows[k]; ok {
			remark = s.Remark
		}
		out = append(out, map[string]interface{}{
			"key":    k,
			"value":  val,
			"remark": remark,
		})
	}
	return out, nil
}

// SaveSetting 保存单个配置项
func SaveSetting(tenantID uint, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("配置项不能为空")
	}
	value = strings.TrimSpace(value)

	var s model.Setting
	err := gormDB().Where("tenant_id = ? AND key = ?", tenantID, key).First(&s).Error
	if err != nil {
		// 不存在则新建
		s = model.Setting{TenantID: tenantID, Key: key, Value: value}
		if d, ok := settingDefaults[key]; ok {
			s.Remark = d.Remark
		}
		return gormDB().Create(&s).Error
	}
	s.Value = value
	return gormDB().Save(&s).Error
}

// DefaultImageURL 默认图片地址（供前端兜底）
func DefaultImageURL(tenantID uint) string {
	return GetSetting(tenantID, SettingDefaultImage)
}

// MessageSound 新消息提示音配置（on / off / 自定义 URL）
func MessageSound(tenantID uint) string {
	return GetSetting(tenantID, SettingMessageSound)
}
