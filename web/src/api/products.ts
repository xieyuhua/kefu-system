import http from './http'
import type { Paged, Product } from '@/types'

export interface ProductQuery {
  keyword?: string
  page?: number
  page_size?: number
}

/** GET /api/products */
export function listProducts(query: ProductQuery = {}): Promise<Paged<Product>> {
  const usp = new URLSearchParams()
  if (query.keyword) usp.set('keyword', query.keyword)
  if (query.page) usp.set('page', String(query.page))
  if (query.page_size) usp.set('page_size', String(query.page_size))
  const qs = usp.toString()
  return http.get<Paged<Product>>(`/products${qs ? `?${qs}` : ''}`)
}
