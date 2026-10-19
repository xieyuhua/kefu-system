package seed

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"kefu-server/internal/auth"
	"kefu-server/internal/config"
	"kefu-server/internal/db"
	"kefu-server/internal/model"
	"kefu-server/internal/service"
)

// Run 初始化演示数据。已存在数据时幂等跳过。
func Run() error {
	var cnt int64
	db.DB.Model(&model.Tenant{}).Count(&cnt)
	if cnt > 0 {
		return nil
	}
	log.Println("[seed] 初始化演示数据 ...")

	// 演示图片
	writeDemoAssets()

	tenant := model.Tenant{Name: "云跳跳科技", Logo: "/demo/logo.svg"}
	if err := db.DB.Create(&tenant).Error; err != nil {
		return err
	}

	// ---- 渠道 ----
	channels := []model.Channel{
		{TenantID: tenant.ID, Type: "wechat", Name: "微信公众号客服", Enabled: true},
		{TenantID: tenant.ID, Type: "wecom", Name: "企业微信客服", Enabled: true},
		{TenantID: tenant.ID, Type: "web", Name: "网页在线客服", Enabled: true},
		{TenantID: tenant.ID, Type: "app", Name: "APP 内客服", Enabled: true},
		{TenantID: tenant.ID, Type: "douyin", Name: "抖音小店", Enabled: true},
		{TenantID: tenant.ID, Type: "robot", Name: "机器人助手", Enabled: true, RobotEnabled: true},
	}
	for i := range channels {
		// 渠道标识（app_id）随机生成，作为接入链接里区分渠道来源的唯一标识
		channels[i].AppID = service.GenerateAppID(tenant.ID)
		db.DB.Create(&channels[i])
	}

	// ---- 账号 ----
	adminHash, _ := auth.HashPassword("admin123")
	admin := model.User{
		TenantID: tenant.ID, Account: "admin", PasswordHash: adminHash,
		Name: "张管理", Role: "admin", Avatar: "/demo/avatar-1.svg",
		MaxConcurrent: 20, Online: true,
	}
	db.DB.Create(&admin)

	kf1Hash, _ := auth.HashPassword("123456")
	kf1 := model.User{
		TenantID: tenant.ID, Account: "kf01", PasswordHash: kf1Hash,
		Name: "小李", Role: "agent", Avatar: "/demo/avatar-2.svg", MaxConcurrent: 5,
	}
	db.DB.Create(&kf1)

	kf2Hash, _ := auth.HashPassword("123456")
	kf2 := model.User{
		TenantID: tenant.ID, Account: "kf02", PasswordHash: kf2Hash,
		Name: "小王", Role: "agent", Avatar: "/demo/avatar-3.svg", MaxConcurrent: 5,
	}
	db.DB.Create(&kf2)

	// 子账号接待渠道：小李→微信/网页；小王→企微/APP/机器人
	assign := func(userID uint, idx ...int) {
		for _, i := range idx {
			db.DB.Create(&model.AgentChannel{UserID: userID, ChannelID: channels[i].ID})
		}
	}
	assign(kf1.ID, 0, 2)
	assign(kf2.ID, 1, 3, 5)

	// ---- 标签 ----
	tags := []model.Tag{
		{TenantID: tenant.ID, Name: "VIP", Color: "red", Sort: 1},
		{TenantID: tenant.ID, Name: "高价值", Color: "orange", Sort: 2},
		{TenantID: tenant.ID, Name: "价格敏感", Color: "blue", Sort: 3},
		{TenantID: tenant.ID, Name: "已退款", Color: "gray", Sort: 4},
		{TenantID: tenant.ID, Name: "需回访", Color: "green", Sort: 5},
	}
	for i := range tags {
		db.DB.Create(&tags[i])
	}

	// ---- 商品 ----
	products := []model.Product{
		{TenantID: tenant.ID, PID: "P10086", Title: "2026春季新款 纯棉圆领T恤", Price: 99.5, Image: "/demo/product-1.svg", Stock: 128, SKU: "白色 / L", URL: "https://example.com/p/P10086"},
		{TenantID: tenant.ID, PID: "P10087", Title: "轻薄透气运动跑鞋", Price: 329.0, Image: "/demo/product-2.svg", Stock: 56, SKU: "黑色 / 42", URL: "https://example.com/p/P10087"},
		{TenantID: tenant.ID, PID: "P10088", Title: "无线蓝牙降噪耳机", Price: 499.0, Image: "/demo/product-3.svg", Stock: 12, SKU: "星空灰", URL: "https://example.com/p/P10088"},
		{TenantID: tenant.ID, PID: "P10089", Title: "不锈钢保温杯 500ml", Price: 79.0, Image: "/demo/product-4.svg", Stock: 300, SKU: "雾霾蓝", URL: "https://example.com/p/P10089"},
	}
	for i := range products {
		db.DB.Create(&products[i])
	}

	// ---- 客户 + 订单 + 会话 + 消息 ----
	names := []string{"王小明", "李婷婷", "张浩然", "刘小雨", "陈先生"}
	for i, name := range names {
		ch := channels[i%len(channels)]
		cu := model.Customer{
			TenantID: tenant.ID, ChannelID: ch.ID,
			ExternalID: fmt.Sprintf("demo_%s_%d", ch.Type, 1000+i),
			Nickname:   name,
			Avatar:     fmt.Sprintf("/demo/avatar-%d.svg", i%6+1),
			Phone:      fmt.Sprintf("138****%04d", 1000+i),
		}
		now := time.Now().Add(-time.Duration(i) * time.Hour)
		cu.LastSeenAt = &now
		db.DB.Create(&cu)

		// 标签
		if i == 0 {
			db.DB.Create(&model.CustomerTag{TenantID: tenant.ID, CustomerID: cu.ID, TagID: tags[0].ID, CreatedBy: admin.ID, CreatedAt: now})
			db.DB.Create(&model.CustomerTag{TenantID: tenant.ID, CustomerID: cu.ID, TagID: tags[1].ID, CreatedBy: admin.ID, CreatedAt: now})
		}
		if i == 1 {
			db.DB.Create(&model.CustomerTag{TenantID: tenant.ID, CustomerID: cu.ID, TagID: tags[2].ID, CreatedBy: kf1.ID, CreatedAt: now})
		}

		// 历史订单
		for j := 0; j <= i%3; j++ {
			p := products[(i+j)%len(products)]
			paid := now.Add(-time.Duration(j+1) * 24 * time.Hour)
			o := model.Order{
				TenantID: tenant.ID, CustomerID: cu.ID,
				OrderNo: fmt.Sprintf("SO2026%02d%02d%03d", 3, i+1, j+1),
				Title:   p.Title, Status: []string{"已发货", "待付款", "已完成", "退款中"}[j%4],
				Amount: p.Price * float64(j+1), Quantity: j + 1,
				Image: p.Image, SKU: p.SKU, PaidAt: &paid,
			}
			db.DB.Create(&o)
		}

		// 会话：前两个进行中，其余已结束
		status := "open"
		agentID := kf1.ID
		if i >= 2 {
			status = "closed"
			agentID = kf2.ID
		}
		if i == 4 {
			status = "pending"
			agentID = 0
		}
		conv := model.Conversation{
			TenantID: tenant.ID, ChannelID: ch.ID, CustomerID: cu.ID,
			AgentID: agentID, Status: status,
			LastMessage: "你好，我想咨询一下", LastMessageAt: now,
			Unread: i % 3, Summary: "咨询商品与发货",
		}
		if status == "closed" {
			closedAt := now.Add(30 * time.Minute)
			conv.ClosedAt = &closedAt
		}
		db.DB.Create(&conv)

		// 会话消息（CreatedAt 在 Create 后单独更新，避免被 GORM 自动填充覆盖）
		type seedMsg struct {
			m  model.Message
			at time.Time
		}
		msgs := []seedMsg{
			{model.Message{TenantID: tenant.ID, ConversationID: conv.ID, SenderID: cu.ID, SenderRole: "customer", Type: "text", Direction: "in", Content: "你好，在吗？"}, now.Add(-20 * time.Minute)},
			{model.Message{TenantID: tenant.ID, ConversationID: conv.ID, SenderID: agentID, SenderRole: "user", Type: "text", Direction: "out", Content: "您好，很高兴为您服务～", Read: true}, now.Add(-19 * time.Minute)},
			{model.Message{TenantID: tenant.ID, ConversationID: conv.ID, SenderID: cu.ID, SenderRole: "customer", Type: "text", Direction: "in", Content: "我想问一下这个什么时候发货？"}, now.Add(-18 * time.Minute)},
		}
		if i%2 == 0 {
			var o model.Order
			db.DB.Where("customer_id = ?", cu.ID).First(&o)
			if o.ID > 0 {
				msgs = append(msgs, seedMsg{model.Message{
					TenantID: tenant.ID, ConversationID: conv.ID, SenderID: cu.ID,
					SenderRole: "customer", Type: "order", Direction: "in",
					Content: service.BuildOrderCard(&o),
				}, now.Add(-17 * time.Minute)})
			}
			p := products[i%len(products)]
			msgs = append(msgs, seedMsg{model.Message{
				TenantID: tenant.ID, ConversationID: conv.ID, SenderID: agentID,
				SenderRole: "user", Type: "product", Direction: "out",
				Content: service.BuildProductCard(&p), Read: true,
			}, now.Add(-16 * time.Minute)})
		}
		msgs = append(msgs, seedMsg{model.Message{
			TenantID: tenant.ID, ConversationID: conv.ID,
			SenderRole: "system", Type: "system", Direction: "system",
			Content: "客服" + map[uint]string{kf1.ID: "小李", kf2.ID: "小王"}[agentID] + " 已接入会话",
			Read:    true,
		}, now.Add(-15 * time.Minute)})

		for mi := range msgs {
			msg := msgs[mi].m
			if err := db.DB.Create(&msg).Error; err == nil {
				db.DB.Model(&model.Message{}).Where("id = ?", msg.ID).
					Update("created_at", msgs[mi].at)
			}
			conv.MessageCount++
		}
		db.DB.Save(&conv)
	}

	// ---- 自动回复规则 ----
	all := uint(0)
	transferTrue := true
	replies := []model.AutoReply{
		{TenantID: tenant.ID, ChannelID: all, Name: "问候语", MatchType: "contain",
			Keywords: "你好,在吗,hi,hello", Reply: "您好，我是智能客服助手，很高兴为您服务～",
			ReplyType: "text", Enabled: true, Priority: 10},
		{TenantID: tenant.ID, ChannelID: all, Name: "发货咨询", MatchType: "contain",
			Keywords: "发货,物流,快递,什么时候到", Reply: "订单付款后 48 小时内发出，物流单号可在「我的订单」查看哦～",
			ReplyType: "text", Enabled: true, Priority: 20},
		{TenantID: tenant.ID, ChannelID: all, Name: "退换货", MatchType: "contain",
			Keywords: "退款,退货,换货", Reply: "支持 7 天无理由退货，请在订单详情页点击「申请退款」。",
			ReplyType: "text", Enabled: true, Priority: 30},
		{TenantID: tenant.ID, ChannelID: all, Name: "转人工", MatchType: "contain",
			Keywords: "人工,转人工,客服,投诉", Reply: "", ReplyType: "text",
			TransferToHuman: true, Enabled: true, Priority: 100},
	}
	for i := range replies {
		db.DB.Create(&replies[i])
	}
	_ = transferTrue

	log.Println("[seed] 演示数据初始化完成")
	log.Println("[seed] 账号：admin/admin123（管理员）、kf01/123456、kf02/123456")
	_ = rand.Intn
	_ = os.MkdirAll
	return nil
}

