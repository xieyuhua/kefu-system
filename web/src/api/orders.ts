import http from './http'
import type { Order, Paged } from '@/types'

export interface OrderQuery {
  user_id?: number
  keyword?: string
  page?: number
  page_size?: number
}

/** GET /api/orders */
export function listOrders(query: OrderQuery = {}): Promise<Paged<Order>> {
  const usp = new URLSearchParams()
  if (query.user_id) usp.set('user_id', String(query.user_id))
  if (query.keyword) usp.set('keyword', query.keyword)
  if (query.page) usp.set('page', String(query.page))
  if (query.page_size) usp.set('page_size', String(query.page_size))
  const qs = usp.toString()
  return http.get<Paged<Order>>(`/orders${qs ? `?${qs}` : ''}`)
}
