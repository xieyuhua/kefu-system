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
      <!-- 管理员可切换「全部会话 / 我接待的」 -->
      <el-radio-group
        v-if="showScopeSwitch"
        :model-value="scope"
        size="small"
        class="scope-switch"
        @change="onScope"
      >
        <el-radio-button value="all">全部</el-radio-button>
        <el-radio-button value="mine">我接待的</el-radio-button>
      </el-radio-group>
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
      <div v-for="c in conversations" :key="c.id" class="conv-swipe">
        <!-- 移动端左滑露出的删除按钮（桌面端用行内的删除图标） -->
        <div class="conv-swipe__actions">
          <button type="button" class="conv-swipe__del" @click.stop="onSwipeDelete(c)">
            删除
          </button>
        </div>

        <div
          class="conv-item"
          :class="{ active: c.id === currentId }"
          :style="swipeStyle(c)"
          @click="onSelect(c)"
          @touchstart="onTouchStart(c, $event)"
          @touchmove="onTouchMove(c, $event)"
          @touchend="onTouchEnd"
          @touchcancel="onTouchEnd"
        >
        <div class="conv-avatar">
          <img v-if="c.user?.avatar" :src="c.user.avatar" alt="" />
          <span v-else>{{ (c.user?.nickname || '?').slice(0, 1) }}</span>
        </div>

        <div class="conv-main">
          <div class="conv-line">
            <span class="conv-name-box">
              <el-tooltip :content="onlineHint(c.user?.online, c.user?.last_seen_at)" placement="top">
                <span
                  class="online-dot"
                  :style="{ background: onlineColor(c.user?.online) }"
                ></span>
              </el-tooltip>
              <span class="conv-name kf-ellipsis" :class="{ unread: unreadOf(c) > 0 }">{{
                c.user?.nickname || '匿名客户'
              }}</span>
            </span>
            <span class="conv-right">
              <span class="conv-time">{{ formatListTime(c.last_message_at || c.created_at) }}</span>
              <!-- 未回复时长：与未读角标同一行，挂在未读数左边（橙色小标签） -->
              <span
                v-if="waitVisible(c)"
                class="conv-wait"
                :data-unreplied-ts="unrepliedStart(c)"
                :title="`客户已等待 ${waitText(c)}`"
              >未回复 {{ waitText(c) }}</span>
              <!-- 未读数：像微信一样挂在右侧（超过 99 显示 99+） -->
              <span v-if="unreadOf(c) > 0" class="unread-badge">{{ unreadText(c) }}</span>
            </span>
          </div>
          <div class="conv-line">
            <span class="conv-msg kf-ellipsis">{{ preview(c) }}</span>
          </div>
          <div class="conv-line conv-line--meta">
            <!-- 来源渠道标注：所有渠道会话合并展示，靠这里区分 -->
            <span class="conv-source" :title="c.channel?.name">
              <span class="conv-source__icon">{{ channelIcon(c.channel?.type) }}</span>
              <span class="conv-source__name kf-ellipsis">{{
                c.channel?.name || channelLabel(c.channel?.type)
              }}</span>
            </span>
            <span v-if="c.status === 'pending'" class="conv-state conv-state--pending">未接待</span>
            <span v-else-if="c.status === 'open'" class="conv-state conv-state--open">进行中</span>
            <span v-else class="conv-state">已结束</span>
            <el-tooltip content="删除会话" placement="top">
              <el-button
                class="conv-del"
                text
                size="small"
                :icon="Delete"
                @click.stop="emit('archive', c)"
              />
            </el-tooltip>
          </div>
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Delete, Refresh, Search } from '@element-plus/icons-vue'
import type { Conversation } from '@/shared/types'
import {
  channelIcon,
  channelLabel,
  formatListTime,
  formatWaitDuration,
  onlineColor,
  onlineHint,
} from '@/shared/utils/format'
import type { StatusFilter } from '@/chat/stores/chat'

const props = defineProps<{
  conversations: Conversation[]
  currentId: number
  loading: boolean
  total: number
  stats: { pending: number; open: number; closed: number; unread: number }
  status: StatusFilter
  /** 管理员才显示「全部 / 我接待的」切换 */
  isAdmin?: boolean
  scope?: 'all' | 'mine'
}>()

const emit = defineEmits<{
  select: [conversation: Conversation]
  'update:status': [status: StatusFilter]
  'update:keyword': [keyword: string]
  'update:scope': [scope: 'all' | 'mine']
  refresh: []
  archive: [conversation: Conversation]
}>()

const keyword = ref('')

watch(keyword, (val) => emit('update:keyword', val))

/* ---------------- 移动端左滑删除 ---------------- */
/** 左滑露出的删除按钮宽度（px） */
const SWIPE_MAX = 76
/** 当前滑开的会话 id（0 = 全部收起） */
const swipeId = ref(0)
/** 当前滑动位移（<= 0） */
const swipeDx = ref(0)
let touchStartX = 0
let touchStartY = 0
let touchBaseDx = 0
let swipeLocked: '' | 'h' | 'v' = ''
let swipeMoved = false

