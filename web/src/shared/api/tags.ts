import http from './http'
import type { TagRef } from '@/shared/types'

/** 标签库项 */
export interface Tag {
  id: number
  name: string
  color: string
  sort: number
  use_count: number
  created_at: string
}

/** GET /api/tags —— 租户标签库 */
export function listTags(): Promise<Tag[]> {
  return http.get<Tag[]>('/tags')
}

/** POST /api/tags —— 新建标签 */
export function createTag(payload: { name: string; color?: string; sort?: number }): Promise<Tag> {
  return http.post<Tag>('/tags', payload)
}

/** DELETE /api/tags/:id —— 删除标签 */
export function deleteTag(id: number): Promise<boolean> {
  return http.del<boolean>(`/tags/${id}`)
}

/** PUT /api/users/:id/tags —— 全量覆盖客户的标签集合，返回最新标签 */
export function setCustomerTags(customerId: number, tagIds: number[]): Promise<TagRef[]> {
  return http.put<TagRef[]>(`/users/${customerId}/tags`, { tag_ids: tagIds })
}

/** PUT /api/users/:id/remark —— 更新客户备注（跨客服可见） */
export function updateCustomerRemark(customerId: number, remark: string): Promise<boolean> {
  return http.put<boolean>(`/users/${customerId}/remark`, { remark })
}
