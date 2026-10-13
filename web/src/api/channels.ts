import http from './http'
import type { Channel, ChannelPayload } from '@/types'

/** GET /api/channels */
export function listChannels(): Promise<Channel[]> {
  return http.get<Channel[]>('/channels')
}

/** POST /api/channels（admin） */
export function createChannel(payload: ChannelPayload): Promise<Channel> {
  return http.post<Channel>('/channels', payload)
}

/** PUT /api/channels/:id（admin） */
export function updateChannel(id: number, payload: Partial<ChannelPayload>): Promise<Channel> {
  return http.put<Channel>(`/channels/${id}`, payload)
}

/** DELETE /api/channels/:id（admin） */
export function deleteChannel(id: number): Promise<boolean> {
  return http.del<boolean>(`/channels/${id}`)
}
