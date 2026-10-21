/**
 * SAAS 后台管理接口封装（docs/API.md §3 / §4 / §7 / §8 / §10.2 / §14 / §15）
 * http 拦截器已剥掉 { code, message, data } 外壳，这里直接返回业务 data。
 */
import http from './http'
import type {
  Channel,
  ChannelBasic,
  ChannelPayload,
  ChannelType,
  ConversationStats,
  Customer,
  CustomerDetailResult,
  Order,
  Paged,
  Product,
  Role,
} from '@/shared/types'

/* ------------------------------------------------------------------ *
 * 3. 渠道
 * ------------------------------------------------------------------ */

/** 渠道列表（agent 角色只返回自己可接待的渠道） */
export function listChannels(): Promise<Channel[]> {
  return http.get<Channel[]>('/channels')
}

/** 新增渠道（admin） */
export function createChannel(payload: ChannelPayload): Promise<Channel> {
  return http.post<Channel>('/channels', payload)
}

/** 修改渠道（admin），字段可选 */
export function updateChannel(id: number, payload: Partial<ChannelPayload>): Promise<Channel> {
  return http.put<Channel>(`/channels/${id}`, payload)
}

/** 删除渠道（admin） */
export function deleteChannel(id: number): Promise<boolean> {
  return http.del<boolean>(`/channels/${id}`)
}

/** 重置渠道标识 app_id（生成新的随机值，旧链接/二维码失效） */
export function resetChannelAppId(id: number): Promise<{ app_id: string; access_url: string }> {
  return http.post<{ app_id: string; access_url: string }>(`/channels/${id}/reset-app-id`)
}

/* ------------------------------------------------------------------ *
 * 4. 子账号（客服）管理
 * ------------------------------------------------------------------ */

/** 子账号在线状态 */
export type AgentStatus = 'online' | 'busy' | 'offline'

/** 子账号列表项 */
export interface Agent {
  id: number
  account: string
  name: string
  role: Role
  avatar: string
  online: boolean
  max_concurrent: number
  status: AgentStatus
  /** 该子账号可接待的渠道 */
  channels: ChannelBasic[]
  /** 当前接待中的会话数 */
  active_count: number
  /** 是否启用：API.md §4 未列出该字段，前端通过 PUT /agents/:id 提交，详见下方说明 */
  enabled?: boolean
}

/** 子账号新增/编辑请求体；password 留空表示不修改 */
export interface AgentPayload {
  account?: string
  password?: string
  name?: string
  role?: Role
  max_concurrent?: number
  channel_ids?: number[]
  /**
   * 启用/禁用子账号。
   * 注意：API.md §4 的 POST/PUT 请求体未列出 enabled 字段，
   * 前端按约定提交（需后端一并支持），否则该字段会被忽略。
   */
  enabled?: boolean
}

/** 上下线响应 */
export interface AgentOnlineResult {
  online: boolean
  status: AgentStatus
}

/** 子账号列表（admin） */
export function listAgents(): Promise<Agent[]> {
  return http.get<Agent[]>('/agents')
}

/** 新增子账号（admin） */
export function createAgent(payload: AgentPayload): Promise<Agent> {
  return http.post<Agent>('/agents', payload)
}

/** 修改子账号（admin）；password 留空表示不修改，可传 channel_ids 覆盖接待渠道 */
export function updateAgent(id: number, payload: AgentPayload): Promise<Agent> {
  return http.put<Agent>(`/agents/${id}`, payload)
}

/** 删除子账号（admin） */
export function deleteAgent(id: number): Promise<boolean> {
  return http.del<boolean>(`/agents/${id}`)
}

/** 上线/下线（自己或 admin） */
export function setAgentOnline(id: number, online: boolean): Promise<AgentOnlineResult> {
  return http.post<AgentOnlineResult>(`/agents/${id}/online`, { online })
}

/* ------------------------------------------------------------------ *
 * 5. 会话统计（概览页用）
 * ------------------------------------------------------------------ */

/** 会话统计 */
export function getConversationStats(): Promise<ConversationStats> {
  return http.get<ConversationStats>('/conversations/stats')
}

