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
    </div>

    <!-- 弹性占位，把底部操作区推到最下面 -->
    <div class="sidebar-spacer" />

    <div class="sidebar-bottom">
        <el-divider class="nav-divider" />
        <el-tooltip :content="`${auth.userName} · 个人资料`" placement="right">
          <div class="nav-item" @click="profileOpen = true">
            <span class="nav-icon nav-avatar">
              <img v-if="auth.user?.avatar" :src="auth.user.avatar" alt="" />
              <span v-else>{{ (auth.userName || '?').slice(0, 1) }}</span>
            </span>
          </div>
        </el-tooltip>
        <el-tooltip content="退出登录" placement="right">
          <div class="nav-item" @click="onLogout">
            <span class="nav-icon">🚪</span>
          </div>
        </el-tooltip>
      </div>

    <!-- 个人资料：修改昵称 / 头像 / 密码 -->
    <ProfileDialog v-model="profileOpen" />
  </aside>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/shared/stores/auth'
import { useChatStore } from '@/chat/stores/chat'
import ProfileDialog from '@/shared/components/ProfileDialog.vue'

const auth = useAuthStore()
const chat = useChatStore()

const stats = computed(() => chat.stats)
const filters = computed(() => chat.filters)

/** 个人资料弹窗（昵称 / 头像 / 密码） */
const profileOpen = ref(false)

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

/** 退出登录：清空登录态并回到登录页 */
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
  window.location.replace('/chat/#/login')
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

/* 弹性占位：把「个人资料 / 退出登录」推到侧边栏最底部，
   与上面的「全部会话 / 未接待」保持距离，不再紧挨着 */
.sidebar-spacer {
  flex: 1 1 auto;
  min-height: 0;
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
