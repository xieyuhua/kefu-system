package model

import (
	"time"
)

// Order 历史订单
type Order struct {
	Model
	TenantID   uint       `gorm:"index;not null" json:"tenant_id"`
	CustomerID uint       `gorm:"index" json:"customer_id"`
	OrderNo    string     `gorm:"size:64;index" json:"order_no"`
	Title      string     `gorm:"size:255" json:"title"`
	Status     string     `gorm:"size:32" json:"status"`
	Amount     float64    `json:"amount"`
	Quantity   int        `json:"quantity"`
	Image      string     `gorm:"size:255" json:"image"`
	SKU        string     `gorm:"size:128" json:"sku"`
	PaidAt     *time.Time `json:"paid_at"`
}
