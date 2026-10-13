import http from './http'
import type { LoginPayload, LoginResult, User } from '@/types'

/** POST /api/auth/login */
export function login(payload: LoginPayload): Promise<LoginResult> {
  return http.post<LoginResult>('/auth/login', payload)
}

/** GET /api/auth/me */
export function me(): Promise<User> {
  return http.get<User>('/auth/me')
}

/** POST /api/auth/logout */
export function logout(): Promise<boolean> {
  return http.post<boolean>('/auth/logout')
}