/* ------------------------------------------------------------------ *
 * 7. 客户（用户）
 * ------------------------------------------------------------------ */

/** 客户列表查询参数 */
export interface UserQuery {
  keyword?: string
  channel_id?: number
  page?: number
  page_size?: number
}

/** 客户列表 */
export function listUsers(params?: UserQuery): Promise<Paged<Customer>> {
  return http.get<Paged<Customer>>('/users', { params })
}

/** 客户详情（含历史订单与历史会话） */
export function getUserDetail(id: number): Promise<CustomerDetailResult> {
  return http.get<CustomerDetailResult>(`/users/${id}`)
}

/* ------------------------------------------------------------------ *
 * 8. 订单 & 商品
 * ------------------------------------------------------------------ */

/** 订单列表查询参数 */
export interface OrderQuery {
  user_id?: number
  keyword?: string
  page?: number
  page_size?: number
}

/** 订单列表 */
export function listOrders(params?: OrderQuery): Promise<Paged<Order>> {
  return http.get<Paged<Order>>('/orders', { params })
}

/** 商品列表查询参数 */
export interface ProductQuery {
  keyword?: string
  page?: number
  page_size?: number
}

/** 商品列表（这些商品用于客服在工作台发送商品卡片） */
export function listProducts(params?: ProductQuery): Promise<Paged<Product>> {
  return http.get<Paged<Product>>('/products', { params })
}

/* ------------------------------------------------------------------ *
 * 10.2 设备注册（APP / PC 桌面端推送）
 * ------------------------------------------------------------------ */

/** 设备类型 */
export type DeviceKind = 'app' | 'desktop' | 'web'

/** 已注册设备 */
export interface Device {
  id: number
  device: DeviceKind
  platform: string
  name: string
  /** 厂商推送 Token；为空表示该设备仅走 WebSocket（如 PC 桌面端） */
  push_token?: string
  created_at: string
}

/** 设备注册请求体 */
export interface DevicePayload {
  device: DeviceKind
  platform: string
  push_token: string
  name: string
}

/** 当前账号已注册设备列表 */
export function listDevices(): Promise<Device[]> {
  return http.get<Device[]>('/devices')
}

/** 注册设备（一般由 APP / PC 桌面端调用） */
export function registerDevice(payload: DevicePayload): Promise<Device> {
  return http.post<Device>('/devices', payload)
}

/** 注销设备 */
export function deleteDevice(id: number): Promise<boolean> {
  return http.del<boolean>(`/devices/${id}`)
}

/** 触发一条测试推送（推给当前登录账号的已注册设备），用于验证厂商通道 */
export function testDevicePush(payload?: { title?: string; body?: string }): Promise<boolean> {
  return http.post<boolean>('/devices/test-push', payload)
}

/* ------------------------------------------------------------------ *
 * 系统配置（默认图片等）
 * ------------------------------------------------------------------ */

/** 配置项 */
export interface SettingItem {
  key: string
  value: string
  remark: string
}

/** GET /api/settings —— 读取全部配置项 */
export function listSettings(): Promise<SettingItem[]> {
  return http.get<SettingItem[]>('/settings')
}

/** PUT /api/settings —— 保存单个配置项 */
export function saveSetting(key: string, value: string): Promise<boolean> {
  return http.put<boolean>('/settings', { key, value })
}

/* ------------------------------------------------------------------ *
 * 对象存储（S3 兼容）
 *
 * 存储方式：settings 表里 key = "storage.object"，value 是整份配置的
 * JSON 字符串；后端返回时会把 secret_key 脱敏成 "******"。
 * ------------------------------------------------------------------ */

/** 配置 key */
export const STORAGE_OBJECT_KEY = 'storage.object'

/** secret_key 脱敏占位符：提交该值表示「保留后端已有密钥」 */
export const STORAGE_SECRET_MASK = '******'

