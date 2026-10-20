/**
 * 快捷回复（常用语）接口封装。
 *
 * 两套端点，语义分开：
 *   - 客服端（工作台，登录即可用）：`/api/quick-replies`
 *     列表 = 自己的私有 + 租户共享（user_id = 0）；改 / 删只能动自己的。
 *   - 后台端（管理员维护租户全部，含公共话术）：`/api/admin/quick-replies`
 *     挂在 /admin/ 前缀下，避免与客服端同 method+path（同 engine 重复注册会冲突）。
 *
 * http 拦截器已剥掉 { code, message, data } 外壳，这里直接返回业务 data。
 */
import http from './http'

/** 快捷回复（后端 QuickReplyVO） */
export interface QuickReply {
  id: number
  /** 0 = 租户共享（管理员维护）；>0 = 该客服私有 */
  user_id: number
  title: string
  content: string
  sort: number
  enabled: boolean
  /** user_id = 0：公共话术 */
  shared: boolean
  /** 当前登录账号能否改 / 删（公共话术与别人的私有项为 false） */
  editable: boolean
  /** 「公共」或创建人昵称 */
  owner: string
  created_at: string
}

/** 新增 / 修改请求体；sort、enabled 不传表示保留原值 */
export interface QuickReplyPayload {
  title: string
  content: string
  sort?: number
  enabled?: boolean
  /** 仅管理员端点生效：指定归属（0 = 公共话术） */
  user_id?: number
}

/* ------------------------------------------------------------------ *
 * 客服端（工作台）：自己的私有 + 租户共享
 * ------------------------------------------------------------------ */

/** GET /api/quick-replies?keyword= */
export function listQuickReplies(keyword?: string): Promise<QuickReply[]> {
  return http.get<QuickReply[]>('/quick-replies', { params: keyword ? { keyword } : undefined })
}

/** POST /api/quick-replies —— 归属强制为当前登录账号（后端忽略 user_id） */
export function createQuickReply(payload: QuickReplyPayload): Promise<QuickReply> {
  return http.post<QuickReply>('/quick-replies', payload)
}

/** PUT /api/quick-replies/:id —— 只能改自己的，否则返回中文业务错误 */
export function updateQuickReply(id: number, payload: QuickReplyPayload): Promise<boolean> {
  return http.put<boolean>(`/quick-replies/${id}`, payload)
}

/** DELETE /api/quick-replies/:id —— 同上限制 */
export function deleteQuickReply(id: number): Promise<boolean> {
  return http.del<boolean>(`/quick-replies/${id}`)
}

/* ------------------------------------------------------------------ *
 * 后台端：租户全部（管理员维护公共话术）
 * ------------------------------------------------------------------ */

/** GET /api/admin/quick-replies?keyword= —— 含所有客服的私有项 */
export function listTenantQuickReplies(keyword?: string): Promise<QuickReply[]> {
  return http.get<QuickReply[]>('/admin/quick-replies', {
    params: keyword ? { keyword } : undefined,
  })
}

/** POST /api/admin/quick-replies —— 默认建为公共话术（user_id = 0） */
export function createTenantQuickReply(payload: QuickReplyPayload): Promise<QuickReply> {
  return http.post<QuickReply>('/admin/quick-replies', payload)
}

/** PUT /api/admin/quick-replies/:id —— 管理员可改租户内任意一条 */
export function updateTenantQuickReply(id: number, payload: QuickReplyPayload): Promise<boolean> {
  return http.put<boolean>(`/admin/quick-replies/${id}`, payload)
}

/** DELETE /api/admin/quick-replies/:id —— 管理员可删租户内任意一条 */
export function deleteTenantQuickReply(id: number): Promise<boolean> {
  return http.del<boolean>(`/admin/quick-replies/${id}`)
}
