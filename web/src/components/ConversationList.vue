<template>
  <section class="conv-list">
    <header class="list-header">
      <el-input
        v-model="keyword"
        size="default"
        placeholder="搜索昵称 / 消息"
        clearable
        :prefix-icon="Search"
        @keyup.enter="onSearch"
        @clear="onSearch"
      />
    </header>

    <div class="tabs">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab"
        :class="{ active: filters.status === tab.key }"
        @click="onTab(tab.key)"
      >
        {{ tab.label }}
        <span v-if="countOf(tab.key) > 0" class="tab-count">{{ countOf(tab.key) }}</span>
      </button>
    </div>

    <div v-loading="loading" class="list-body kf-scroll">
      <div
        v-for="c in conversations"
        :key="c.id"
        class="conv-item"
        :class="{ active: c.id === currentId }"
        @click="emit('select', c)"
      >
        <div class="conv-avatar">
          <img v-if="c.user?.avatar" :src="c.user.avatar" alt="" />
          <span v-else>{{ (c.user?.nickname || '?').slice(0, 1) }}</span>
          <span class="dot" v-if="c.unread > 0">{{ c.unread > 99 ? '99+' : c.unread }}</span>
        </div>

        <div class="conv-main">
          <div class="conv-line">
            <span class="conv-name kf-ellipsis">{{ c.user?.nickname || '匿名客户' }}</span>
            <span class="conv-time">{{ formatListTime(c.last_message_at || c.created_at) }}</span>
          </div>
          <div class="conv-line">
            <span class="conv-msg kf-ellipsis">{{ preview(c) }}</span>
            <span class="dot-inline" v-if="c.unread > 0">{{ c.unread > 99 ? '99+' : c.unread }}</span>
          </div>
          <div class="conv-line conv-line--meta">
            <!-- 来源渠道标注：所有渠道会话合并展示，靠这里区分 -->
            <span class="conv-source" :title="c.channel?.name">
              <i class="conv-source__dot" :style="{ background: channelColor(c.channel?.type) }"></i>
              <span class="conv-source__icon">{{ channelIcon(c.channel?.type) }}</span>
              <span class="conv-source__name kf-ellipsis">{{
                c.channel?.name || channelLabel(c.channel?.type)
              }}</span>
            </span>
            <!-- 客户归属地：IP + 城市 -->
            <span
              v-if="c.user?.city || c.user?.ip"
              class="conv-geo"
              :title="geoTitle(c.user)"
            >
              <span class="conv-geo__icon">📍</span>
              <span v-if="c.user?.city" class="conv-geo__city">{{ c.user.city }}</span>
              <span v-if="c.user?.ip" class="conv-geo__ip">{{ c.user.ip }}</span>
            </span>
            <span v-if="c.status === 'pending'" class="conv-state conv-state--pending">未接待</span>
            <span v-else-if="c.status === 'open'" class="conv-state conv-state--open">进行中</span>
            <span v-else class="conv-state">已结束</span>
          </div>
        </div>
      </div>

      <el-empty v-if="!loading && !conversations.length" description="暂无会话" :image-size="72" />
    </div>

    <footer class="list-footer">
      <span>共 {{ total }} 条</span>
      <el-button size="small" text :icon="Refresh" @click="emit('refresh')">刷新</el-button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import type { Conversation } from '@/types'
import { channelColor, channelIcon, channelLabel, formatListTime } from '@/utils/format'
import type { StatusFilter } from '@/stores/chat'

const props = defineProps<{
  conversations: Conversation[]
  currentId: number
  loading: boolean
  total: number
  stats: { pending: number; open: number; closed: number; unread: number }
  status: StatusFilter
}>()

const emit = defineEmits<{
  select: [conversation: Conversation]
  'update:status': [status: StatusFilter]
  'update:keyword': [keyword: string]
  refresh: []
}>()

const keyword = ref('')

watch(keyword, (val) => emit('update:keyword', val))

const filters = computed(() => ({ status: props.status }))

const tabs: { key: StatusFilter; label: string }[] = [
  { key: 'open', label: '进行中' },
  { key: 'pending', label: '待接待' },
  { key: 'closed', label: '已结束' },
  { key: 'all', label: '全部' },
]

function countOf(key: StatusFilter): number {
  switch (key) {
    case 'open':
      return props.stats.open
    case 'pending':
      return props.stats.pending
    case 'closed':
      return props.stats.closed
    default:
      return props.stats.open + props.stats.pending
  }
}

