// Package service 三端（后台端 / 客服端 / 用户端）共用的业务编排层。
//
// 原先按域分散在 internal/core/<域> 下，现在合并成同一个包，靠类型名区分（按域取）：
//   - AccountService       账号、登录鉴权、个人资料、子账号、在线状态
//   - ChannelService       渠道、渠道类型库、二维码 / 接入地址、渠道 ↔ 接待账号
//   - CustomerService      客户、客户标签、订单、商品、历史档案、订单商品查询
//   - ConversationService  会话与消息、可见范围、归档 / 转接 / 关闭、实时推送
//   - AutomationService    关键词自动回复规则
//   - NotifyService        推送设备与站内提醒
//   - StatsService         概览统计口径
//   - UploadService        附件上传（含游客态兜底租户）
//
// 依赖方向：common/service → common/repository → common/model → platform → pkg。
// 本包不得 import 任何端（admin / agent / client），端只能单向依赖本包。
//
// 形态约定：业务编排都是 *XxxService 的方法（指针接收者），参数与返回值用 *model.X /
// []*model.X；跨域调用在构造时注入对方 Service（如 ConversationService.customer），
// 同包内不再有 import 关系。只有无状态的纯函数（authErr / hashPwd / messagePreview /
// matchKeywords / AccessPath / ChannelKey / AccessURL 等）保留为包级函数。
//
// 同名方法说明：Go 允许不同类型上的同名方法，因此合并后同名方法按「所属类型」区分语义
// （如 AccountService.FindUserByID 与 ConversationService.FindUserByID）；
// 只有确实会让同包读代码产生歧义的仓储方法才按域加前缀（见 common/repository 的
// UserListChannelsByTenant / UserCreateAgentChannel / ListCustomerConversations /
// FindChannelRefByID / FindUserRefByID）。
package service