/** 对象存储配置（字段与后端 JSON 契约严格一致） */
export interface StorageObjectConfig {
  /** 是否启用对象存储（关闭时上传走服务器本地存储） */
  enabled: boolean
  /** 服务商标识，固定 ctyun（内部值，界面上统一表述为 S3 兼容协议） */
  provider: string
  /** S3 端点，不带 bucket */
  endpoint: string
  /** 区域，留空使用默认 */
  region: string
  bucket: string
  access_key: string
  /** 密钥；后端返回时已脱敏为 ****** */
  secret_key: string
  /** 文件对外访问域名（CDN 或 bucket 域名） */
  public_base: string
  /** 对象 key 路径前缀，默认 uploads/ */
  path_prefix: string
}

/** 配置缺省值：「读取失败 / 字段缺失」时的兜底，保证页面不白屏 */
export const STORAGE_OBJECT_DEFAULTS: StorageObjectConfig = {
  enabled: false,
  provider: 'ctyun',
  endpoint: '',
  region: '',
  bucket: 'kefu',
  access_key: '',
  secret_key: '',
  public_base: '',
  path_prefix: 'uploads/',
}

/** 规范化路径前缀：去空白、补尾部斜杠，空值回落 uploads/ */
export function normalizePathPrefix(value: string): string {
  const v = (value || '').trim().replace(/^\/+/, '')
  if (!v) return STORAGE_OBJECT_DEFAULTS.path_prefix
  return v.endsWith('/') ? v : `${v}/`
}

function str(value: unknown, fallback: string): string {
  return typeof value === 'string' ? value : fallback
}

/**
 * 解析 settings 里的 storage.object（JSON 字符串）。
 * 解析失败、非对象、字段缺失一律用默认值兜底，绝不抛错。
 */
export function parseStorageObject(raw: string | null | undefined): StorageObjectConfig {
  const cfg: StorageObjectConfig = { ...STORAGE_OBJECT_DEFAULTS }
  if (!raw) return cfg

  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return cfg
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return cfg

  const src = parsed as Record<string, unknown>
  cfg.enabled = typeof src.enabled === 'boolean' ? src.enabled : cfg.enabled
  cfg.provider = str(src.provider, cfg.provider) || cfg.provider
  cfg.endpoint = str(src.endpoint, cfg.endpoint)
  cfg.region = str(src.region, cfg.region)
  cfg.bucket = str(src.bucket, cfg.bucket)
  cfg.access_key = str(src.access_key, cfg.access_key)
  cfg.secret_key = str(src.secret_key, cfg.secret_key)
  cfg.public_base = str(src.public_base, cfg.public_base)
  cfg.path_prefix = normalizePathPrefix(str(src.path_prefix, cfg.path_prefix))
  return cfg
}

/** 序列化 storage.object：字段顺序与后端契约一致 */
export function serializeStorageObject(cfg: StorageObjectConfig): string {
  const payload: StorageObjectConfig = {
    enabled: !!cfg.enabled,
    provider: cfg.provider || 'ctyun',
    endpoint: (cfg.endpoint || '').trim(),
    region: (cfg.region || '').trim(),
    bucket: (cfg.bucket || '').trim(),
    access_key: (cfg.access_key || '').trim(),
    secret_key: (cfg.secret_key || '').trim(),
    public_base: (cfg.public_base || '').trim(),
    path_prefix: normalizePathPrefix(cfg.path_prefix),
  }
  return JSON.stringify(payload)
}

/** POST /api/settings/storage/test 返回值：body 为配置对象本体 */
export interface StorageTestResult {
  ok: boolean
  message: string
}

/** POST /api/settings/storage/test —— 用当前表单值实测一次连通性 */
export function testStorageObject(cfg: StorageObjectConfig): Promise<StorageTestResult> {
  return http.post<StorageTestResult>('/settings/storage/test', cfg)
}

/* ------------------------------------------------------------------ *
 * AI 应答（OpenAI 兼容接口）+ RAG 检索 —— 设置项
 *
 * 存储方式：settings 表逐项存储（一个 key 一个值），因此这里用「key → 值」的
 * 映射还原成一份表单，保存时逐项 PUT /api/settings。
 * ai.api_key 由后端脱敏为 ******，原样回写表示沿用已存密钥。
 * 检索语料 = 渠道的自动回复规则（关键词 + 回复内容），无独立知识库。
 * ------------------------------------------------------------------ */

