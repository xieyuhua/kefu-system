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

/** GET /api/agents（admin 看全部） */
export function listAgents(): Promise<Agent[]> {
  return http.get<Agent[]>('/agents')
}

/**
 * GET /api/agents?scope=transfer —— 转接用候选客服。
 * 客服账号可调用（仅返回在线且未停用的同事），再按会话所属渠道在前端过滤。
 */
export function listTransferAgents(): Promise<Agent[]> {
  return http.get<Agent[]>('/agents', { params: { scope: 'transfer' } })
}
