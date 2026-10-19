package model

// User 后台账号：租户管理员 admin 或客服子账号 agent
type User struct {
	Model
	TenantID      uint   `gorm:"index;not null" json:"tenant_id"`
	Account       string `gorm:"size:64;uniqueIndex;not null" json:"account"`
	PasswordHash  string `gorm:"size:255;not null" json:"-"`
	Name          string `gorm:"size:64" json:"name"`
	Avatar        string `gorm:"size:255" json:"avatar"`
	Role          string `gorm:"size:16;default:'agent'" json:"role"` // admin | agent
	Online        bool   `gorm:"default:false" json:"online"`
	Enabled       bool   `gorm:"default:true" json:"enabled"` // 停用后不可登录、不可接待
	MaxConcurrent int    `gorm:"default:5" json:"max_concurrent"`
}