/** AI 设置项 key（与后端 pkg/setting 的常量逐字一致） */
export const AI_SETTING_KEYS = {
  enabled: 'ai.enabled',
  baseUrl: 'ai.base_url',
  apiKey: 'ai.api_key',
  model: 'ai.model',
  systemPrompt: 'ai.system_prompt',
  timeoutSeconds: 'ai.timeout_seconds',
  topK: 'rag.top_k',
  minScore: 'rag.min_score',
} as const

/** ai.api_key 的脱敏占位符：提交该值表示「保留后端已存密钥」 */
export const AI_API_KEY_MASK = '******'

/** AI 应答配置（表单模型，字段与后端设置项一一对应） */
export interface AISettingForm {
  /** 是否启用 AI 应答（默认关闭：关闭时机器人只走规则 + 兜底话术） */
  enabled: boolean
  /** OpenAI 兼容接口根地址，如 https://api.deepseek.com/v1 */
  baseUrl: string
  /** 接口密钥；后端返回时已脱敏为 ****** */
  apiKey: string
  /** 模型名，如 deepseek-chat */
  model: string
  /** 系统提示词 */
  systemPrompt: string
  /** 请求超时（秒） */
  timeoutSeconds: number
  /** 检索返回条数 */
  topK: number
  /** 最低相关度 0~1 */
  minScore: number
}

/** AI 默认系统提示词（与后端 setting.DefaultAISystemPrompt 同义，仅用于首屏回显兜底） */
export const AI_DEFAULT_SYSTEM_PROMPT =
  '你是企业在线客服助手。只能依据下面提供的「参考资料」回答客户问题；' +
  '参考资料没有覆盖的内容不要猜测或编造，直接回复：这个问题我需要转接人工客服为您确认。' +
  '回答使用简体中文，语气礼貌、简洁，不要输出标题、编号列表或 Markdown 标记。'

/** 表单缺省值：读取失败 / 字段缺失时兜底，保证页面不白屏 */
export const AI_SETTING_DEFAULTS: AISettingForm = {
  enabled: false,
  baseUrl: '',
  apiKey: '',
  model: '',
  systemPrompt: AI_DEFAULT_SYSTEM_PROMPT,
  timeoutSeconds: 20,
  topK: 3,
  minScore: 0.15,
}

/** 从 GET /api/settings 的原始结果里解析出 AI 表单（缺项一律落默认值，绝不抛错） */
export function parseAISetting(values: Record<string, string>): AISettingForm {
  const num = (raw: string | undefined, def: number): number => {
    const v = Number((raw ?? '').trim())
    return Number.isFinite(v) && v > 0 ? v : def
  }
  const score = Number((values[AI_SETTING_KEYS.minScore] ?? '').trim())
  return {
    enabled: (values[AI_SETTING_KEYS.enabled] ?? '').trim().toLowerCase() === 'true',
    baseUrl: (values[AI_SETTING_KEYS.baseUrl] ?? '').trim(),
    apiKey: (values[AI_SETTING_KEYS.apiKey] ?? '').trim(),
    model: (values[AI_SETTING_KEYS.model] ?? '').trim(),
    systemPrompt: (values[AI_SETTING_KEYS.systemPrompt] ?? '').trim() || AI_DEFAULT_SYSTEM_PROMPT,
    timeoutSeconds: num(values[AI_SETTING_KEYS.timeoutSeconds], AI_SETTING_DEFAULTS.timeoutSeconds),
    topK: num(values[AI_SETTING_KEYS.topK], AI_SETTING_DEFAULTS.topK),
    minScore: Number.isFinite(score) && score >= 0 && score <= 1 ? score : AI_SETTING_DEFAULTS.minScore,
  }
}

