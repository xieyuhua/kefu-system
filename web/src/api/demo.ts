import http from './http'
import type { Conversation, DemoIncomingPayload, Paged, Product } from '@/types'

/** POST /api/demo/incoming 模拟用户发起新咨询 */
export function incoming(payload: DemoIncomingPayload): Promise<Conversation> {
  return http.post<Conversation>('/demo/incoming', payload)
}

/** GET /api/demo/products 可发送的商品卡列表（等价 /api/products） */
export function listDemoProducts(): Promise<Paged<Product>> {
  return http.get<Paged<Product>>('/demo/products')
}
