package service

// ---------------- 卡片入参映射 ----------------
// pkg/card 是可复用 SDK，不认识 internal 的 model（也不该认识）。
// 实体 → 卡片入参的映射属于业务层：这里按卡片用到的字段一一对应地转换，
// 卡片 JSON 的字段名、顺序、内容与改造前完全一致。

import (
	"kefu-server/internal/common/model"
	"kefu-server/pkg/card"
)

// orderCardInput 把订单实体映射成订单卡片入参。
func orderCardInput(o *model.Order) card.OrderInput {
	return card.OrderInput{
		OrderNo:   o.OrderNo,
		Title:     o.Title,
		Status:    o.Status,
		Amount:    o.Amount,
		Quantity:  o.Quantity,
		CreatedAt: o.CreatedAt,
		Image:     o.Image,
		SKU:       o.SKU,
	}
}

// productCardInput 把商品实体映射成商品卡片入参。
func productCardInput(p *model.Product) card.ProductInput {
	return card.ProductInput{
		PID:   p.PID,
		Title: p.Title,
		Price: p.Price,
		Image: p.Image,
		Stock: p.Stock,
		SKU:   p.SKU,
		URL:   p.URL,
	}
}

// BuildOrderCard 把订单实体转成聊天订单卡片 JSON。
func (s *CustomerService) BuildOrderCard(o *model.Order) string {
	return card.Default().BuildOrderCard(orderCardInput(o))
}

// BuildProductCard 把商品实体转成聊天商品卡片 JSON。
func (s *CustomerService) BuildProductCard(p *model.Product) string {
	return card.Default().BuildProductCard(productCardInput(p))
}
