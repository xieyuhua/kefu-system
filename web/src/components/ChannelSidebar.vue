<template>
  <aside class="channel-sidebar">
    <div class="sidebar-top">
      <div
        class="nav-item"
        :class="{ active: !pendingOnly && filters.channelId === 0 }"
        title="全部会话（所有渠道合并，列表内标注来源）"
        @click="showAll"
      >
        <span class="nav-icon">🗂</span>
        <span class="badge" v-if="totalUnread > 0">{{ badge(totalUnread) }}</span>
      </div>

      <div
        class="nav-item"
        :class="{ active: pendingOnly }"
        :title="`未接待 ${stats.pending} 条`"
        @click="togglePending"
      >
        <span class="nav-icon">🔔</span>
        <span class="badge" v-if="stats.pending > 0">{{ badge(stats.pending) }}</span>
      </div>

      <el-divider class="nav-divider" />
    </div>

    <div class="sidebar-channels kf-scroll">
      <!-- 渠道来源只作「标注」展示在会话列表里，这里只保留统计概览，不再逐个切换 -->
      <div
        v-for="ch in channels"
        :key="ch.id"
        class="stat-item"
        :class="{ 'stat-item--alert': ch.pending_count > 0 }"
        :title="`${ch.name}：待接待 ${ch.pending_count}`"
      >
        <span class="stat-icon" :style="{ background: channelColor(ch.type) }">
          {{ channelIcon(ch.type) }}
        </span>
        <span class="stat-badge" v-if="ch.pending_count > 0">{{ badge(ch.pending_count) }}</span>
      </div>

      <div v-if="!channels.length && !loading" class="nav-empty">无渠道</div>
    </div>

    <div class="sidebar-bottom">
      <el-divider class="nav-divider" />
      <el-tooltip :content="auth.userName" placement="right">
        <div class="nav-item" @click="onProfile">
          <span class="nav-icon nav-avatar">
            <img v-if="auth.user?.avatar" :src="auth.user.avatar" alt="" />
            <span v-else>{{ (auth.userName || '?').slice(0, 1) }}</span>
          </span>
        </div>
      </el-tooltip>
      <div class="nav-item" title="退出登录" @click="onLogout">
        <span class="nav-icon">🚪</span>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'
import { channelColor, channelIcon } from '@/utils/format'

const auth = useAuthStore()
const chat = useChatStore()
const router = useRouter()

const channels = computed(() => chat.channels)
const stats = computed(() => chat.stats)
const loading = computed(() => chat.listLoading)
const filters = computed(() => chat.filters)

// 会话已合并为单一列表，渠道只作为来源标注；这里仅保留「未接待」筛选
const pendingOnly = computed(() => filters.value.status === 'pending')
const totalUnread = computed(() => stats.value.unread)

function badge(n: number): string {
  return n > 99 ? '99+' : String(n)
}

// 全部会话：清空渠道筛选，所有渠道合并展示（每条会话带来源渠道标注）
function showAll(): void {
  chat.setFilter({ channelId: 0, status: 'all' })
}

// 未接待：所有渠道的待接待会话合并展示，角标为汇总数量
function togglePending(): void {
  chat.setFilter({ channelId: 0, status: pendingOnly.value ? 'all' : 'pending' })
}

function onProfile(): void {
  void router.push('/chat/history')
}

async function onLogout(): Promise<void> {
  try {
    await ElMessageBox.confirm('确定退出登录吗？', '提示', {
      type: 'warning',
      confirmButtonText: '退出',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  await auth.logout()
  void router.replace('/login')
}
</script>

<style scoped lang="scss">
.channel-sidebar {
  display: flex;
  flex-direction: column;
  width: var(--kf-nav-width);
  min-width: var(--kf-nav-width);
  background: #2e2f33;
  color: #fff;
}

.sidebar-top,
.sidebar-bottom {
  flex-shrink: 0;
}

.sidebar-channels {
  flex: 1;
  padding: 4px 0;
}

.nav-divider {
  margin: 6px 8px;
  border-color: rgba(255, 255, 255, 0.12);
  width: calc(100% - 16px);
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 48px;
  margin: 4px 6px;
  border-radius: var(--kf-radius);
  cursor: pointer;

  &:hover {
    background: rgba(255, 255, 255, 0.08);
  }

  &.active {
    background: rgba(7, 193, 96, 0.18);
  }
}

.nav-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.1);
  font-size: 17px;
  overflow: hidden;
}

.nav-avatar {
  background: var(--kf-primary);
  color: #fff;
  font-size: 14px;
  font-weight: 600;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

/* 渠道仅作来源统计展示（不再用于切换），无点击事件 */
.stat-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 40px;
  margin: 3px 6px;
  border-radius: var(--kf-radius);
  opacity: 0.75;

  &--alert {
    opacity: 1;
  }
}

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 5px;
  font-size: 14px;
}

.stat-badge {
  position: absolute;
  top: 2px;
  right: 2px;
  min-width: 15px;
  height: 15px;
  padding: 0 3px;
  border-radius: 8px;
  background: var(--kf-red);
  color: #fff;
  font-size: 10px;
  line-height: 15px;
  text-align: center;
}

.badge {
  position: absolute;
  top: 4px;
  right: 4px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: var(--kf-red);
  color: #fff;
  font-size: 11px;
  line-height: 16px;
  text-align: center;
}

.nav-empty {
  padding: 8px 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 12px;
  text-align: center;
}
</style>