/** 表单 → 逐项 PUT /api/settings 的 (key, value) 列表 */
export function serializeAISetting(form: AISettingForm): { key: string; value: string }[] {
  return [
    { key: AI_SETTING_KEYS.enabled, value: form.enabled ? 'true' : 'false' },
    { key: AI_SETTING_KEYS.baseUrl, value: (form.baseUrl || '').trim() },
    { key: AI_SETTING_KEYS.apiKey, value: (form.apiKey || '').trim() },
    { key: AI_SETTING_KEYS.model, value: (form.model || '').trim() },
    { key: AI_SETTING_KEYS.systemPrompt, value: (form.systemPrompt || '').trim() },
    { key: AI_SETTING_KEYS.timeoutSeconds, value: String(form.timeoutSeconds || AI_SETTING_DEFAULTS.timeoutSeconds) },
    { key: AI_SETTING_KEYS.topK, value: String(form.topK || AI_SETTING_DEFAULTS.topK) },
    { key: AI_SETTING_KEYS.minScore, value: String(form.minScore) },
  ]
}

/** 「测试连接」提交体：api_key 传 ****** 或留空表示沿用已保存的密钥 */
export interface AITestPayload {
  base_url: string
  api_key: string
  model: string
  timeout_seconds: number
}

/** POST /api/settings/ai/test —— 用当前表单值实测一次「地址 + 鉴权 + 模型名」 */
export function testAISetting(payload: AITestPayload): Promise<StorageTestResult> {
  return http.post<StorageTestResult>('/settings/ai/test', payload)
}

/* ------------------------------------------------------------------ *
 * 渠道类型库（可自定义，不限制枚举）—— §3 扩展
 * ------------------------------------------------------------------ */

/** 渠道类型（租户可自定义任意 code，不限制枚举） */
export interface ChannelTypeItem {
  id: number
  /** 英文标识，如 wechat / xiaohongshu；自定义类型亦可传中文 */
  code: string
  name: string
  color: string
  /** emoji 或图标名 */
  icon: string
  sort: number
  built_in: boolean
  /** 被渠道引用的次数，> 0 时不允许删除 */
  used_count: number
  created_at: string
}

export interface ChannelTypePayload {
  code: string
  name: string
  color: string
  icon: string
  sort: number
}

/** GET /api/channel-types */
export function listChannelTypes(): Promise<ChannelTypeItem[]> {
  return http.get<ChannelTypeItem[]>('/channel-types')
}

/** POST /api/channel-types */
export function createChannelType(payload: ChannelTypePayload): Promise<ChannelTypeItem> {
  return http.post<ChannelTypeItem>('/channel-types', payload)
}

/** PUT /api/channel-types/:id */
export function updateChannelType(
  id: number,
  payload: ChannelTypePayload,
): Promise<boolean> {
  return http.put<boolean>(`/channel-types/${id}`, payload)
}

/** DELETE /api/channel-types/:id（被渠道引用时后端会报错） */
export function deleteChannelType(id: number): Promise<boolean> {
  return http.del<boolean>(`/channel-types/${id}`)
}

/* ------------------------------------------------------------------ *
 * 14. 自动回复规则
 * ------------------------------------------------------------------ */

/** 匹配方式：contain 包含 / equal 完全匹配 / prefix 前缀 */
export type MatchType = 'contain' | 'equal' | 'prefix'

export const MATCH_TYPE_OPTIONS: { value: MatchType; label: string }[] = [
  { value: 'contain', label: '包含' },
  { value: 'equal', label: '完全匹配' },
  { value: 'prefix', label: '前缀' },
]

export function matchTypeLabel(type: string): string {
  const hit = MATCH_TYPE_OPTIONS.find((item) => item.value === type)
  return hit ? hit.label : type
}

/** 自动回复规则 */
export interface AutoReply {
  id: number
  /** 归属：0 = 公共规则（管理员建的）；> 0 = 创建该规则的客服账号 ID */
  user_id: number
  /** 「公共」或创建人昵称 */
  owner: string
  /** user_id = 0：公共规则（客服只读） */
  shared: boolean
  /** 当前登录账号能否改 / 删（客服对公共规则与别人的规则为 false） */
  editable: boolean
  name: string
  /** 旧单渠道字段：0 = 全渠道（历史数据） */
  channel_id: number
  /** 旧单渠道名（渠道名 / 全渠道） */
  channel_name: string
  /** 多选渠道（已解析成数组，空 = 全渠道） */
  channel_ids: number[]
  /** 多选渠道名，与 channel_ids 一一对应（渠道被删时后端退化成「渠道 #id」） */
  channel_names: string[]
  match_type: string
  /** 逗号分隔的关键词 */
  keywords: string
  reply: string
  reply_type: string
  transfer_to_human: boolean
  enabled: boolean
  priority: number
  hit_count: number
  created_at: string
}

