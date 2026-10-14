<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ChatRound,
  CollectionTag,
  DataLine,
  Grid,
  Headset,
  Iphone,
  Message,
  Monitor,
  ShoppingCart,
  SwitchButton,
  Tickets,
  UserFilled,
} from '@element-plus/icons-vue'
import { useAuthStore } from '../../stores/auth'

interface MenuItem {
  path: string
  title: string
  icon: typeof Grid
}

const MENUS: MenuItem[] = [
  { path: '/dashboard', title: '概览', icon: DataLine },
  { path: '/channels', title: '渠道管理', icon: Grid },
  { path: '/channel-types', title: '渠道类型', icon: CollectionTag },
  { path: '/auto-replies', title: '自动回复', icon: ChatRound },
  { path: '/agents', title: '子账号管理', icon: UserFilled },
  { path: '/users', title: '客户管理', icon: Headset },
  { path: '/orders', title: '订单管理', icon: Tickets },
  { path: '/products', title: '商品管理', icon: ShoppingCart },
  { path: '/devices', title: '设备管理', icon: Iphone },
]

const route = useRouter()
const current = useRoute()
const auth = useAuthStore()

const tenantName = computed(() => auth.user?.tenant?.name || '客服工作台')
const userName = computed(() => auth.user?.name || auth.user?.account || '未登录')
const userRole = computed(() => (auth.user?.role === 'admin' ? '管理员' : '客服'))
const pageTitle = computed(() => (current.meta.title as string | undefined) || '概览')

/** 跨端跳转：工作台在另一个入口 /chat/ */
const WORKBENCH_URL = '/chat/'

function goWorkbench(): void {
  window.location.href = WORKBENCH_URL
}

async function onLogout(): Promise<void> {
  await auth.logout()
  void route.push('/login')
}
</script>

<template>
  <div class="admin admin-layout">
    <aside class="admin-sidebar">
      <div class="admin-brand">
        <el-icon :size="18" color="#07C160"><Message /></el-icon>
        <span class="admin-brand__text">管理后台</span>
      </div>

      <nav class="admin-menu">
        <router-link
          v-for="item in MENUS"
          :key="item.path"
          :to="item.path"
          class="admin-menu__item"
          active-class="admin-menu__item--active"
        >
          <el-icon :size="16"><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </router-link>
      </nav>

      <div class="admin-sidebar__footer">
        <button type="button" class="admin-link-btn" @click="goWorkbench">
          <el-icon :size="14"><Monitor /></el-icon>
          <span>返回工作台</span>
        </button>
      </div>
    </aside>

    <div class="admin-main">
      <header class="admin-header">
        <div class="admin-header__left">
          <span class="admin-header__title">{{ pageTitle }}</span>
          <span class="admin-header__tenant">{{ tenantName }}</span>
        </div>
        <div class="admin-header__right">
          <div class="admin-user">
            <span class="admin-user__name">{{ userName }}</span>
            <span class="admin-user__role">{{ userRole }}</span>
          </div>
          <button type="button" class="admin-link-btn" @click="onLogout">
            <el-icon :size="14"><SwitchButton /></el-icon>
            <span>退出登录</span>
          </button>
        </div>
      </header>

      <main class="admin-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped lang="scss">
.admin-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* 左侧菜单：浅灰底，选中项浅灰高亮 + 左侧绿色竖条 */
.admin-sidebar {
  width: 200px;
  flex: 0 0 200px;
  background: #f7f8fa;
  border-right: 1px solid #e5e5e5;
  display: flex;
  flex-direction: column;
}

.admin-brand {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-bottom: 1px solid #e5e5e5;

  &__text {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }
}

.admin-menu {
  flex: 1;
  padding: 8px 0;
  overflow-y: auto;

  &__item {
    position: relative;
    display: flex;
    align-items: center;
    gap: 10px;
    height: 40px;
    padding: 0 16px;
    color: #191919;
    font-size: 14px;
    text-decoration: none;

    &:hover {
      background: #eceef1;
    }

    &--active {
      background: #e9eaec;
      color: #07c160;

      &::before {
        content: '';
        position: absolute;
        left: 0;
        top: 8px;
        bottom: 8px;
        width: 3px;
        border-radius: 0 2px 2px 0;
        background: #07c160;
      }
    }
  }
}

.admin-sidebar__footer {
  padding: 12px 16px;
  border-top: 1px solid #e5e5e5;
}

.admin-link-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  padding: 0;
  font-size: 13px;
  color: #7f7f7f;
  cursor: pointer;

  &:hover {
    color: #07c160;
  }
}

.admin-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* 顶栏白色：租户名 + 当前用户 + 退出登录 */
.admin-header {
  height: 56px;
  flex: 0 0 56px;
  background: #fff;
  border-bottom: 1px solid #e5e5e5;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;

  &__left {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }

  &__title {
    font-size: 16px;
    font-weight: 600;
    color: #191919;
  }

  &__tenant {
    font-size: 13px;
    color: #7f7f7f;
  }

  &__right {
    display: flex;
    align-items: center;
    gap: 16px;
  }
}

.admin-user {
  display: flex;
  align-items: center;
  gap: 8px;

  &__name {
    font-size: 14px;
    color: #191919;
  }

  &__role {
    font-size: 12px;
    color: #7f7f7f;
    background: #f5f5f5;
    border-radius: 3px;
    padding: 1px 6px;
  }
}

.admin-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  background: #f5f5f5;
}
</style>
