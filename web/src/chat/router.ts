/**
 * 客服工作台端（/chat/）—— 独立入口 chat.html + 独立路由。
 * 需要登录；管理员与客服都可进入。
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { getToken } from '@/shared/api/token'
import { useAuthStore } from '@/shared/stores/auth'

export const CHAT_HOME = '/'

const routes: RouteRecordRaw[] = [
  // 登录页
  {
    path: '/login',
    name: 'login',
    component: () => import('@/shared/views/LoginView.vue'),
    meta: { public: true },
  },
  // 工作台首页
  {
    path: CHAT_HOME,
    name: 'workbench',
    component: () => import('@/chat/WorkbenchView.vue'),
  },
  // 未知路径回工作台首页
  { path: '/:pathMatch(.*)*', redirect: CHAT_HOME },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const token = getToken()
  const auth = useAuthStore()

  if (to.path === '/login') {
    // 已登录（含子账号）一律直接进工作台，不要停在登录页
    return token ? { path: CHAT_HOME } : true
  }

  if (!token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  // token 有效但 Pinia 里的 user 为空（刷新页面后必然如此）→ 补拉一次，
  // 否则昵称会退化成「未登录」、头像空白、个人资料打不开。
  if (!auth.user) {
    try {
      await auth.me()
    } catch {
      return { path: '/login', query: { redirect: to.fullPath } }
    }
  }

  return true
})

export default router
