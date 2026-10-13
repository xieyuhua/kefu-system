import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { getToken } from '@/api/token'
import { useAuthStore } from '@/stores/auth'
import { adminRoutes } from './adminRoutes'

/**
 * 三端独立前缀：
 *   /        → 用户端（客户端聊天页）
 *   /chat/   → 客服工作台（需登录）
 *   /admin/  → 管理后台（需登录 + 管理员）
 * 每个前缀都用自己的 hash 路由（如 /admin/#/channels），互不干扰、可单独部署与鉴权。
 * 旧的 /login、/history 路径保留兼容，会重定向到对应前缀。
 */
const routes: RouteRecordRaw[] = [
  // ---------- 用户端 ----------
  {
    path: '/',
    name: 'client',
    component: () => import('@/views/ClientView.vue'),
    meta: { public: true },
  },

  // ---------- 客服工作台 ----------
  {
    path: '/chat',
    redirect: '/chat/',
  },
  {
    path: '/chat/',
    component: () => import('@/views/ChatLayout.vue'),
    children: [
      {
        path: '',
        name: 'workbench',
        component: () => import('@/views/WorkbenchView.vue'),
      },
      {
        path: 'history',
        name: 'history',
        component: () => import('@/views/HistoryView.vue'),
      },
    ],
  },

  // ---------- 登录（独立页，工作台/后台共用） ----------
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true },
  },

  // ---------- 兼容旧路径 ----------
  { path: '/c', redirect: '/' },
  { path: '/workbench', redirect: '/chat/' },
  { path: '/history', redirect: '/chat/history' },

  // ---------- 管理后台 ----------
  ...adminRoutes,

  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const token = getToken()

  // 已登录访问登录页 → 按角色回各自入口（管理员回后台，客服回工作台）
  if (to.path === '/login' && token) {
    const auth = useAuthStore()
    if (!auth.user) {
      try {
        await auth.me()
      } catch {
        return true
      }
    }
    return { path: auth.isAdmin ? '/admin/' : '/chat/' }
  }

  if (!to.meta.public && !token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  // 后台管理：仅租户管理员可进入，先补齐用户信息再判断角色
  if (to.meta.requiresAdmin) {
    const auth = useAuthStore()
    if (!auth.user) {
      try {
        await auth.me()
      } catch {
        return { path: '/login' }
      }
    }
    if (!auth.isAdmin) {
      return { path: '/chat/' }
    }
  }

  return true
})

export default router
