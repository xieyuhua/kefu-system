import axios, { type AxiosInstance, type AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/shared/types'
import { clearToken, getToken } from './token'

const TOKEN_PREFIX = 'Bearer '

/**
 * 登录态失效时统一处理。
 *
 * 关键：必须用「整页跳转」而不是改 hash。
 * 之前只改 hash → 路由守卫又把用户送回 /login（子账号在 /admin 会被弹去 /chat，
 * 再被 chat 守卫弹回），形成无限弹跳，表现就是「一直卡在登录页」。
 * 整页 reload 会重新走一次守卫，且此时 token 已清空，能稳定停在登录页。
 */
let redirecting = false

function redirectToLogin(): void {
  if (redirecting) return
  redirecting = true
  clearToken()
  const { pathname, search, hash } = window.location
  // 已在本端登录页就不重复跳转，避免循环
  if (hash.replace(/^#/, '').endsWith('/login')) {
    redirecting = false
    window.location.reload()
    return
  }
  window.location.replace(`${pathname}${search}#/login`)
}

/** 从统一响应壳 { code, message, data } 中取出 data，非 0 抛错 */
function unwrap(payload: unknown): unknown {
  if (payload && typeof payload === 'object' && 'code' in payload) {
    const body = payload as ApiResponse<unknown>
    if (body.code === 0) {
      return body.data
    }
    if (body.code === 401) {
      redirectToLogin()
    }
    const err = new Error(body.message || `请求失败（${body.code}）`) as Error & { code?: number }
    err.code = body.code
    throw err
  }
  return payload
}

const client: AxiosInstance = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

client.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = TOKEN_PREFIX + token
  }
  return config
})

client.interceptors.response.use(
  (response) => {
    try {
      return unwrap(response.data) as never
    } catch (err) {
      if (err instanceof Error && err.message) {
        ElMessage.error(err.message)
      }
      return Promise.reject(err)
    }
  },
  (error: unknown) => {
    const axiosError = error as { response?: { data?: { message?: string } }; message?: string }
    const message = axiosError?.response?.data?.message || axiosError?.message || '网络异常'
    ElMessage.error(message)
    return Promise.reject(new Error(message))
  },
)

/** 类型安全的请求封装：拦截器已剥掉 { code, message, data } 外壳 */
const http = {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return client.get(url, config) as unknown as Promise<T>
  },
  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return client.post(url, data, config) as unknown as Promise<T>
  },
  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return client.put(url, data, config) as unknown as Promise<T>
  },
  del<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return client.delete(url, config) as unknown as Promise<T>
  },
}

export default http
