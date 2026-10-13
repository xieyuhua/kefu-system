import http from './http'
import type {
  Conversation,
  ConversationListResult,
  ConversationQuery,
  ConversationStats,
} from '@/types'

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

/** GET /api/conversations/stats */
export function getConversationStats(): Promise<ConversationStats> {
  return http.get<ConversationStats>('/conversations/stats')
}
