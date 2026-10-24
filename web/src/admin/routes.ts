/**
 * 管理后台端内部路由（挂载在 /admin/ 入口下，使用相对路径）。
 * 独立入口后不再需要 /admin 前缀，路径如 /dashboard、/channels。
 */
import type { RouteRecordRaw } from 'vue-router'

export const adminRoutes: RouteRecordRaw[] = [
  {
    path: '/dashboard',
    name: 'admin-dashboard',
    component: () => import('./views/DashboardView.vue'),
    meta: { title: '概览' },
  },
  {
    path: '/channels',
    name: 'admin-channels',
    component: () => import('./views/ChannelsView.vue'),
    meta: { title: '渠道管理' },
  },
  {
    path: '/channel-types',
    name: 'admin-channel-types',
    component: () => import('./views/ChannelTypesView.vue'),
    meta: { title: '渠道类型' },
  },
  {
    path: '/auto-replies',
    name: 'admin-auto-replies',
    component: () => import('./views/AutoRepliesView.vue'),
    meta: { title: '自动回复' },
  },
  {
    path: '/quick-replies',
    name: 'admin-quick-replies',
    component: () => import('./views/QuickRepliesView.vue'),
    meta: { title: '快捷回复' },
  },
  {
    path: '/agents',
    name: 'admin-agents',
    component: () => import('./views/AgentsView.vue'),
    meta: { title: '子账号管理' },
  },
  {
    path: '/users',
    name: 'admin-users',
    component: () => import('./views/UsersView.vue'),
    meta: { title: '客户管理' },
  },
  {
    path: '/conversation-records',
    name: 'admin-conversation-records',
    component: () => import('./views/ConversationRecordsView.vue'),
    meta: { title: '对话记录' },
  },
  {
    path: '/orders',
    name: 'admin-orders',
    component: () => import('./views/OrdersView.vue'),
    meta: { title: '订单管理' },
  },
  {
    path: '/products',
    name: 'admin-products',
    component: () => import('./views/ProductsView.vue'),
    meta: { title: '商品管理' },
  },
  {
    path: '/devices',
    name: 'admin-devices',
    component: () => import('./views/DevicesView.vue'),
    meta: { title: '设备管理' },
  },
  {
    path: '/ws-connections',
    name: 'admin-ws-connections',
    component: () => import('./views/WsConnectionsView.vue'),
    meta: { title: '连接管理' },
  },
  {
    path: '/settings',
    name: 'admin-settings',
    component: () => import('./views/SettingsView.vue'),
    meta: { title: '系统设置' },
  },
]
