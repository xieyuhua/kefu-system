/**
 * 客服工作台端（/chat/）—— 独立入口 chat.html + 独立路由。
 * 需要登录；管理员与客服都可进入。
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { getToken } from '@/api/token'

export const CHAT_HOME = '/'

const routes: RouteRecordRaw[] = [
  // 登录页
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true },
  },
  // 工作台首页
  {
    path: CHAT_HOME,
    name: 'workbench',
    component: () => import('@/views/WorkbenchView.vue'),
  },
  // 历史档案
  {
    path: '/history',
    name: 'history',
    component: () => import('@/views/HistoryView.vue'),
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

  if (to.path === '/login') {
    // 已登录直接进工作台
    return token ? { path: CHAT_HOME } : true
  }

  if (!token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  return true
})

export default router
