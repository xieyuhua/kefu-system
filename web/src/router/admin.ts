/**
 * 管理后台端（/admin/）—— 独立入口 admin.html + 独立路由。
 * 需要登录且必须是租户管理员。
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { getToken } from '@/api/token'
import { useAuthStore } from '@/stores/auth'
import { adminRoutes } from './adminRoutes'

/** 后台首页（重定向目标） */
export const ADMIN_HOME = '/dashboard'

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: ADMIN_HOME },
  // 登录页（仅本端可用）
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true },
  },
  // 所有后台页面共用 AdminLayout 外壳（左侧菜单 + 顶栏）
  {
    path: '/',
    component: () => import('@/views/admin/AdminLayout.vue'),
    children: adminRoutes,
  },
  // 未知路径回后台首页，不外跳到其它端
  { path: '/:pathMatch(.*)*', redirect: ADMIN_HOME },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const token = getToken()
  const auth = useAuthStore()

  if (to.path === '/login') {
    if (!token) return true
    if (!auth.user) {
      try {
        await auth.me()
      } catch {
        return true
      }
    }
    return auth.isAdmin ? { path: ADMIN_HOME } : { path: '/login' }
  }

  if (!token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  // 仅管理员可进
  if (!auth.user) {
    try {
      await auth.me()
    } catch {
      return { path: '/login' }
    }
  }
  if (!auth.isAdmin) return { path: '/login' }

  return true
})

export default router
