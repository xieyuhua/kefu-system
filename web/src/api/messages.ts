import http from './http'
import type { Message, SendMessagePayload } from '@/types'

/** GET /api/conversations/:id/messages */
export function listMessages(
  conversationId: number,
  params: { before_id?: number; limit?: number } = {},
): Promise<Message[]> {
  const usp = new URLSearchParams()
  if (params.before_id) usp.set('before_id', String(params.before_id))
  if (params.limit) usp.set('limit', String(params.limit))
  const qs = usp.toString()
  return http.get<Message[]>(`/conversations/${conversationId}/messages${qs ? `?${qs}` : ''}`)
}

/** GET /api/conversations/:id/messages/search 会话内历史消息搜索 */
export function searchMessages(
  conversationId: number,
  params: { keyword: string; limit?: number } = { keyword: '' },
): Promise<Message[]> {
  const usp = new URLSearchParams()
  usp.set('keyword', params.keyword)
  if (params.limit) usp.set('limit', String(params.limit))
  return http.get<Message[]>(`/conversations/${conversationId}/messages/search?${usp.toString()}`)
}

/** POST /api/conversations/:id/messages */
export function sendMessage(
  conversationId: number,
  payload: SendMessagePayload,
): Promise<Message> {
  return http.post<Message>(`/conversations/${conversationId}/messages`, payload)
}
