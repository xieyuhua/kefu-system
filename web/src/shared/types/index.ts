/**
 * 与 docs/API.md 字段一一对应的类型定义。
 * 所有后端响应均包在 { code, message, data } 中，data 部分在此描述。
 */

/** 渠道类型 */
export type ChannelType = 'wechat' | 'wecom' | 'web' | 'app' | 'weibo' | 'douyin' | 'robot'

/** 会话状态 */
export type ConversationStatus = 'pending' | 'open' | 'closed'

/** 消息类型 */
export type MessageType = 'text' | 'image' | 'video' | 'file' | 'order' | 'product' | 'system'

/** 消息方向 */
export type MessageDirection = 'in' | 'out' | 'system'

/** 角色 */
export type Role = 'admin' | 'agent'

/** 统一响应外壳 */
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

/** 租户信息 */
export interface Tenant {
  id: number
  name: string
  logo: string
  /** 租户配置的默认图片（客户/商品/订单无图时兜底） */
  default_image?: string
}

/** 当前登录用户 */
export interface User {
  id: number
  tenant_id: number
  role: Role
  name: string
  account: string
  avatar: string
  online: boolean
  channels: number[]
  tenant?: Tenant
}

/** 登录响应 */
export interface LoginResult {
  token: string
  user: User
}

/** 登录请求 */
export interface LoginPayload {
  account: string
  password: string
}

/** 渠道 */
export interface Channel {
  id: number
  type: ChannelType
  name: string
  app_id: string
  enabled: boolean
  robot_enabled: boolean
  /** 用户端展示控制（后台渠道配置） */
  hide_header: boolean
  hide_debug: boolean
  hide_more_menu: boolean
  created_at: string
  agent_count: number
  pending_count: number
  /** 用户端接入相对路径（来自后端，方便调试） */
  access_path?: string
  /** 完整接入地址（基于后端 public_base 拼接），直接对外复制/分享 */
  access_url?: string
}

export interface ChannelPayload {
  /**
   * 渠道类型：租户可自定义任意类型（见 GET /api/channel-types），
   * 因此这里放宽为 string，不再限制在 ChannelType 枚举内。
   */
  type: string
  name: string
  app_id: string
  enabled: boolean
  robot_enabled: boolean
  hide_header?: boolean
  hide_debug?: boolean
  hide_more_menu?: boolean
}

/** 会话中的客户简要信息 */
export interface ConversationUser {
  id: number
  external_id: string
  nickname: string
  avatar: string
  phone: string
  order_count?: number
  /** 是否在线（最近 2 分钟内有活跃） */
  online?: boolean
  /** 最后一次活跃时间 */
  last_seen_at?: string
  /** 客户最近一次访问 IP */
  ip?: string
  /** IP 归属城市，如「广东 深圳」 */
  city?: string
  tags?: TagRef[]
  remark?: string
}

/** 客户标签 */
export interface TagRef {
  id: number
  name: string
  color: string
}

/** 会话中的客服简要信息 */
export interface ConversationAgent {
  id: number
  name: string
}

/** 会话 */
export interface Conversation {
  id: number
  channel_id: number
  channel: ChannelBasic
  user: ConversationUser
  status: ConversationStatus
  agent: ConversationAgent | null
  unread: number
  last_message: string
  last_message_at: string
  created_at: string
  order_count: number
  closed_at?: string
  /** 是否已归档（归档后不再展示在工作台列表） */
  archived?: boolean
  /** 归档时间 */
  archived_at?: string
}

/** 嵌套在会话/用户中的渠道精简对象 */
export interface ChannelBasic {
  id: number
  type: ChannelType
  name: string
  /** 渠道级用户端展示控制（后台渠道配置，优先级高于 URL 参数） */
  hide_header?: boolean
  hide_debug?: boolean
  hide_more_menu?: boolean
}

