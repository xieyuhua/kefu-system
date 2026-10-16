import http from './http'

/** 修改资料返回的用户信息 */
export interface ProfileResult {
  id: number
  name: string
  avatar: string
  account: string
  role: string
}

/** PUT /api/auth/profile —— 修改昵称与头像 */
export function updateProfile(payload: { name?: string; avatar?: string }): Promise<ProfileResult> {
  return http.put<ProfileResult>('/auth/profile', payload)
}

/** PUT /api/auth/password —— 修改密码 */
export function changePassword(payload: {
  old_password: string
  new_password: string
}): Promise<boolean> {
  return http.put<boolean>('/auth/password', payload)
}