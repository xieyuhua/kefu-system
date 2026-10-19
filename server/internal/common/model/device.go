package model

// Device 推送设备（APP / PC 桌面端预留）
type Device struct {
	Model
	TenantID  uint   `gorm:"index" json:"tenant_id"`
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	Device    string `gorm:"size:16" json:"device"`   // app | desktop | web
	Platform  string `gorm:"size:16" json:"platform"` // ios | android | windows | mac
	PushToken string `gorm:"size:255" json:"push_token"`
	Name      string `gorm:"size:64" json:"name"`
}
