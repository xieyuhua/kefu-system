<template>
  <section class="chat-window">
    <header v-if="conversation" class="chat-header">
      <div class="header-info">
        <div class="header-title">
          <el-tooltip :content="onlineHint(conversation.user?.online, conversation.user?.last_seen_at)" placement="top">
            <span class="online-dot" :style="{ background: onlineColor(conversation.user?.online) }" />
          </el-tooltip>
          <span class="title-name kf-ellipsis">{{
            conversation.user?.nickname || '匿名客户'
          }}</span>
          <el-tag
            size="small"
            effect="light"
            class="header-tag header-tag--channel"
            :color="channelColor(conversation.channel?.type)"
          >
            {{ conversation.channel?.name || channelLabel(conversation.channel?.type) }}
          </el-tag>
          <el-tag
            size="small"
            class="header-tag header-tag--status"
            :type="statusTagType(conversation.status)"
            effect="plain"
          >
            {{ statusLabel(conversation.status) }}
          </el-tag>
        </div>
        <div class="header-sub">
          <span v-if="conversation.agent">接待人：{{ conversation.agent.name }}</span>
          <span v-else>未分配</span>
          <span class="dot-sep">·</span>
          <span>会话 {{ conversation.id }}</span>
        </div>
      </div>

      <div class="header-actions">
        <el-button
          v-if="conversation.status === 'pending'"
          type="primary"
          size="small"
          @click="emit('claim', conversation)"
        >
          {{ compactHeader ? '接入' : '接入会话' }}
        </el-button>
        <el-button
          size="small"
          :disabled="conversation.status === 'closed'"
          :icon="Switch"
          @click="emit('transfer', conversation)"
        >
          <span v-if="!compactHeader">转接</span>
        </el-button>
        <el-button
          size="small"
          :disabled="conversation.status === 'closed'"
          @click="emit('close', conversation)"
        >
          {{ compactHeader ? '结束' : '结束会话' }}
        </el-button>
        <el-button
          size="small"
          :icon="User"
          :title="panelOpen ? '收起资料' : '客户资料'"
          @click="emit('toggle-panel')"
        >
          <span v-if="!compactHeader">{{ panelOpen ? '收起资料' : '客户资料' }}</span>
        </el-button>
      </div>
    </header>

    <div v-if="!conversation" class="kf-empty chat-empty">
      <span class="empty-icon">💬</span>
      <span>选择左侧会话开始接待</span>
    </div>

    <template v-else>
      <div ref="scrollRef" v-loading="loading" class="chat-body kf-scroll" @scroll="onScroll">
        <div class="load-more">
          <span v-if="loadingMore" class="load-more__tip">加载中…</span>
          <span v-else-if="!hasMore" class="load-more__tip">没有更多消息了</span>
        </div>
        <el-empty v-if="!messages.length && !loading" description="暂无消息" :image-size="80" />

        <MessageBubble
          v-for="m in messages"
          :key="m.id"
          :message="m"
          :flash-key="flashId === m.id ? m.id : 0"
          :wait-ms="waitMessageId === m.id ? waitMs : 0"
          :compact="isMobile"
        />
      </div>

      <MessageInput
        :disabled="conversation.status === 'closed'"
        :conversation-id="conversation.id"
        :search-active="searchOpen"
        :sending="props.sending"
        :compact="compactHeader"
        @send="onSend"
        @pick-order="emit('pick-order')"
        @pick-product="emit('pick-product')"
        @toggle-search="toggleSearch"
      />

      <!-- 搜索浮层（微信式）：点「搜索」按钮弹出，输入即搜，定位后保留结果可继续点 -->
      <div v-if="searchOpen" class="chat-search">
        <div class="chat-search__bar">
          <el-input
            ref="searchInputRef"
            v-model="searchKeyword"
            size="small"
            placeholder="搜索聊天记录"
            clearable
            :prefix-icon="Search"
            @input="onSearchInput"
            @keyup.enter="onSearch"
            @clear="onClearSearch"
          />
          <span v-if="searchedKeyword" class="chat-search__count">
            <template v-if="searching">搜索中…</template>
            <template v-else-if="searchList.length">
              {{ currentIndex + 1 }}/{{ searchList.length }}
            </template>
            <template v-else>无结果</template>
          </span>
          <!-- 上一个 / 下一个 -->
          <template v-if="searchList.length > 1">
            <el-button
              size="small"
              text
              :icon="ArrowUp"
              :disabled="currentIndex <= 0"
              title="上一个"
              @click="stepResult(-1)"
            />
            <el-button
              size="small"
              text
              :icon="ArrowDown"
              :disabled="currentIndex >= searchList.length - 1"
              title="下一个"
              @click="stepResult(1)"
            />
          </template>
          <el-button size="small" text @click="closeSearch">关闭</el-button>
        </div>

        <!-- 结果列表：点击定位并高亮，面板保持打开可连续查看 -->
        <div v-if="searchedKeyword" class="chat-search__panel kf-scroll">
          <div v-if="searching" class="chat-search__tip">搜索中…</div>
          <div v-else-if="!searchList.length" class="chat-search__tip">
            未找到与「{{ searchedKeyword }}」相关的消息
          </div>
          <button
            v-for="(r, i) in searchList"
            :key="r.id"
            type="button"
            class="chat-search__item"
            :class="{ 'is-current': i === currentIndex }"
            @click="gotoResult(i)"
          >
            <span class="chat-search__time">{{ formatMessageTime(r.created_at) }}</span>
            <span class="chat-search__text">{{ snippet(r.content) }}</span>
          </button>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ArrowDown, ArrowUp, Search, Switch, User } from '@element-plus/icons-vue'
