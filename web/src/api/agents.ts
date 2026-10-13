import http from './http'
import type { ChannelBasic } from '@/types'

export interface Agent {
  id: number
  account: string
  name: string
  role: 'admin' | 'agent'
  avatar: string
  online: boolean
  max_concurrent: number
  status: string
  channels: ChannelBasic[]
  active_count: number
}

/** GET /api/agents（admin） */
export function listAgents(): Promise<Agent[]> {
  return http.get<Agent[]>('/agents')
}