function onTab(key: StatusFilter): void {
  emit('update:status', key)
}

function onSearch(): void {
  emit('update:keyword', keyword.value)
}

function preview(c: Conversation): string {
  return c.last_message || '[暂无消息]'
}

/** IP + 城市 的完整提示文本 */
function geoTitle(u: Conversation['user']): string {
  const parts: string[] = []
  if (u?.city) parts.push(`归属地：${u.city}`)
  if (u?.ip) parts.push(`IP：${u.ip}`)
  return parts.join('\n')
}
</script>

<style scoped lang="scss">
.conv-list {
  display: flex;
  flex-direction: column;
  width: var(--kf-list-width);
  min-width: 260px;
  background: #fff;
  border-right: 1px solid var(--kf-border);
}

/* ---------------- 移动端：占满整屏 ---------------- */
@media (max-width: 768px) {
  .conv-list {
    width: 100%;
    min-width: 0;
    border-right: none;
  }

  .list-header {
    padding: 8px 10px;
  }

  .conv-item {
    padding: 10px;
  }

  /* 移动端触摸友好：加大点击区域 */
  .tab {
    padding: 8px 0;
  }
}

.list-header {
  padding: 10px 12px;
  border-bottom: 1px solid var(--kf-border-light);
}

.tabs {
  display: flex;
  padding: 6px 8px;
  border-bottom: 1px solid var(--kf-border-light);
  gap: 4px;
}

.tab {
  flex: 1;
  padding: 5px 0;
  border: none;
  border-radius: var(--kf-radius-sm);
  background: transparent;
  color: var(--kf-text-sub);
  font-size: 12px;
  cursor: pointer;

  &:hover {
    background: var(--kf-bg-hover);
  }

  &.active {
    background: var(--kf-primary-light);
    color: var(--kf-primary);
    font-weight: 600;
  }
}

.tab-count {
  margin-left: 2px;
  font-size: 11px;
  opacity: 0.85;
}

.list-body {
  flex: 1;
}

.conv-item {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  cursor: pointer;
  border-bottom: 1px solid var(--kf-border-light);

  &:hover {
    background: var(--kf-bg-hover);
  }

  &.active {
    background: var(--kf-bg-active);
  }
}

.conv-avatar {
  position: relative;
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg-active);
  color: var(--kf-text-sub);
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .dot {
    position: absolute;
    top: -4px;
    right: -4px;
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
}

.conv-main {
  flex: 1;
  min-width: 0;
}

.conv-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;

  & + .conv-line {
    margin-top: 4px;
  }
}

.conv-name {
  font-size: 14px;
  color: var(--kf-text);
  font-weight: 500;
}

.conv-time {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--kf-text-weak);
}

.conv-msg {
  flex: 1;
  font-size: 12px;
  color: var(--kf-text-sub);
}

/* ---------- 来源渠道标注（所有渠道合并展示，靠这里区分） ---------- */
.conv-line--meta {
  justify-content: flex-start;
  gap: 8px;
}

.conv-source {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  max-width: 62%;
  height: 18px;
  padding: 0 6px 0 5px;
  border-radius: 3px;
  background: var(--kf-bg-active);
  font-size: 11px;
  color: var(--kf-text-sub);

  &__dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  &__icon {
    font-size: 11px;
    line-height: 1;
  }

  &__name {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
}

/* ---------- 客户归属地（IP + 城市） ---------- */
.conv-geo {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
  max-width: 52%;
  padding: 0 5px;
  border-radius: 3px;
  background: #f2f6ff;
  color: #4a6fa5;
  font-size: 11px;
  line-height: 18px;
  cursor: default;

  &__icon {
    font-size: 10px;
  }

  &__city {
    flex-shrink: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  &__ip {
    flex-shrink: 0;
    color: #8aa0c4;
    font-family: Consolas, Monaco, monospace;
    font-size: 10px;
  }
}

.conv-state {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--kf-text-weak);

  &--pending {
    color: var(--kf-red);
  }

  &--open {
    color: var(--kf-primary);
  }
}

.dot-inline {
  flex-shrink: 0;
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

.list-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-top: 1px solid var(--kf-border-light);
  font-size: 12px;
  color: var(--kf-text-weak);
}
</style>
