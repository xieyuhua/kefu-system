import http from './http'
import type {
  Conversation,
  ConversationListResult,
  ConversationQuery,
  ConversationStats,
} from '@/shared/types'

function toQuery(params?: Record<string, unknown>): string {
  if (!params) return ''
  const usp = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    usp.append(key, String(value))
  })
  const qs = usp.toString()
  return qs ? `?${qs}` : ''
}

/** GET /api/conversations */
export function listConversations(query: ConversationQuery = {}): Promise<ConversationListResult> {
  return http.get<ConversationListResult>('/conversations' + toQuery({ ...query }))
}

/** GET /api/conversations/:id */
export function getConversation(id: number): Promise<Conversation> {
  return http.get<Conversation>(`/conversations/${id}`)
}

/** POST /api/conversations/:id/claim 接入（领取）会话 */
export function claimConversation(id: number): Promise<Conversation> {
  return http.post<Conversation>(`/conversations/${id}/claim`)
}

/** POST /api/conversations/:id/transfer 转接 */
export function transferConversation(id: number, agentId: number): Promise<Conversation> {
  return http.post<Conversation>(`/conversations/${id}/transfer`, { agent_id: agentId })
}

/** POST /api/conversations/:id/close 结束会话 */
export function closeConversation(id: number): Promise<Conversation> {
  return http.post<Conversation>(`/conversations/${id}/close`)
}

/** POST /api/conversations/:id/read 标记已读 */
export function readConversation(id: number): Promise<boolean> {
  return http.post<boolean>(`/conversations/${id}/read`)
}

/** POST /api/conversations/:id/archive 归档（从工作台移除） */
export function archiveConversation(id: number): Promise<boolean> {
  return http.post<boolean>(`/conversations/${id}/archive`)
}

/** POST /api/conversations/:id/unarchive 取消归档 */
export function unarchiveConversation(id: number): Promise<boolean> {
  return http.post<boolean>(`/conversations/${id}/unarchive`)
}

/**
 * POST /api/conversations/archive-bulk —— 一键清空工作台列表。
 * 按当前筛选条件批量归档，返回实际处理的条数。
 */
export function archiveConversationsBulk(query: {
  channel_id?: number
  status?: string
  scope?: string
  keyword?: string
}): Promise<{ archived: number }> {
  const params: Record<string, string | number> = {}
  if (query.channel_id) params.channel_id = query.channel_id
  if (query.status) params.status = query.status
  if (query.scope) params.scope = query.scope
  if (query.keyword) params.keyword = query.keyword
  return http.post<{ archived: number }>('/conversations/archive-bulk', null, { params })
}

/** GET /api/conversations/stats */
export function getConversationStats(): Promise<ConversationStats> {
  return http.get<ConversationStats>('/conversations/stats')
}
