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
          <el-tag size="small" effect="light" :color="channelColor(conversation.channel?.type)">
            {{ conversation.channel?.name || channelLabel(conversation.channel?.type) }}
          </el-tag>
          <el-tag size="small" :type="statusTagType(conversation.status)" effect="plain">
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
          @click="emit('transfer', conversation)"
        >
          转接
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
        />
      </div>

      <MessageInput
        :disabled="conversation.status === 'closed'"
        :conversation-id="conversation.id"
        :search-active="searchOpen"
        :sending="props.sending"
        @send="onSend"
        @pick-order="emit('pick-order')"
        @pick-product="emit('pick-product')"
        @toggle-search="toggleSearch"
      />

      <!-- 会话内消息搜索面板：点击工具栏「搜索」按钮展开（微信式），选中结果后自动收起 -->
      <div v-if="searchOpen" class="chat-search">
        <div class="chat-search__head">
          <el-input
            v-model="searchKeyword"
            size="small"
            placeholder="搜索该会话的历史消息"
            clearable
            :prefix-icon="Search"
            @keyup.enter="onSearch"
            @clear="onClearSearch"
          />
          <el-button size="small" text @click="closeSearch">收起</el-button>
        </div>

        <div v-if="searchKeyword.trim()" class="chat-search__panel kf-scroll">
          <div v-if="searching" class="chat-search__tip">搜索中…</div>
          <div v-else-if="!searchList.length" class="chat-search__tip">未找到匹配的消息</div>
          <button
            v-for="r in searchList"
            :key="r.id"
            type="button"
            class="chat-search__item"
            @click="locate(r.id)"
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
import { Search, User } from '@element-plus/icons-vue'
import type { Conversation, Message, MessageType } from '@/types'
import {
  channelColor,
  channelLabel,
  formatMessageTime,
  onlineColor,
  onlineHint,
  statusLabel,
} from '@/utils/format'
import MessageBubble from './MessageBubble.vue'
import MessageInput from './MessageInput.vue'

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
/** 搜索面板是否展开（由工具栏「搜索」按钮控制） */
const searchOpen = ref(false)
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
    searchOpen.value = false
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

/** 收起搜索面板并清空结果 */
function closeSearch(): void {
  searchOpen.value = false
  searchKeyword.value = ''
  flashId.value = 0
  emit('clear-search')
}

function onSearch(): void {
  emit('search', searchKeyword.value.trim())
}

function onClearSearch(): void {
  flashId.value = 0
  emit('clear-search')
}

/**
 * 点击搜索结果：滚动定位并高亮该条消息，然后**自动收起搜索结果**（避免遮挡聊天）。
 */
function locate(id: number): void {
  nextTick(() => {
    const el = scrollRef.value
    if (!el) return
    const target = el.querySelector<HTMLElement>(`[data-msg-id="${id}"]`)
    if (!target) {
      // 该消息尚未加载：先翻页加载更早消息
      stickToBottom = false
      emit('load-more')
      return
    }
    stickToBottom = false
    target.scrollIntoView({ block: 'center', behavior: 'smooth' })
    flashId.value = 0
    nextTick(() => {
      flashId.value = id
    })
    // 定位后收起搜索面板，把聊天区让给客服
    searchOpen.value = false
    emit('clear-search')
  })
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
    gap: 8px;
  }

  .title-name {
    max-width: 40vw;
    font-size: 14px;
  }

  /* 副标题在移动端隐藏，节省纵向空间 */
  .header-sub {
    display: none;
  }

  .header-actions {
    gap: 4px;

    :deep(.el-button) {
      padding: 5px 8px;
    }
  }

  .chat-body {
    padding: 10px 12px;
  }
}
</style>
