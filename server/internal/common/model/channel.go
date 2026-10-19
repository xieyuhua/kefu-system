package model

// Channel 渠道（微信/企微/网页/APP/微博/抖音/机器人）
type Channel struct {
	Model
	TenantID     uint   `gorm:"index;not null" json:"tenant_id"`
	Type         string `gorm:"size:32;not null" json:"type"`
	Name         string `gorm:"size:64;not null" json:"name"`
	AppID        string `gorm:"size:128;index" json:"app_id"` // 渠道唯一标识：接入链接用它区分渠道来源与接待
	Enabled      bool   `gorm:"default:true" json:"enabled"`
	RobotEnabled bool   `gorm:"default:false" json:"robot_enabled"`

	// 用户端展示控制（APP WebView 内嵌等场景）
	HideHeader   bool `gorm:"default:false" json:"hide_header"`    // 隐藏顶部 header
	HideDebug    bool `gorm:"default:false" json:"hide_debug"`     // 隐藏「接入调试」入口
	HideMoreMenu bool `gorm:"default:false" json:"hide_more_menu"` // 隐藏「更多」菜单入口
}
