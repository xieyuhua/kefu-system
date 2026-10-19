import http from './http'
import type { Customer, CustomerDetailResult, Paged } from '@/shared/types'

export interface CustomerQuery {
  keyword?: string
  channel_id?: number
  page?: number
  page_size?: number
}

/** GET /api/users */
export function listCustomers(query: CustomerQuery = {}): Promise<Paged<Customer>> {
  const usp = new URLSearchParams()
  if (query.keyword) usp.set('keyword', query.keyword)
  if (query.channel_id) usp.set('channel_id', String(query.channel_id))
  if (query.page) usp.set('page', String(query.page))
  if (query.page_size) usp.set('page_size', String(query.page_size))
  const qs = usp.toString()
  return http.get<Paged<Customer>>(`/users${qs ? `?${qs}` : ''}`)
}

/** GET /api/users/:id —— 用户详情 + 历史订单 + 历史会话 */
export function getCustomer(id: number): Promise<CustomerDetailResult> {
  return http.get<CustomerDetailResult>(`/users/${id}`)
}
