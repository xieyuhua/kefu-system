import { defineStore } from 'pinia'
import { login as apiLogin, logout as apiLogout, me as apiMe } from '@/api/auth'
import { clearToken, getToken, setToken } from '@/api/token'
import type { LoginPayload, User } from '@/types'

interface AuthState {
  token: string
  user: User | null
  loading: boolean
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    token: getToken(),
    user: null,
    loading: false,
  }),
  getters: {
    isLogin: (state): boolean => !!state.token,
    isAdmin: (state): boolean => state.user?.role === 'admin',
    userName: (state): string => state.user?.name || state.user?.account || '未登录',
  },
  actions: {
    async login(payload: LoginPayload): Promise<void> {
      this.loading = true
      try {
        const res = await apiLogin(payload)
        this.token = res.token
        this.user = res.user
        setToken(res.token)
      } finally {
        this.loading = false
      }
    },
    async me(): Promise<void> {
      if (!this.token) return
      this.user = await apiMe()
    },
    async logout(): Promise<void> {
      try {
        if (this.token) {
          await apiLogout()
        }
      } catch {
        // 后端可能不可用，忽略失败，前端仍要清理登录态
      }
      this.token = ''
      this.user = null
      clearToken()
    },
    reset(): void {
      this.token = ''
      this.user = null
      clearToken()
    },
  },
})
