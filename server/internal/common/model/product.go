package model

// Product 商品（客服可发送商品卡片）
type Product struct {
	Model
	TenantID uint    `gorm:"index;not null" json:"tenant_id"`
	PID      string  `gorm:"size:64;index" json:"id"` // 对外商品 ID，如 P10086
	Title    string  `gorm:"size:255" json:"title"`
	Price    float64 `json:"price"`
	Image    string  `gorm:"size:255" json:"image"`
	Stock    int     `json:"stock"`
	SKU      string  `gorm:"size:128" json:"sku"`
	URL      string  `gorm:"size:255" json:"url"`
}