/** 会话列表查询参数 */
export interface ConversationQuery {
  channel_id?: number
  status?: ConversationStatus | 'all'
  scope?: 'mine' | 'all'
  keyword?: string
  /** 是否包含已归档会话 */
  include_archived?: boolean | number
  page?: number
  page_size?: number
}

/** 会话列表响应 */
export interface ConversationListResult {
  list: Conversation[]
  total: number
  page: number
  page_size: number
}

/** 会话统计 */
export interface ConversationStats {
  pending: number
  open: number
  closed: number
  unread: number
}

/** 消息发送者 */
export interface MessageSender {
  id: number
  name: string
  avatar: string
  role: string
}

/** 消息 */
export interface Message {
  id: number
  conversation_id: number
  type: MessageType
  direction: MessageDirection
  content: string
  sender: MessageSender | null
  created_at: string
  /**
   * 消息创建时间的 Unix 毫秒时间戳（后端新增字段，可选）。
   * created_at 只有秒级精度的字符串，秒级计时（未回复计时）优先用它；
   * 缺失时回退解析 created_at，因此旧后端/旧缓存数据也能正常工作。
   */
  created_ts?: number
  /** 乐观更新临时标记 */
  pending?: boolean
  failed?: boolean
  /**
   * 前端本地生成的消息（乐观气泡、转人工提示等），服务端无对应 id。
   * 用户端用它排除在 since_id 游标之外，避免把本地临时 id 当作轮询起点。
   */
  local?: boolean
}

/** 发送消息请求 */
export interface SendMessagePayload {
  type: MessageType
  content: string
}

/** 订单卡片 JSON（message.content 解析结果） */
export interface OrderCardPayload {
  order_no: string
  title: string
  status: string
  amount: number
  quantity: number
  created_at: string
  image: string
  sku: string
}

/** 商品卡片 JSON（message.content 解析结果） */
export interface ProductCardPayload {
  id: string
  title: string
  price: number
  image: string
  stock: number
  sku: string
  url: string
}

/** 历史订单 / 订单列表项 */
export interface Order {
  id: number
  order_no: string
  title: string
  status: string
  amount: number
  quantity: number
  image: string
  sku: string
  created_at: string
  paid_at?: string
}

/** 商品 */
export interface Product {
  id: string
  title: string
  price: number
  image: string
  stock: number
  sku: string
  url: string
}

/** 客户列表项 */
export interface Customer {
  id: number
  external_id: string
  nickname: string
  avatar: string
  phone: string
  channel: ChannelBasic
  order_count: number
  conversation_count: number
  last_seen_at: string
  created_at: string
  /** 客户标签（后端返回 {id,name,color} 对象数组，跨客服可见） */
  tags?: TagRef[]
  /** 客服备注（跨客服可见） */
  remark?: string
  /** 客户最近一次访问 IP */
  ip?: string
  /** IP 归属城市 */
  city?: string
}

/** 客户详情（历史档案） */
export interface CustomerDetailResult {
  user: Customer
  orders: Order[]
  conversations: CustomerConversation[]
}

/** 历史会话 */
export interface CustomerConversation {
  id: number
  channel: ChannelBasic
  status: ConversationStatus
  agent: ConversationAgent | null
  message_count: number
  summary: string
  created_at: string
  closed_at?: string
}

/** 分页列表通用结构 */
export interface Paged<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

/** 发起模拟咨询请求 */
export interface DemoIncomingPayload {
  channel_id: number
  /** 客户唯一 ID：填写则复用同一客户/会话，可反复发消息；留空则新建客户 */
  uid?: string
  nickname: string
  content: string
  type: MessageType
}

/** WebSocket 下行事件 */
export type SocketEvent =
  | 'message.new'
  | 'conversation.new'
  | 'conversation.updated'
  | 'notification.new'

/** WebSocket 下行报文 */
export interface SocketFrame {
  event: SocketEvent
  data: unknown
}

/** 通知事件数据 */
export interface NotificationPayload {
  id: number
  title: string
  body: string
  conversation_id: number
}