function closeSwipe(): void {
  swipeId.value = 0
  swipeDx.value = 0
  swipeLocked = ''
  swipeMoved = false
}

/** 行内位移样式：只有当前滑开的行才发生位移，其余保持 0 */
function swipeStyle(c: Conversation): Record<string, string> {
  if (swipeId.value !== c.id || swipeDx.value === 0) return {}
  return {
    transform: `translateX(${swipeDx.value}px)`,
    transition: swipeMoved ? 'none' : 'transform 0.18s ease',
  }
}

function onTouchStart(c: Conversation, e: TouchEvent): void {
  if (e.touches.length !== 1) return
  const t = e.touches[0]
  touchStartX = t.clientX
  touchStartY = t.clientY
  touchBaseDx = swipeId.value === c.id ? swipeDx.value : 0
  swipeLocked = ''
  swipeMoved = false
}

function onTouchMove(c: Conversation, e: TouchEvent): void {
  if (e.touches.length !== 1) return
  const t = e.touches[0]
  const dx = t.clientX - touchStartX
  const dy = t.clientY - touchStartY
  // 首次移动先判定方向：横向才接管，纵向交还给列表滚动
  if (swipeLocked === '') {
    if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
    swipeLocked = Math.abs(dx) > Math.abs(dy) ? 'h' : 'v'
  }
  if (swipeLocked !== 'h') return
  // 左滑为负、右滑回弹，统一夹在 [-SWIPE_MAX, 0]
  swipeId.value = c.id
  swipeDx.value = Math.max(-SWIPE_MAX, Math.min(0, touchBaseDx + dx))
  swipeMoved = true
}

function onTouchEnd(): void {
  if (swipeLocked !== 'h' || !swipeMoved) {
    swipeMoved = false
    swipeLocked = ''
    return
  }
  // 超过一半吸附打开，否则回弹收起
  swipeDx.value = swipeDx.value < -SWIPE_MAX / 2 ? -SWIPE_MAX : 0
  if (swipeDx.value === 0) swipeId.value = 0
  swipeMoved = false
  swipeLocked = ''
}

/** 点击行：滑开状态下先收起（不进入会话），否则正常选中 */
function onSelect(c: Conversation): void {
  if (swipeId.value === c.id && swipeDx.value < 0) {
    closeSwipe()
    return
  }
  if (swipeId.value !== 0) closeSwipe()
  emit('select', c)
}

/** 滑动删除：先收起再交给父组件归档（与行内删除图标同一动作） */
function onSwipeDelete(c: Conversation): void {
  closeSwipe()
  emit('archive', c)
}

// 列表数据变化（刷新/切 Tab/删除）时收起滑动状态，避免停在已不存在的行上
watch(
  () => props.conversations,
  () => closeSwipe(),
)

const filters = computed(() => ({ status: props.status }))

/** 仅管理员需要「全部 / 我接待的」切换 */
const showScopeSwitch = computed(() => props.isAdmin === true)
const scope = computed<'all' | 'mine'>(() => props.scope || 'all')

function onScope(val: unknown): void {
  emit('update:scope', val === 'mine' ? 'mine' : 'all')
}

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

function unreadOf(c: Conversation): number {
  return c.unread || 0
}

/** 未读数文案：超过 99 显示 99+ */
function unreadText(c: Conversation): string {
  const n = unreadOf(c)
  return n > 99 ? '99+' : String(n)
}

/* ---------------- 未回复时长（秒级） ---------------- */
/**
 * 当前时间（Unix 毫秒），由下面唯一的一个 setInterval 每秒推进。
 * 它只用于「未回复 N 秒」文案重算，是整张列表共用的时钟——
 * 刻意不按会话各开一个定时器（会话几十上百个时会同时跑几十上百个表）。
 */
const now = ref(Date.now())
let tickTimer: number | null = null

function stopTicker(): void {
  if (tickTimer !== null) {
    window.clearInterval(tickTimer)
    tickTimer = null
  }
}

function startTicker(): void {
  if (tickTimer !== null) return // 已在跑：绝不重复开表
  // 起表即对齐一次，避免刚出现计时时数字滞后接近 1 秒
  now.value = Date.now()
  tickTimer = window.setInterval(() => {
    now.value = Date.now()
  }, 1000)
}

/**
 * 未回复起点（Unix 毫秒）：后端 unreplied_ts 只在「最后一条是客户消息」时 > 0。
 * 旧接口没有该字段 → undefined → 返回 0 → 不显示标签（不报错、不显示 0 秒）。
 */
function unrepliedStart(c: Conversation): number {
  const ts = c.unreplied_ts
  return typeof ts === 'number' && Number.isFinite(ts) && ts > 0 ? ts : 0
}

