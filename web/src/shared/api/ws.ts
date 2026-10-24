import http from './http'

/** 一条 WebSocket 长连接（客服端 / 访客端） */
export interface WsConnection {
  id: number
  kind: 'agent' | 'guest'
  tenant_id: number
  user_id: number
  conversation_id: number
  device: string
  remote_addr: string
  connected_at: number
  duration_seconds: number
}

export interface WsConnectionList {
  list: WsConnection[]
  total: number
  agents: number
  guests: number
}

/** 查看当前所有长连接；kind 可选 agent | guest */
export function listWsConnections(kind?: string): Promise<WsConnectionList> {
  const qs = kind ? `?kind=${encodeURIComponent(kind)}` : ''
  return http.get<WsConnectionList>(`/ws/connections${qs}`)
}

/** 断开指定连接；ids 为空时按 kind 批量断开 */
export function kickWsConnections(ids: number[], kind?: string): Promise<{ closed: number }> {
  return http.post<{ closed: number }>('/ws/connections/kick', { ids, kind: kind || '' })
}