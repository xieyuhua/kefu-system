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

/** POST /api/conversations/:id/messages */
export function sendMessage(
  conversationId: number,
  payload: SendMessagePayload,
): Promise<Message> {
  return http.post<Message>(`/conversations/${conversationId}/messages`, payload)
}
