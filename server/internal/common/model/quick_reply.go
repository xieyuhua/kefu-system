package model

// QuickReply 快捷回复（常用语）：客服自己的常用话术，工作台一键插入输入框。
type QuickReply struct {
	Model
	TenantID uint   `gorm:"index;not null" json:"tenant_id"`
	UserID   uint   `gorm:"index" json:"user_id"`     // 0 = 租户共享（管理员维护）；>0 = 该客服私有
	Title    string `gorm:"size:64" json:"title"`     // 标题/快捷名，列表展示
	Content  string `gorm:"type:text" json:"content"` // 回复正文（纯文本）
	Sort     int    `gorm:"default:0" json:"sort"`    // 排序，小的在前
	Enabled  bool   `gorm:"default:true" json:"enabled"`
}
