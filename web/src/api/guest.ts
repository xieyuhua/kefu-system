/**
 * 用户端（客户端）接口封装（docs/API.md §13 / §14）
 *
 * 用户端无需登录，渠道等对接参数一律通过 URL query 传递
 * （/c?channel=web&uid=...&nickname=...），便于运营直接拼链接投放。
 *
 * 说明：这里复用 web/src/api/http.ts 的 axios 实例（baseURL `/api`），
 * 用户端未登录时不会带 Authorization 头，后端 /api/guest/* 也不做鉴权。
 */
import http from './http'
import type { ChannelBasic, Message, OrderCardPayload, ProductCardPayload } from '@/types'

/** 用户端 URL 上承载的全部对接参数（全部来自 GET query） */
export interface GuestParams {
  /** 渠道标识：渠道类型（wechat/web/...）或渠道 ID，必填 */
  channel: string
  /** 业务侧 openid / user_id，不传由服务端生成游客 ID */
  uid?: string
  nickname?: string
  avatar?: string
  phone?: string
  /** 来源订单号 */
  order_no?: string
  /** 来源商品 ID */
  pid?: string
  /** 自定义场景标识（product / order / vip ...） */
  scene?: string
  /** 游客追踪 ID */
  visitor_id?: string
  /** 租户 ID（多租户演示用，缺省取第一个租户） */
  tenant?: string
  /** 签名（可选，防篡改） */
  sign?: string
}

/** GET /api/guest/init 响应 */
export interface GuestInitResult {
  channel: ChannelBasic
  customer: {
    id: number
    external_id: string
    nickname: string
    avatar: string
  }
  conversation: {
    id: number
    status: string
  }
  /** 机器人欢迎语，页面渲染为第一条气泡 */
  greeting: string
  order: OrderCardPayload | null
  product: ProductCardPayload | null
  /** 最近 20 条历史消息 */
  history: Message[]
}

/** POST /api/guest/message 响应 */
export interface GuestSendResult {
  /** 已入库的「我」的消息 */
  message: Message
  /** 命中的机器人回复，可能为 null */
  reply: Message | null
  /** 是否命中转人工 */
  transferred: boolean
}

/** 把参数对象整理成 query（去空值，保证 URL 干净） */
function toQuery(params: GuestParams): Record<string, string> {
  const query: Record<string, string> = {}
  const source = params as unknown as Record<string, string | undefined>
  for (const key of Object.keys(source)) {
    const value = source[key]
    if (value !== undefined && value !== null && String(value).trim() !== '') {
      query[key] = String(value)
    }
  }
  return query
}

/** 13.2 用户端初始化：渠道 / 客户 / 会话 / 欢迎语 / 上下文卡片 */
export function guestInit(params: GuestParams): Promise<GuestInitResult> {
  return http.get<GuestInitResult>('/guest/init', { params: toQuery(params) })
}

/**
 * 13.3 用户端发消息
 * POST /api/guest/message?channel=web&uid=u_10001  body { type, content }
 */
export function guestSend(
  params: Pick<GuestParams, 'channel' | 'uid' | 'tenant'>,
  body: { type: string; content: string },
): Promise<GuestSendResult> {
  return http.post<GuestSendResult>('/guest/message', body, { params: toQuery(params) })
}

/** 13.4 降级轮询：拉取 since_id 之后的消息（无 WebSocket 环境） */
export function guestMessages(
  params: Pick<GuestParams, 'channel' | 'uid' | 'tenant'>,
  sinceId: number,
): Promise<Message[]> {
  const query = toQuery(params)
  query.since_id = String(sinceId)
  return http.get<Message[]>('/guest/messages', { params: query })
}