/** 是否显示「未回复」标签：有起点且会话未结束（已结束的会话不再催回复） */
function waitVisible(c: Conversation): boolean {
  return c.status !== 'closed' && unrepliedStart(c) > 0
}

/** 标签文案：未回复 38 秒 / 未回复 2 分 05 秒 */
function waitText(c: Conversation): string {
  return formatWaitDuration(Math.max(0, now.value - unrepliedStart(c)))
}

/**
 * 是否需要走秒：列表里只要还有一条会话在等回复就保持走表，
 * 全部消失（客服都回了 / 都结束了 / 旧接口无字段）立即停表。
 * 存活定时器因此始终 ≤ 1 个，切换会话、收到 WS 新消息都不会叠加。
 */
const hasWaiting = computed(() => props.conversations.some((c) => waitVisible(c)))

watch(
  hasWaiting,
  (active) => {
    if (active) {
      startTicker()
    } else {
      stopTicker()
    }
  },
  { immediate: true },
)

onBeforeUnmount(stopTicker)

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
    /* 移动端行略紧凑，一屏能多看一两条会话 */
    padding: 9px 10px;
    align-items: flex-start;
  }

  .conv-avatar {
    /* 手机上头像与首行文字基线对齐，避免看起来「悬空」 */
    margin-top: 1px;
    width: 38px;
    height: 38px;
  }

  .conv-name {
    font-size: 15px;
  }

  .conv-time {
    font-size: 11px;
  }

  .conv-msg {
    font-size: 12.5px;
    line-height: 1.45;
  }

  /* 窄屏：时间 / 未回复 / 未读数一行可能放不下，
     允许「未回复」标签换到第二行（不溢出、不挤压未读数），见下方 .conv-wait 的 order */
  .conv-right {
    flex-wrap: wrap;
    justify-content: flex-end;
    row-gap: 2px;
    max-width: 58%;
  }

  .conv-wait {
    order: 3;
  }

  /* 移动端触摸友好：加大点击区域 */
  .tab {
    padding: 8px 0;
  }

  /* 触摸设备无 hover，删除按钮常驻 */
  .conv-del {
    opacity: 1;
  }
}

.list-header {
  padding: 10px 12px;
  border-bottom: 1px solid var(--kf-border-light);
}

/* 管理员「全部 / 我接待的」切换 */
.scope-switch {
  margin-top: 8px;
  display: flex;

  :deep(.el-radio-button__inner) {
    padding: 5px 12px;
    font-size: 12px;
  }
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

/* ---------------- 移动端左滑删除 ---------------- */
.conv-swipe {
  position: relative;
  overflow: hidden;
  /* 背景色与行一致，避免滑动时透出底色 */
  background: var(--kf-bg-card, #fff);
}

.conv-swipe__actions {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  display: flex;
  width: 76px;
}

.conv-swipe__del {
  flex: 1;
  border: none;
  background: #f56c6c;
  color: #fff;
  font-size: 14px;
  cursor: pointer;

  &:active {
    background: #e45656;
  }
}

.conv-item {
  position: relative;
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  cursor: pointer;
  background: var(--kf-bg-card, #fff);
  border-bottom: 1px solid var(--kf-border-light);
  /* 横向手势自己处理，纵向滚动交给浏览器 */
  touch-action: pan-y;
  will-change: transform;

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

/* 昵称 + 在线状态圆点 */
.conv-name-box {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.online-dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

/* 有未读：昵称变蓝 + 加粗（微信式） */
.conv-name.unread {
  color: #2c7be5;
  font-weight: 600;
}

/* 右侧：时间 + 未回复标签 + 未读气泡 */
.conv-right {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

/* 未回复时长：与未读角标同一行，橙色小标签（比未读角标略宽，但不抢视觉） */
.conv-wait {
  flex-shrink: 0;
  height: 18px;
  padding: 0 6px;
  border-radius: 9px;
  background: rgba(255, 143, 31, 0.12);
  color: var(--kf-orange);
  font-size: 11px;
  line-height: 18px;
  font-weight: 500;
  white-space: nowrap;
  /* 等宽数字：每秒跳动时标签宽度不抖，避免整行抖动 */
  font-variant-numeric: tabular-nums;
}

.unread-badge {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--kf-red);
  color: #fff;
  font-size: 11px;
  line-height: 18px;
  text-align: center;
  font-weight: 500;
  /* 窄屏压缩时优先挤昵称，未读数不允许被压扁 */
  flex-shrink: 0;
}

/* 列表项操作：删除（默认隐藏，hover 显示） */
.conv-del {
  margin-left: auto;
  padding: 0 4px;
  height: 20px;
  color: var(--kf-text-weak);
  opacity: 0;
  transition: opacity 0.15s;

  &:hover {
    color: var(--kf-red);
  }
}

.conv-item:hover .conv-del {
  opacity: 1;
}

.conv-time {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--kf-text-weak);
}

.conv-msg {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.5;
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
