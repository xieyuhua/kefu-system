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

/** GET /api/conversation-records/:id/messages —— 查看某个会话的完整消息 */
export function fetchRecordMessages(id: number, limit = 200): Promise<unknown[]> {
  return http.get<unknown[]>(`/conversation-records/${id}/messages`, { params: { limit } })
}