export interface AutoReplyPayload {
  name: string
  /** 多选渠道：空列表 = 全渠道（仅管理员）；客服至少选 1 个自己已授权的渠道 */
  channel_ids: number[]
  /** 兼容旧客户端的单渠道字段（可选，0 = 全渠道）；新前端只传 channel_ids */
  channel_id?: number
  match_type: string
  keywords: string
  reply: string
  reply_type: string
  transfer_to_human: boolean
  enabled: boolean
  priority: number
}

/** GET /api/auto-replies —— 全租户可见（含其它客服建的规则），每条带 editable */
export function listAutoReplies(): Promise<AutoReply[]> {
  return http.get<AutoReply[]>('/auto-replies')
}

/** POST /api/auto-replies —— 客服建的归属自己；管理员建的为公共规则（user_id = 0） */
export function createAutoReply(payload: AutoReplyPayload): Promise<AutoReply> {
  return http.post<AutoReply>('/auto-replies', payload)
}

/** PUT /api/auto-replies/:id —— 客服只能改自己创建的，公共规则返回「公共规则仅管理员可修改」 */
export function updateAutoReply(id: number, payload: AutoReplyPayload): Promise<boolean> {
  return http.put<boolean>(`/auto-replies/${id}`, payload)
}

/** DELETE /api/auto-replies/:id —— 权限口径同修改 */
export function deleteAutoReply(id: number): Promise<boolean> {
  return http.del<boolean>(`/auto-replies/${id}`)
}

/* ------------------------------------------------------------------ *
 * 15. 数据统计
 * ------------------------------------------------------------------ */

/** 趋势单日数据点 */
export interface TrendPoint {
  date: string
  total: number
  handled: number
  closed: number
  pending: number
  messages: number
  customer_msgs: number
  agent_msgs: number
}

/** 客服工作量 */
export interface AgentStat {
  agent_id: number
  name: string
  avatar: string
  /** admin | agent：管理员若也参与接待会计入工作量 */
  role?: string
  handled: number
  closed: number
  /** 最近 N 天的消息数 */
  messages: number
  active_now: number
  avg_first_reply: number
}

/** 渠道分布 */
export interface ChannelStat {
  channel_id: number
  name: string
  type: string
  total: number
  pending: number
  open: number
  closed: number
}

export interface StatsSummary {
  total_conversations: number
  handled_rate: number
  avg_messages: number
  customers: number
  new_customers: number
  orders: number
  online_agents: number
}

export interface StatsOverviewResult {
  trend: TrendPoint[]
  agents: AgentStat[]
  channels: ChannelStat[]
  summary: StatsSummary
}

/** GET /api/stats/overview?days=7（days 支持 1-90，默认 7） */
export function getStatsOverview(days: number): Promise<StatsOverviewResult> {
  return http.get<StatsOverviewResult>('/stats/overview', { params: { days } })
}

/* ------------------------------------------------------------------ *
 * 渠道类型工具：后台多处需要将 type 落成「图标 + 文字」
 * ------------------------------------------------------------------ */

/**
 * 渠道类型展示元数据：后台多处需要把 type 落成「图标 + 文字」。
 * 颜色取自 docs/API.md §1 的侧边栏图标配色。
 */
export interface ChannelTypeMeta {
  label: string
  /** 图标（emoji 占位，见 PROJECT.md「渠道图标可用 emoji」） */
  icon: string
  color: string
}

