// Package setting 租户系统配置（默认图片、提示音等）。
//
// 一开始是 service/setting.go + repository/setting.go 两层，后来整体搬入 platform：
// 公共账号服务的登录响应需要默认图片地址（DefaultImageURL），若留在业务层会形成
// 账号域 ⇄ setting 的环，因此把它当基础设施看待。业务层只调用本包的导出函数。
package setting

import (
	"errors"
	"gorm.io/gorm/clause"
	"strings"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
)

// ---------------- 设置项 Key ----------------

const (
	// SettingDefaultImage 统一默认图片：客户/商品/订单无图时使用
	SettingDefaultImage = "default_image"
	// SettingMessageSound 新消息提示音：on=内置提示音（默认），off=静音，或填音频 URL
	SettingMessageSound = "message_sound"
	// SettingSystemName 系统名称：展示在后台顶栏与页面标题，留空则回退到租户名称
	SettingSystemName = "system.name"
	// SettingObjectStorage 对象存储（S3 兼容）配置：value 是一段 JSON
	SettingObjectStorage = "storage.object"
)

// DefaultObjectStorageJSON 对象存储配置的默认值（未配置时的初值，端点留空由使用者填写）。
// 与 common/service 里 ObjectStorage 的字段一一对应；连接参数的解析与校验在那一边。
const DefaultObjectStorageJSON = `{"enabled":false,"provider":"ctyun","endpoint":"","region":"","bucket":"","access_key":"","secret_key":"","public_base":"","path_prefix":"uploads/"}`

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
	SettingObjectStorage: {
		Value:  DefaultObjectStorageJSON,
		Remark: "对象存储（S3 兼容）：enabled=true 时上传走对象存储，value 为 JSON 配置",
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

	// 用 upsert 一步写入：存在则更新 value（与 remark 同步），不存在则插入。
	// 这样既不走「先查后插」的竞态，也不依赖 findSettingByKey 的查询是否命中。
	row := model.Setting{TenantID: tenantID, Key: key, Value: value}
	if d, ok := settingDefaults[key]; ok {
		row.Remark = d.Remark
	}
	return storage.Default().DB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
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
	// 用结构体条件而不是裸 SQL：`key` 是 MySQL 保留字，写成 "key = ?" 会报语法错，
	// 被上层当成「不存在」→ 走 INSERT → 撞 idx_tenant_key 唯一索引（1062）。GORM 对结构体条件会自动加反引号。
	if err := storage.Default().DB().Where(&model.Setting{TenantID: tenantID, Key: key}).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// listSettingsByTenant 列出租户已保存的全部配置项（保持原有不排序的取数方式）。
func listSettingsByTenant(tenantID uint) ([]*model.Setting, error) {
	var list []*model.Setting
	if err := storage.Default().DB().Where("tenant_id = ?", tenantID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// createSetting 新增配置项，成功后回填主键与时间戳。
func createSetting(s *model.Setting) error {
	return storage.Default().DB().Create(s).Error
}

// saveSetting 保存配置项全字段（已存在配置的更新路径）。
func saveSetting(s *model.Setting) error {
	return storage.Default().DB().Save(s).Error
}
