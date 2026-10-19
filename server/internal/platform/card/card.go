// Package card 生成聊天里的「卡片」消息体（订单卡片 / 商品卡片）。
//
// 这里只有纯函数、不碰数据库：用户端下单入口、演示数据初始化（platform/seed）
// 都要拼同样的 JSON，放在 platform 是为了让 seed 不必依赖业务模块。
package card

import (
	"encoding/json"

	"kefu-server/internal/platform/model"
	"kefu-server/internal/platform/util"
)

// BuildOrderCard 把订单转成聊天卡片 JSON（供前端发送订单卡片）
func BuildOrderCard(o *model.Order) string {
	card := map[string]interface{}{
		"order_no":   o.OrderNo,
		"title":      o.Title,
		"status":     o.Status,
		"amount":     o.Amount,
		"quantity":   o.Quantity,
		"created_at": util.FmtTime(o.CreatedAt),
		"image":      o.Image,
		"sku":        o.SKU,
	}
	b, _ := json.Marshal(card)
	return string(b)
}

// BuildProductCard 商品卡片 JSON
func BuildProductCard(p *model.Product) string {
	card := map[string]interface{}{
		"id":    p.PID,
		"title": p.Title,
		"price": p.Price,
		"image": p.Image,
		"stock": p.Stock,
		"sku":   p.SKU,
		"url":   p.URL,
	}
	b, _ := json.Marshal(card)
	return string(b)
}
