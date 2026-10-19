package model

import (
	"time"
)

// 时间格式化统一使用 "2006-01-02 15:04:05"，前端直接展示。
type Model struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
