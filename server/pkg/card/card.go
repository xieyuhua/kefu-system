// Package card 生成聊天里的「卡片」消息体（订单卡片 / 商品卡片）。
//
// 纯函数式、不碰数据库，也不认识 internal 的 model：调用方把实体映射成
// OrderInput / ProductInput 后传进来（字段与卡片用到的字段一一对应），
// 这样用户端、演示数据初始化都能拼出同样的 JSON，而本包可以独立成 SDK。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewBuilder 与单例访问器 Default，
// 业务能力一律是 *Builder 的指针接收者方法；调用点写 card.Default().BuildOrderCard(...)。
package card

import (
	"encoding/json"
	"time"

	"kefu-server/pkg/util"
)

// OrderInput 订单卡片入参（与订单卡片用到的字段一一对应）。
type OrderInput struct {
	OrderNo   string
	Title     string
	Status    string
	Amount    float64
	Quantity  int
	CreatedAt time.Time
	Image     string
	SKU       string
}

// ProductInput 商品卡片入参（与商品卡片用到的字段一一对应）。
type ProductInput struct {
	PID   string
	Title string
	Price float64
	Image string
	Stock int
	SKU   string
	URL   string
}

// Builder 卡片构造器。无状态，所有卡片都是它的方法。
type Builder struct{}

// NewBuilder 创建卡片构造器实例。
func NewBuilder() *Builder { return &Builder{} }

// defaultBuilder 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultBuilder = NewBuilder()

// Default 返回包级默认实例。
func Default() *Builder { return defaultBuilder }

// BuildOrderCard 把订单转成聊天卡片 JSON（供前端发送订单卡片）
func (b *Builder) BuildOrderCard(o OrderInput) string {
	card := map[string]interface{}{
		"order_no":   o.OrderNo,
		"title":      o.Title,
		"status":     o.Status,
		"amount":     o.Amount,
		"quantity":   o.Quantity,
		"created_at": util.Default().FmtTime(o.CreatedAt),
		"image":      o.Image,
		"sku":        o.SKU,
	}
	b2, _ := json.Marshal(card)
	return string(b2)
}

// BuildProductCard 商品卡片 JSON
func (b *Builder) BuildProductCard(p ProductInput) string {
	card := map[string]interface{}{
		"id":    p.PID,
		"title": p.Title,
		"price": p.Price,
		"image": p.Image,
		"stock": p.Stock,
		"sku":   p.SKU,
		"url":   p.URL,
	}
	b2, _ := json.Marshal(card)
	return string(b2)
}