// writeDemoAssets 生成内置演示 SVG 资源。
// 注意：必须在每次启动时都执行（幂等覆盖写），不能只在首次种子时执行。
// 否则 data/demo 被清理后，/demo/*.svg 会 404 并被 SPA 回退成 HTML，
// 前端所有头像/商品图/订单图表现为「图片加载异常」。
func writeDemoAssets() {
	dir := filepath.Join("data", "demo")
	_ = os.MkdirAll(dir, 0755)

	colors := []string{"#07C160", "#2E6BE6", "#FF7D41", "#8B5CF6", "#F5576C", "#0BB2D4"}
	// 头像
	for i := 0; i < 6; i++ {
		svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80"><rect width="80" height="80" rx="40" fill="%s"/><text x="40" y="52" font-size="34" fill="#fff" text-anchor="middle" font-family="sans-serif">%d</text></svg>`, colors[i], i+1)
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("avatar-%d.svg", i+1)), []byte(svg), 0644)
	}
	// 商品图
	for i := 0; i < 4; i++ {
		svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120"><rect width="120" height="120" rx="8" fill="%s" opacity="0.85"/><text x="60" y="70" font-size="40" fill="#fff" text-anchor="middle" font-family="sans-serif">%d</text></svg>`, colors[i], i+1)
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("product-%d.svg", i+1)), []byte(svg), 0644)
	}
	// logo
	logo := `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80"><rect width="80" height="80" rx="16" fill="#07C160"/><text x="40" y="54" font-size="34" fill="#fff" text-anchor="middle" font-family="sans-serif">云</text></svg>`
	_ = os.WriteFile(filepath.Join(dir, "logo.svg"), []byte(logo), 0644)

	_ = config.C
}
