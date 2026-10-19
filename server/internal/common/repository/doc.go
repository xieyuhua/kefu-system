// Package repository 三端共用的数据访问层。
//
// 原先按子域分散在 internal/core/<域>/<子域>_repository.go，现在合并成同一个包，
// 每个子域一个仓储类型（原先同一个 core 包里的所有方法都挂在一个 Repository 类型上）：
//
//	UserRepository                   users / agent_channels / tenants（账号域）
//	ChannelRepository                channels / agent_channels（渠道维度）
//	ChannelTypeRepository            channel_types
//	ConversationRepository           conversations
//	ConversationVisibilityRepository 会话可见范围的条件拼装
//	MessageRepository                messages
//	CustomerRepository               customers
//	OrderRepository                  orders
//	ProductRepository                products
//	TagRepository                    tags / customer_tags
//	RefsRepository                   客户域的跨表只读（渠道 / 账号展示字段、兜底租户）
//	AutoReplyRepository              auto_replies
//	DeviceRepository                 devices
//	NotificationRepository           notifications
//	StatsRepository                  概览统计的各条计数 SQL
//
// 依赖方向：common/repository → common/model → platform → pkg（不 import 任何端与 service）。
//
// 形态约定：所有数据访问都是 *XxxRepository 的方法（指针接收者），参数与返回值用
// *model.X / []*model.X；仓储不持有 *gorm.DB 字段，每个方法现取 pkg/storage 的全局连接。
package repository
