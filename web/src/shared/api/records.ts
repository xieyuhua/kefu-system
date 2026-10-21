import http from './http'

/** 对话记录列表项（管理后台全局检索） */
export interface ConversationRecord {
  id: number
  customer_id: number
  customer_name: string
  customer_avatar: string
  channel_id: number
  channel_name: string
  channel_type: string
  agent_id: number
  agent_name: string
  status: 'open' | 'pending' | 'closed'
  archived: boolean
  message_count: number
  last_message: string
  last_message_at: string
  /** 未回复起点（Unix 毫秒）：最后一条消息是客户发的才 > 0，与工作台会话列表同一口径 */
  unreplied_ts?: number
  created_at: string
  first_message: string
  first_message_at: string
}

export interface RecordQuery {
  keyword?: string
  channel_id?: number
  status?: string
  agent_id?: number
  start?: string
  end?: string
  page?: number
  page_size?: number
}

export interface Paged<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

/** GET /api/conversation-records —— 全局查询对话记录 */
export function fetchConversationRecords(query: RecordQuery = {}): Promise<Paged<ConversationRecord>> {
  const params: Record<string, string | number> = {}
  if (query.keyword) params.keyword = query.keyword
  if (query.channel_id) params.channel_id = query.channel_id
  if (query.status) params.status = query.status
  if (query.agent_id) params.agent_id = query.agent_id
  if (query.start) params.start = query.start
  if (query.end) params.end = query.end
  params.page = query.page ?? 1
  params.page_size = query.page_size ?? 20
  return http.get<Paged<ConversationRecord>>('/conversation-records', { params })
}

/** 对话记录抽屉里的单条消息（与 GET /api/conversations/:id/messages 的 MessageVO 同构） */
export interface RecordMessage {
  id: number
  conversation_id?: number
  type?: string
  direction?: string
  content?: string
  sender?: { id?: number; name?: string; avatar?: string; role?: string }
  created_at?: string
  created_ts?: number
}

/**
 * 对话记录抽屉的一页消息。
 * - total：该会话的消息总数（DB 实际条数，与分页无关），用于显示「共 X 条」；
 * - has_more：本页最早一条之前还有更早的消息，为 true 时继续用 list[0].id 作为 before_id 拉取。
 */
export interface RecordMessagePage {
  list: RecordMessage[]
  total: number
  has_more: boolean
}

/** 抽屉单页条数：与后端缺省值一致（后端上限 200） */
export const RECORD_MESSAGE_PAGE_SIZE = 50

/**
 * GET /api/conversation-records/:id/messages?before_id=&limit=
 * 向前翻页取某会话的消息（id 升序返回）；before_id 缺省表示取最新一页。
 */
export function fetchRecordMessages(
  id: number,
  params: { before_id?: number; limit?: number } = {},
): Promise<RecordMessagePage> {
  const query: Record<string, string | number> = { limit: params.limit ?? RECORD_MESSAGE_PAGE_SIZE }
  if (params.before_id) query.before_id = params.before_id
  return http.get<RecordMessagePage>(`/conversation-records/${id}/messages`, { params: query })
}