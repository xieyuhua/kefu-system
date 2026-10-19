package seed

// 演示数据里的订单 / 商品卡片。
//
// pkg/card 是可复用 SDK、不认识 internal 的 model，实体 → 卡片入参的映射由调用方完成。
// 这里没有复用 internal/common/service 的同名映射，是为了让 seed（只管写演示数据）
// 不反向依赖业务层；字段与卡片用到的字段一一对应，卡片内容与改造前逐字一致。

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/card"
)

// buildOrderCard 把演示订单转成聊天订单卡片 JSON。
func buildOrderCard(o *model.Order) string {
	return card.Default().BuildOrderCard(card.OrderInput{
		OrderNo:   o.OrderNo,
		Title:     o.Title,
		Status:    o.Status,
		Amount:    o.Amount,
		Quantity:  o.Quantity,
		CreatedAt: o.CreatedAt,
		Image:     o.Image,
		SKU:       o.SKU,
	})
}

// buildProductCard 把演示商品转成聊天商品卡片 JSON。
func buildProductCard(p *model.Product) string {
	return card.Default().BuildProductCard(card.ProductInput{
		PID:   p.PID,
		Title: p.Title,
		Price: p.Price,
		Image: p.Image,
		Stock: p.Stock,
		SKU:   p.SKU,
		URL:   p.URL,
	})
}