import type { Conversation, Message, MessageType } from '@/shared/types'
import {
  channelColor,
  channelLabel,
  formatMessageTime,
  onlineColor,
  onlineHint,
  statusLabel,
} from '@/shared/utils/format'
import MessageBubble from './MessageBubble.vue'
import MessageInput from './MessageInput.vue'
import { useReplyWait } from '@/chat/composables/useReplyWait'
import { useIsMobile } from '@/shared/composables/useIsMobile'

const props = defineProps<{
  conversation: Conversation | null
  messages: Message[]
  loading: boolean
  /** 是否还有更早的历史消息 */
  hasMore: boolean
  /** 正在加载更早的消息 */
  loadingMore?: boolean
  /** 会话内搜索结果 */
  searchResults?: Message[]
  searching?: boolean
  /** 当前会话是否有消息正在发送（转发给输入框禁用发送按钮） */
  sending?: boolean
  panelOpen: boolean
  /** 移动端：客户资料以抽屉打开，此处仅显示图标按钮 */
  compactHeader?: boolean
  /** 移动端：是否显示返回按钮（由父级控制顶栏，这里仅作占位） */
  showBack?: boolean
}>()

const emit = defineEmits<{
  send: [payload: { type: MessageType; content: string }]
  claim: [conversation: Conversation]
  transfer: [conversation: Conversation]
  close: [conversation: Conversation]
  'toggle-panel': []
  'pick-order': []
  'pick-product': []
  'load-more': []
  search: [keyword: string]
  'clear-search': []
}>()

const scrollRef = ref<HTMLElement | null>(null)
const searchKeyword = ref('')
/** 已发起搜索的关键词（用于区分「未搜索」与「搜了但无结果」） */
const searchedKeyword = ref('')
/** 搜索浮层是否展开（由工具栏「搜索」按钮控制） */
const searchOpen = ref(false)
/** 当前定位到第几条结果（0 起），用于「上一个/下一个」与高亮 */
const currentIndex = ref(0)
/** 当前需要闪烁高亮的消息 id */
const flashId = ref(0)
/** 抑制 watch 里的自动滚底（向上翻页/定位时用） */
let stickToBottom = true

function statusTagType(status: Conversation['status']): 'success' | 'warning' | 'info' {
  if (status === 'open') return 'success'
  if (status === 'pending') return 'warning'
  return 'info'
}

