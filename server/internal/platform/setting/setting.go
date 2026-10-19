// Package setting 租户系统配置（默认图片、提示音等）。
//
// 原先是 service/setting.go + repository/setting.go 两层，整体搬入 platform：
// account 的登录响应需要默认图片地址（DefaultImageURL），若留在 ops 模块会与
// ops → account 形成环，因此把它当基础设施看待。业务模块只调用本包的导出函数。
package setting

import (
	"errors"
	"strings"

	"kefu-server/internal/platform/model"
	"kefu-server/pkg/storage"
)

// ---------------- 设置项 Key ----------------

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

// ---------------- 对外能力 ----------------

// GetSetting 读取配置项，不存在时返回默认值
func GetSetting(tenantID uint, key string) string {
	// 查不到（ErrRecordNotFound）或读库失败都落到默认值，与原逻辑一致
	if s, err := findSettingByKey(tenantID, key); err == nil {
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
	rows := map[string]*model.Setting{}
	list, err := listSettingsByTenant(tenantID)
	if err != nil {
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

	s, err := findSettingByKey(tenantID, key)
	if err != nil {
		// 不存在则新建
		row := model.Setting{TenantID: tenantID, Key: key, Value: value}
		if d, ok := settingDefaults[key]; ok {
			row.Remark = d.Remark
		}
		return createSetting(&row)
	}
	s.Value = value
	return saveSetting(s)
}

// DefaultImageURL 默认图片地址（供前端兜底）
func DefaultImageURL(tenantID uint) string {
	return GetSetting(tenantID, SettingDefaultImage)
}

// MessageSound 新消息提示音配置（on / off / 自定义 URL）
func MessageSound(tenantID uint) string {
	return GetSetting(tenantID, SettingMessageSound)
}

// ---------------- 数据访问（原 repository/setting.go） ----------------

// findSettingByKey 按租户 + key 查询单个配置项；未命中返回 gorm.ErrRecordNotFound。
func findSettingByKey(tenantID uint, key string) (*model.Setting, error) {
	var s model.Setting
	if err := storage.DB().Where("tenant_id = ? AND key = ?", tenantID, key).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// listSettingsByTenant 列出租户已保存的全部配置项（保持原有不排序的取数方式）。
func listSettingsByTenant(tenantID uint) ([]*model.Setting, error) {
	var list []*model.Setting
	if err := storage.DB().Where("tenant_id = ?", tenantID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// createSetting 新增配置项，成功后回填主键与时间戳。
func createSetting(s *model.Setting) error {
	return storage.DB().Create(s).Error
}

// saveSetting 保存配置项全字段（已存在配置的更新路径）。
func saveSetting(s *model.Setting) error {
	return storage.DB().Save(s).Error
}
