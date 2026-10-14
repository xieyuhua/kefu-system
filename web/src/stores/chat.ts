import { computed, reactive, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { listChannels } from '@/api/channels'
import * as convApi from '@/api/conversations'
import * as msgApi from '@/api/messages'
import dayjs from 'dayjs'
import type { Channel } from '@/types'
import type {
  Conversation,
  ConversationStats,
  ConversationStatus,
  Message,
  MessageType,
  SendMessagePayload,
} from '@/types'

export type StatusFilter = ConversationStatus | 'all'

/** 会话列表筛选条件 */
export interface ChatFilters {
  channelId: number
  status: StatusFilter
  scope: 'mine' | 'all'
  keyword: string
}

function toFilters(): ChatFilters {
  return reactive<ChatFilters>({
    channelId: 0,
    status: 'open',
    scope: 'mine',
    keyword: '',
  })
}

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<Conversation[]>([])
  const currentId = ref<number>(0)
  const messages = ref<Message[]>([])
  const channels = ref<Channel[]>([])
  const stats = ref<ConversationStats>({ pending: 0, open: 0, closed: 0, unread: 0 })
  const filters = toFilters()
  const listLoading = ref(false)
  const messageLoading = ref(false)
  const sending = ref(false)
  const total = ref(0)
  const messagesTotal = ref(0)

  const current = computed<Conversation | null>(
    () => conversations.value.find((c) => c.id === currentId.value) || null,
  )
  const pendingCount = computed(() => stats.value.pending)
  const unreadTotal = computed(() => stats.value.unread)

  function upsertConversation(conversation: Conversation): void {
    const idx = conversations.value.findIndex((c) => c.id === conversation.id)
    if (idx >= 0) {
      conversations.value[idx] = { ...conversations.value[idx], ...conversation }
    } else {
      conversations.value.unshift(conversation)
    }
  }

  function patchConversation(id: number, patch: Partial<Conversation>): void {
    const idx = conversations.value.findIndex((c) => c.id === id)
    if (idx >= 0) {
      conversations.value[idx] = { ...conversations.value[idx], ...patch }
    }
  }

  function removeConversation(id: number): void {
    conversations.value = conversations.value.filter((c) => c.id !== id)
    if (currentId.value === id) {
      currentId.value = 0
      messages.value = []
    }
  }

  async function loadChannels(): Promise<void> {
    try {
      channels.value = await listChannels()
    } catch {
      channels.value = []
    }
  }

  async function loadStats(): Promise<void> {
    try {
      stats.value = await convApi.getConversationStats()
    } catch {
      // 保持上一次的统计
    }
  }

  async function loadConversations(): Promise<void> {
    listLoading.value = true
    try {
      const res = await convApi.listConversations({
        channel_id: filters.channelId,
        status: filters.status,
        scope: filters.scope,
        keyword: filters.keyword || undefined,
        page: 1,
        page_size: 100,
      })
      conversations.value = res.list || []
      total.value = res.total || 0
    } catch {
      conversations.value = []
      total.value = 0
    } finally {
      listLoading.value = false
    }
  }

  async function loadMessages(conversationId: number): Promise<void> {
    messageLoading.value = true
    try {
      const list = await msgApi.listMessages(conversationId, { limit: 50 })
      const arr = Array.isArray(list) ? list : []
      // 与 WebSocket 可能已推入的消息去重后按 id 升序合并，避免重复气泡
      messages.value = dedupeMessages(arr)
      messagesTotal.value = messages.value.length
    } catch {
      messages.value = []
      messagesTotal.value = 0
    } finally {
      messageLoading.value = false
    }
  }

  async function selectConversation(conversation: Conversation): Promise<void> {
    currentId.value = conversation.id
    messages.value = []
    await loadMessages(conversation.id)
    await markRead(conversation.id)
  }

  async function markRead(conversationId: number): Promise<void> {
    try {
      await convApi.readConversation(conversationId)
      patchConversation(conversationId, { unread: 0 })
      refreshStats()
    } catch {
      // 已读失败不影响主流程
    }
  }

  async function claim(conversationId: number): Promise<void> {
    const updated = await convApi.claimConversation(conversationId)
    upsertConversation(updated)
  }

  async function transfer(conversationId: number, agentId: number): Promise<void> {
    const updated = await convApi.transferConversation(conversationId, agentId)
    upsertConversation(updated)
  }

  async function close(conversationId: number): Promise<void> {
    const updated = await convApi.closeConversation(conversationId)
    upsertConversation(updated)
  }

  /**
   * 按 id 插入或更新消息；同 id 已存在则原地替换（不追加）。
   * 这是避免「一条消息显示两次」的唯一入口。
   */
  function upsertMessage(message: Message): void {
    const idx = messages.value.findIndex((m) => m.id === message.id)
    if (idx >= 0) {
      messages.value[idx] = message
      return
    }
    messages.value.push(message)
    messages.value.sort((a, b) => a.id - b.id)
  }

  /**
   * 乐观消息回填：用服务端返回的真实消息替换临时气泡；
   * 若 WebSocket 已把真实消息推入，则先移除对应临时气泡再替换，避免重复。
   */
  function replaceOptimistic(tempId: number, created: Message): void {
    // 1) 移除临时气泡
    const tempIdx = messages.value.findIndex((m) => m.id === tempId)
    if (tempIdx >= 0) messages.value.splice(tempIdx, 1)
    // 2) 真实消息若已在（WS 推来的），替换之；否则插入
    upsertMessage(created)
  }

  /** 乐观上屏：先插入本地临时消息，失败则回滚 */
  async function send(payload: SendMessagePayload): Promise<Message | null> {
    const conversationId = currentId.value
    if (!conversationId) return null
    const tempId = -Date.now()
    const optimistic: Message = {
      id: tempId,
      conversation_id: conversationId,
      type: payload.type,
      direction: 'out',
      content: payload.content,
      sender: null,
      created_at: '',
      pending: true,
    }
    messages.value.push(optimistic)
    sending.value = true
    try {
      const created = await msgApi.sendMessage(conversationId, payload)
      if (created && typeof created.id === 'number') {
        replaceOptimistic(tempId, created)
      } else {
        const idx = messages.value.findIndex((m) => m.id === tempId)
        if (idx >= 0) messages.value[idx] = { ...optimistic, pending: false }
      }
      patchConversation(conversationId, {
        last_message: previewOf(payload),
        last_message_at: dayjs().format('YYYY-MM-DD HH:mm:ss'),
      })
      return created || null
    } catch (err) {
      const idx = messages.value.findIndex((m) => m.id === tempId)
      if (idx >= 0) messages.value.splice(idx, 1)
      throw err
    } finally {
      sending.value = false
    }
  }

  /**
   * 按 id 去重并按 id 升序排序。
   * 场景：乐观上屏用临时负 ID，HTTP 返回与 WebSocket 回推会先后到达同一消息，
   * 必须保证最终只渲染一条。
   */
  function dedupeMessages(list: Message[]): Message[] {
    const byId = new Map<number, Message>()
    for (const m of list) {
      if (!byId.has(m.id)) byId.set(m.id, m)
    }
    return Array.from(byId.values()).sort((a, b) => a.id - b.id)
  }

  /** WebSocket 收到新消息 */
  function onMessage(message: Message): void {
    if (message.conversation_id === currentId.value) {
      upsertMessage(message)
      void markRead(message.conversation_id)
      return
    }
    const target = conversations.value.find((c) => c.id === message.conversation_id)
    if (target) {
      target.unread = (target.unread || 0) + 1
      target.last_message = previewOf({ type: message.type, content: message.content })
    } else {
      void loadConversations()
    }
    refreshStats()
  }

  /** WebSocket 收到会话新建/更新 */
  function onConversation(conversation: Conversation): void {
    upsertConversation(conversation)
    refreshStats()
  }

  function refreshStats(): void {
    void loadStats()
  }

  function setFilter(patch: Partial<ChatFilters>): void {
    Object.assign(filters, patch)
  }

  watch(
    () => [filters.channelId, filters.status, filters.scope] as const,
    () => {
      void loadConversations()
    },
  )

  return {
    conversations,
    currentId,
    current,
    messages,
    messagesTotal,
    channels,
    stats,
    filters,
    total,
    listLoading,
    messageLoading,
    sending,
    pendingCount,
    unreadTotal,
    loadChannels,
    loadStats,
    loadConversations,
    loadMessages,
    selectConversation,
    markRead,
    claim,
    transfer,
    close,
    send,
    onMessage,
    onConversation,
    setFilter,
    removeConversation,
    patchConversation,
    upsertConversation,
  }
})

/** 列表里「最后一条消息」的展示文本 */
export function previewOf(payload: Pick<SendMessagePayload, 'type' | 'content'>): string {
  const map: Record<MessageType, string> = {
    text: '',
    image: '[图片]',
    order: '[订单]',
    product: '[商品]',
    system: '',
  }
  if (payload.type === 'text') return payload.content
  return map[payload.type] || '[消息]'
}