function scrollToBottom(): void {
  nextTick(() => {
    const el = scrollRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

watch(
  () => props.messages.length,
  () => {
    if (stickToBottom) scrollToBottom()
  },
)

watch(
  () => props.conversation?.id,
  () => {
    stickToBottom = true
    searchKeyword.value = ''
    searchedKeyword.value = ''
    searchOpen.value = false
    currentIndex.value = 0
    flashId.value = 0
    scrollToBottom()
  },
)

function onSend(payload: { type: MessageType; content: string }): void {
  emit('send', payload)
  stickToBottom = true
  scrollToBottom()
}

/** 滚动到顶部时加载更早的消息，并补偿 scrollHeight 增量保持视口不跳动 */
async function onScroll(): Promise<void> {
  const el = scrollRef.value
  if (!el) return
  const nearTop = el.scrollTop <= 24
  // 距底部 60px 以内视为「贴底」，继续跟随新消息
  stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= 60
  if (!nearTop || props.loadingMore || !props.hasMore) return

  const prevHeight = el.scrollHeight
  const prevTop = el.scrollTop
  emit('load-more')
  await nextTick()
  // 等待父组件把新消息推入 store（store 内 loadingMore 状态变化后 DOM 才会更新）
  await new Promise((resolve) => {
    let waited = 0
    const timer = window.setInterval(() => {
      waited += 60
      if (el.scrollHeight !== prevHeight || waited > 1200) {
        window.clearInterval(timer)
        resolve(undefined)
      }
    }, 60)
  })
  // 把新增高度补回滚动位置，视觉上停留在原处
  el.scrollTop = prevTop + (el.scrollHeight - prevHeight)
  stickToBottom = false
}

/** 点击工具栏「搜索」按钮：展开/收起搜索面板（微信式） */
function toggleSearch(): void {
  searchOpen.value = !searchOpen.value
  if (!searchOpen.value) closeSearch()
}

/** 收起搜索浮层并清空结果 */
function closeSearch(): void {
  searchOpen.value = false
  searchKeyword.value = ''
  searchedKeyword.value = ''
  currentIndex.value = 0
  flashId.value = 0
  emit('clear-search')
}

/** 输入即搜（防抖 300ms，微信式手感） */
let searchTimer: number | null = null
function onSearchInput(): void {
  if (searchTimer !== null) window.clearTimeout(searchTimer)
  const kw = searchKeyword.value.trim()
  if (!kw) {
    searchedKeyword.value = ''
    currentIndex.value = 0
    emit('clear-search')
    return
  }
  searchTimer = window.setTimeout(() => {
    doSearch(kw)
  }, 300)
}

function onSearch(): void {
  if (searchTimer !== null) {
    window.clearTimeout(searchTimer)
    searchTimer = null
  }
  const kw = searchKeyword.value.trim()
  if (!kw) return
  doSearch(kw)
}

function doSearch(kw: string): void {
  searchedKeyword.value = kw
  currentIndex.value = 0
  emit('search', kw)
}

function onClearSearch(): void {
  if (searchTimer !== null) {
    window.clearTimeout(searchTimer)
    searchTimer = null
  }
  searchedKeyword.value = ''
  currentIndex.value = 0
  flashId.value = 0
  emit('clear-search')
}

/**
 * 跳转到第 index 条搜索结果。
 * 若该消息尚未加载到当前视图，会自动上翻分页直到找到为止（最多 5 轮），
 * 避免出现「点了没反应」。
 */
async function gotoResult(index: number): Promise<void> {
  const target = searchList.value[index]
  if (!target) return
  currentIndex.value = index
  await locate(target.id)
}

/** 上一个 / 下一个结果 */
async function stepResult(delta: number): Promise<void> {
  const total = searchList.value.length
  if (!total) return
  const next = Math.min(Math.max(currentIndex.value + delta, 0), total - 1)
  if (next === currentIndex.value) return
  await gotoResult(next)
}

/**
 * 滚动定位并高亮该条消息。
 * 消息不在当前已加载范围内时，先加载更早的消息再定位（最多 5 轮）。
 */
async function locate(id: number): Promise<void> {
  const el = scrollRef.value
  if (!el) return

  for (let attempt = 0; attempt < 5; attempt++) {
    await nextTick()
    const node = el.querySelector<HTMLElement>(`[data-msg-id="${id}"]`)
    if (node) {
      stickToBottom = false
      node.scrollIntoView({ block: 'center', behavior: 'smooth' })
      // 高亮闪烁
      flashId.value = 0
      await nextTick()
      flashId.value = id
      window.setTimeout(() => {
        if (flashId.value === id) flashId.value = 0
      }, 2000)
      return
    }
    // 还没加载出来 → 向上翻一页再试
    if (props.loadingMore || !props.hasMore) break
    stickToBottom = false
    emit('load-more')
    // 等待父组件把新消息推入 store（DOM 更新后继续下一轮查找）
    await new Promise<void>((resolve) => {
      let waited = 0
      const timer = window.setInterval(() => {
        waited += 80
        if (!props.loadingMore || waited > 2000) {
          window.clearInterval(timer)
          resolve()
        }
      }, 80)
    })
  }
}

/** 搜索结果摘要 */
function snippet(content: string): string {
  const text = content || ''
  return text.length > 60 ? `${text.slice(0, 60)}…` : text
}

const conversation = computed(() => props.conversation)
const messages = computed(() => props.messages)
const loading = computed(() => props.loading)
const hasMore = computed(() => props.hasMore)
const loadingMore = computed(() => props.loadingMore === true)
/** 搜索结果（store 提供，可为空） */
const searchList = computed<Message[]>(() => props.searchResults || [])

/** 是否移动端窄屏：复用全局断点（matchMedia），不在这里新增 resize 监听 */
const { isMobile } = useIsMobile()

/**
 * 未回复计时：全聊天区共用一个 setInterval（见 useReplyWait）。
 * 会话切换（conversationId 变化）、客服回复、新消息到达都会让计时起点变化，
 * 从而自动停表或按新会话重新起表；组件卸载时 clearInterval。
 */
const { pendingId: waitMessageId, waitMs } = useReplyWait({
  messages,
  conversationId: computed(() => props.conversation?.id),
  active: () => props.conversation?.status !== 'closed',
})
</script>

<style scoped lang="scss">
.chat-window {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  background: var(--kf-bg);
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  height: var(--kf-header-height);
  padding: 0 14px;
  background: #fff;
  border-bottom: 1px solid var(--kf-border);
}

.header-info {
  min-width: 0;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;

  :deep(.el-tag) {
    color: #fff;
    border: none;
  }
}

.title-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--kf-text);
}

/* 标题栏在线状态圆点 */
.online-dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

/* ---------------- 会话内消息搜索 ---------------- */
.chat-search {
  position: relative;
  flex-shrink: 0;
  padding: 6px 16px;
  background: #fff;
  border-bottom: 1px solid var(--kf-border-light);

  &__panel {
    position: absolute;
    z-index: 20;
    top: 100%;
    left: 16px;
    right: 16px;
    max-height: 320px;
    overflow-y: auto;
    background: #fff;
    border: 1px solid var(--kf-border);
    border-radius: var(--kf-radius);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.1);
  }

  &__tip {
    padding: 14px 12px;
    text-align: center;
    font-size: 12px;
    color: var(--kf-text-weak);
  }

  &__item {
    display: block;
    width: 100%;
    padding: 8px 12px;
    border: none;
    border-bottom: 1px solid var(--kf-border-light);
    background: transparent;
    text-align: left;
    cursor: pointer;

    &:hover {
      background: var(--kf-bg-hover);
    }
  }

  &__time {
    display: block;
    font-size: 11px;
    color: var(--kf-text-weak);
  }

  &__text {
    display: block;
    margin-top: 2px;
    font-size: 13px;
    color: var(--kf-text);
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
}

.header-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.dot-sep {
  margin: 0 4px;
}

.header-actions {
  display: flex;
  flex-shrink: 0;
  gap: 8px;
}

.chat-body {
  flex: 1;
  padding: 12px 16px;
}

.chat-empty {
  flex: 1;
}

.empty-icon {
  font-size: 40px;
  opacity: 0.5;
}

.load-more {
  display: flex;
  justify-content: center;
  margin-bottom: 8px;

  &__tip {
    font-size: 12px;
    color: var(--kf-text-weak);
  }
}

/* ---------------- 移动端适配 ---------------- */
@media (max-width: 768px) {
  .chat-header {
    height: 48px;
    padding: 0 10px;
    gap: 6px;
  }

  .header-info {
    flex: 1;
  }

  .title-name {
    max-width: 26vw;
    font-size: 14px;
  }

  /* 移动端隐藏渠道 tag（会话列表已标注来源），只保留状态 tag，给按钮腾空间 */
  .header-tag--channel {
    display: none;
  }

  .header-tag {
    flex-shrink: 0;
  }

  /* 副标题在移动端隐藏，节省纵向空间 */
  .header-sub {
    display: none;
  }

  .header-actions {
    gap: 4px;

    :deep(.el-button) {
      padding: 5px 7px;
    }
  }

  .chat-body {
    padding: 10px 12px;
  }
}
</style>
