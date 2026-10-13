/**
 * /admin 下的后台管理子路由。
 * 工作台路由由 web/src/router/index.ts 维护，这里只导出数组，避免两边同时改同一个文件冲突。
 */
import type { RouteRecordRaw } from 'vue-router'

export const adminRoutes: RouteRecordRaw[] = [
  {
    path: '/admin',
    name: 'admin',
    component: () => import('../views/admin/AdminLayout.vue'),
    redirect: '/admin/dashboard',
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      {
        path: 'dashboard',
        name: 'admin-dashboard',
        component: () => import('../views/admin/DashboardView.vue'),
        meta: { title: '概览' },
      },
      {
        path: 'channels',
        name: 'admin-channels',
        component: () => import('../views/admin/ChannelsView.vue'),
        meta: { title: '渠道管理' },
      },
      {
        path: 'channel-types',
        name: 'admin-channel-types',
        component: () => import('../views/admin/ChannelTypesView.vue'),
        meta: { title: '渠道类型' },
      },
      {
        path: 'auto-replies',
        name: 'admin-auto-replies',
        component: () => import('../views/admin/AutoRepliesView.vue'),
        meta: { title: '自动回复' },
      },
      {
        path: 'agents',
        name: 'admin-agents',
        component: () => import('../views/admin/AgentsView.vue'),
        meta: { title: '子账号管理' },
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('../views/admin/UsersView.vue'),
        meta: { title: '客户管理' },
      },
      {
        path: 'orders',
        name: 'admin-orders',
        component: () => import('../views/admin/OrdersView.vue'),
        meta: { title: '订单管理' },
      },
      {
        path: 'products',
        name: 'admin-products',
        component: () => import('../views/admin/ProductsView.vue'),
        meta: { title: '商品管理' },
      },
      {
        path: 'devices',
        name: 'admin-devices',
        component: () => import('../views/admin/DevicesView.vue'),
        meta: { title: '设备管理' },
      },
    ],
  },
]
