/**
 * 用户端（/kefu/）—— 独立入口 kefu.html + 独立路由。
 * 无需登录：客户通过 URL 参数直接接入聊天。
 */
import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  // 聊天页（对接参数通过 URL query 传递：?channel=web&uid=xxx&pid=xxx）
  {
    path: '/',
    name: 'client',
    component: () => import('@/client/ClientView.vue'),
  },
  // 兼容旧路径 /c
  { path: '/c', redirect: '/' },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

export default router
