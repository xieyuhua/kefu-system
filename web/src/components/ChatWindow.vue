<template>
  <section class="chat-window">
    <header v-if="conversation" class="chat-header">
      <div class="header-info">
        <div class="header-title">
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
      <div ref="scrollRef" v-loading="loading" class="chat-body kf-scroll">
        <div v-if="hasMore" class="load-more">
          <el-button size="small" text :loading="loading" @click="emit('load-more')">
            加载更早的消息
          </el-button>
        </div>
        <el-empty v-else-if="!messages.length && !loading" description="暂无消息" :image-size="80" />

        <MessageBubble v-for="m in messages" :key="m.id" :message="m" />
      </div>

      <MessageInput
        :disabled="conversation.status === 'closed'"
        :conversation-id="conversation.id"
        @send="onSend"
        @pick-order="emit('pick-order')"
        @pick-product="emit('pick-product')"
      />
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { User } from '@element-plus/icons-vue'
import type { Conversation, Message, MessageType } from '@/types'
import { channelColor, channelLabel, statusLabel } from '@/utils/format'
import MessageBubble from './MessageBubble.vue'
import MessageInput from './MessageInput.vue'

const props = defineProps<{
  conversation: Conversation | null
  messages: Message[]
  loading: boolean
  hasMore: boolean
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
}>()

const scrollRef = ref<HTMLElement | null>(null)

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
  () => scrollToBottom(),
)

watch(
  () => props.conversation?.id,
  () => scrollToBottom(),
)

function onSend(payload: { type: MessageType; content: string }): void {
  emit('send', payload)
  scrollToBottom()
}

const conversation = computed(() => props.conversation)
const messages = computed(() => props.messages)
const loading = computed(() => props.loading)
const hasMore = computed(() => props.hasMore)
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