export const CHANNEL_TYPE_META: Record<ChannelType, ChannelTypeMeta> = {
  wechat: { label: '微信客服', icon: '💬', color: '#07C160' },
  wecom: { label: '企业微信', icon: '🏢', color: '#2E6BE6' },
  web: { label: '网页在线客服', icon: '🌐', color: '#1890FF' },
  app: { label: 'APP 内客服', icon: '📱', color: '#FF7A1A' },
  weibo: { label: '微博', icon: '📣', color: '#E6162D' },
  douyin: { label: '抖音', icon: '🎵', color: '#191919' },
  robot: { label: '机器人助手', icon: '🤖', color: '#7B61FF' },
}

const FALLBACK_META: ChannelTypeMeta = { label: '未知渠道', icon: '❓', color: '#7F7F7F' }

/** 取渠道类型展示元数据，未知类型走兜底 */
export function channelTypeMeta(type: string): ChannelTypeMeta {
  return CHANNEL_TYPE_META[type as ChannelType] || FALLBACK_META
}

export const CHANNEL_TYPE_OPTIONS: { value: ChannelType; label: string }[] = [
  { value: 'wechat', label: '微信客服' },
  { value: 'wecom', label: '企业微信' },
  { value: 'web', label: '网页在线客服' },
  { value: 'app', label: 'APP 内客服' },
  { value: 'weibo', label: '微博' },
  { value: 'douyin', label: '抖音' },
  { value: 'robot', label: '机器人助手' },
]

/** 渠道类型中文名，未知类型原样返回 */
export function channelTypeLabel(type: string): string {
  const hit = CHANNEL_TYPE_OPTIONS.find((item) => item.value === type)
  return hit ? hit.label : type
}

/* ------------------------------------------------------------------ *
 * 渠道二维码 & 接待账号（§3 扩展）
 * ------------------------------------------------------------------ */

/** GET /api/channels/:id/qr 返回值 */
export interface ChannelQr {
  channel_id: number
  channel_name: string
  channel_type: string
  /** 二维码内容（完整接入地址） */
  url: string
  /** 接入相对路径，如 /c?channel=web */
  path?: string
  /** 二维码图片，data URI（data:image/png;base64,...）或裸 base64 */
  base64: string
  size: number
  /** 下载文件名 */
  file_name: string
}

/** 渠道二维码请求参数 */
export interface ChannelQrQuery {
  /** 图片边长，默认 320 */
  size?: number
  /** 二维码内容，默认后端按 origin + access_path 生成 */
  base?: string
}

/** GET /api/channels/:id/qr —— 渠道接入二维码（base64 图片 + 完整地址） */
export function getChannelQr(id: number, query?: ChannelQrQuery): Promise<ChannelQr> {
  return http.get<ChannelQr>(`/channels/${id}/qr`, { params: query })
}

/**
 * GET /api/channels/:id/qr.png 的可下载地址（直接返回 PNG，带 Content-Disposition）。
 * 同域直连，不经过 axios，避免响应拦截器把图片当成 { code, message, data } 处理。
 * 该接口属于 JWT 分组，而 <a href> 点击不会带 Authorization 头，
 * 因此带上 ?token= （后端 JWTAuth 中间件兼容 query 传 token）。
 */
export function channelQrPngUrl(id: number, size?: number, token?: string): string {
  const params: string[] = []
  if (size) params.push(`size=${encodeURIComponent(String(size))}`)
  if (token) params.push(`token=${encodeURIComponent(token)}`)
  const query = params.length ? `?${params.join('&')}` : ''
  return `/api/channels/${encodeURIComponent(String(id))}/qr.png${query}`
}

/** 渠道可分配的接待客服（子账号） */
export interface ChannelAgent {
  id: number
  name: string
  account: string
  avatar: string
  online: boolean
  enabled: boolean
  /** 是否已分配给该渠道 */
  selected: boolean
}

/** GET /api/channels/:id/agents —— 该渠道的接待账号及选中状态 */
export function listChannelAgents(id: number): Promise<ChannelAgent[]> {
  return http.get<ChannelAgent[]>(`/channels/${id}/agents`)
}

/** PUT /api/channels/:id/agents —— 全量覆盖该渠道的接待账号 */
export function updateChannelAgents(id: number, userIds: number[]): Promise<ChannelAgent[]> {
  return http.put<ChannelAgent[]>(`/channels/${id}/agents`, { user_ids: userIds })
}
