/**
 * 管理后台端（/admin/）—— 独立入口 admin.html + 独立路由。
 * 需要登录且必须是租户管理员。
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { getToken } from '@/shared/api/token'
import { useAuthStore } from '@/shared/stores/auth'
import { adminRoutes } from './routes'

/** 后台首页（重定向目标） */
export const ADMIN_HOME = '/dashboard'

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: ADMIN_HOME },
  // 登录页（仅本端可用）
  {
    path: '/login',
    name: 'login',
    component: () => import('@/shared/views/LoginView.vue'),
    meta: { public: true },
  },
  // 所有后台页面共用 AdminLayout 外壳（左侧菜单 + 顶栏）
  {
    path: '/',
    component: () => import('@/admin/AdminLayout.vue'),
    children: adminRoutes,
  },
  // 未知路径回后台首页，不外跳到其它端
  { path: '/:pathMatch(.*)*', redirect: ADMIN_HOME },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

/**
 * 子账号（客服）没有后台权限，直接整页跳到客服工作台。
 * 不能用 router.replace('/login') —— 守卫会把他们又送回 /admin，
 * 形成「登录页 ↔ 后台」来回弹跳，表现为「提示登录成功却不跳转、一直卡着」。
 */
function leaveToWorkbench(): false {
  window.location.replace('/chat/')
  return false
}

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
    // 已登录：管理员进后台，子账号去工作台
    return auth.isAdmin ? { path: ADMIN_HOME } : leaveToWorkbench()
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
  if (!auth.isAdmin) return leaveToWorkbench()

  return true
})

export default router
