import { computed, reactive, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { listChannels } from '@/shared/api/channels'
import * as convApi from '@/shared/api/conversations'
import * as msgApi from '@/shared/api/messages'
import dayjs from 'dayjs'
import type { Channel } from '@/shared/types'
import type {
  Conversation,
  ConversationStats,
  ConversationStatus,
  Message,
  MessageType,
  SendMessagePayload,
} from '@/shared/types'

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
    status: 'all',
    // 管理员默认看全部会话；客服固定为「我接待的」
    scope: 'all',
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
  /**
   * 正在发送的会话 id 集合 —— 防重复提交的关键。
   * sending 是全局布尔，会话 A 发送中时切到会话 B 仍应允许发送，
   * 因此按 conversation_id 分别加锁；同一会话在途时直接忽略后续提交。
   */
  const sendingConvIds = new Set<number>()
  const total = ref(0)
  const messagesTotal = ref(0)
  /** 是否还有更早的历史消息可加载（上滑分页） */
  const hasMoreMessages = ref(false)
  /** 正在加载更早的消息 */
  const loadingMoreMessages = ref(false)
  /** 会话内搜索结果 */
  const searchResults = ref<Message[]>([])
  const searching = ref(false)

  /**
   * 已读幂等台账：会话 id → 已经标记过已读的最后一条消息 id。
   * 用来判断「同一条消息是否标记过」，避免 WS 重推 / 会话重开时重复 POST /conversations/:id/read。
   */
  const lastReadId = new Map<number, number>()
  /**
   * 已读单飞：会话 id → 在途的 read 请求。
   * 同一会话并发调用（WS 推送 + 手动点开会话）只保留一个请求，后来者复用同一个 Promise。
   */
  const readInflight = new Map<number, Promise<boolean>>()
  /**
   * 非当前会话最近一次已计入未读的消息 id。
   * 后端可能把同一条 message.new 重复投递（Redis 广播回声），
   * 用它保证未读只 +1、统计也只刷新一次。
   */
  const lastCountedMessageId = new Map<number, number>()
  /** 统计请求合并窗口（ms）：窗口内多次 refreshStats 只发 1 个 GET /conversations/stats */
  const STATS_DEBOUNCE_MS = 250
  let statsTimer = 0
  /** 是否有 stats 请求在途 */
  let statsInflight = false
  /** 在途期间又来了刷新诉求：结束后最多再补 1 次 */
  let statsDirty = false

  const current = computed<Conversation | null>(
    () => conversations.value.find((c) => c.id === currentId.value) || null,
  )
  const pendingCount = computed(() => stats.value.pending)

  /** 当前会话是否有消息正在发送（仅供输入框禁用按钮） */
  const sendingCurrent = computed(
    () => currentId.value > 0 && sendingConvIds.has(currentId.value),
  )
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
    lastReadId.delete(id)
    lastCountedMessageId.delete(id)
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
        // all = 全部会话（管理员默认）；mine = 只看我接待的
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

  /** 单页加载条数：进入会话只取最后 20 条 */
  const MESSAGE_PAGE_SIZE = 20

  /** 加载当前会话消息（默认只取最后 20 条） */
  async function loadMessages(conversationId: number, limit = MESSAGE_PAGE_SIZE): Promise<void> {
    messageLoading.value = true
    try {
      const list = await msgApi.listMessages(conversationId, { limit })
      const arr = Array.isArray(list) ? list : []
      // 与 WebSocket 可能已推入的消息去重后按 id 升序合并，避免重复气泡
      messages.value = dedupeMessages(arr)
      messagesTotal.value = messages.value.length
      // 拉满即说明还有更早的历史消息
      hasMoreMessages.value = arr.length >= limit
    } catch {
      messages.value = []
      messagesTotal.value = 0
      hasMoreMessages.value = false
    } finally {
      messageLoading.value = false
    }
  }

  /**
   * 向上翻页：拉取 id 小于 beforeId 的更早消息，前插到当前列表头部。
   * 返回本次新增的条数，供调用方做滚动位置补偿。
   */
  async function loadMoreMessages(conversationId: number): Promise<number> {
    if (loadingMoreMessages.value || !hasMoreMessages.value) return 0
    const first = messages.value.find((m) => m.id > 0)
    if (!first) return 0
    loadingMoreMessages.value = true
    try {
      const list = await msgApi.listMessages(conversationId, {
        before_id: first.id,
        limit: MESSAGE_PAGE_SIZE,
      })
      const older = (Array.isArray(list) ? list : []).filter((m) => m.id < first.id)
      if (!older.length) {
        hasMoreMessages.value = false
        return 0
      }
      const existing = new Set(messages.value.map((m) => m.id))
      const fresh = older.filter((m) => !existing.has(m.id))
      messages.value = dedupeMessages([...fresh, ...messages.value])
      messagesTotal.value = messages.value.length
      if (older.length < MESSAGE_PAGE_SIZE) hasMoreMessages.value = false
      return fresh.length
    } catch {
      return 0
    } finally {
      loadingMoreMessages.value = false
    }
  }

  /** 会话内搜索历史消息 */
  async function searchMessages(conversationId: number, keyword: string): Promise<Message[]> {
    const kw = keyword.trim()
    if (!kw) {
      searchResults.value = []
      return []
    }
    searching.value = true
    try {
      const list = await msgApi.searchMessages(conversationId, { keyword: kw, limit: 50 })
      const arr = Array.isArray(list) ? list : []
      searchResults.value = arr.slice().sort((a, b) => a.id - b.id)
      return searchResults.value
    } catch {
      searchResults.value = []
      return []
    } finally {
      searching.value = false
    }
  }

  function clearSearch(): void {
    searchResults.value = []
  }

  async function selectConversation(conversation: Conversation): Promise<void> {
    currentId.value = conversation.id
    messages.value = []
    searchResults.value = []
    await loadMessages(conversation.id)
    const read = await markRead(conversation.id)
    // 真的清掉了服务端未读才需要回校准统计（未读数已在 markRead 里本地扣减），
    // 且走合并后的 refreshStats，和同期的 WS 事件共用一次请求。
    if (read) refreshStats()
  }

  /** 归档（工作台「删除」）：会话从列表移除；若正选中则清空当前会话 */
  async function archiveConversation(conversationId: number): Promise<void> {
    await convApi.archiveConversation(conversationId)
    removeConversation(conversationId)
    clearSearch()
    refreshStats()
  }

  /**
   * 一键清空工作台列表：按当前筛选条件批量归档。
   * 返回实际清空的条数；清空后如果当前会话被一并归档，会取消选中状态。
   */
  async function clearAllConversations(): Promise<number> {
    const f = filters
    const res = await convApi.archiveConversationsBulk({
      channel_id: f.channelId,
      status: f.status,
      scope: f.scope,
      keyword: f.keyword,
    })
    const n = res?.archived ?? 0
    if (n > 0) {
      // 本地同步：清掉列表并重置总数
      conversations.value = []
      total.value = 0
      // 当前选中的会话若已被归档，清空聊天区
      if (currentId.value) {
        currentId.value = 0
        messages.value = []
      }
      clearSearch()
      refreshStats()
    }
    return n
  }

  /** 当前会话已加载的最后一条真实消息 id（用于「已标记到哪条」） */
  function latestMessageIdOf(conversationId: number): number {
    if (conversationId !== currentId.value) return 0
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const m = messages.value[i]
      if (m.id > 0 && !m.pending) return m.id
    }
    return 0
  }

  /**
   * 标记会话已读（状态闸门 + 幂等 + 单飞）。
   *
   * - 状态闸门：只有「已接管（open）」的会话才清未读。待接待 / 已结束的会话点开只是「看」，
   *   不该清未读（保留角标），因此直接返回、一个请求都不发。
   * - 幂等：会话无未读、且没有「比上次标记更新的消息」时直接返回，不发请求；
   *   同一会话 / 同一条消息重复调用不会产生第二次 POST /conversations/:id/read。
   * - 单飞：同一会话的 read 请求在途时，后来的调用复用同一个 Promise，不并发重复发。
   * - 以接口返回值为准：服务端返回 true（真的清了未读）才本地置零；
   *   返回 false（状态已变等）则不动本地未读数。
   * - 不再在内部刷新统计：未读数本地 patch（并同步扣减未读统计），
   *   统计由 onConversation（conversation.updated）与发送成功统一走合并后的 refreshStats。
   *
   * @param conversationId 会话 id
   * @param messageId 触发本次已读的消息 id（WS 新消息时传，用于幂等判断）
   * @returns 服务端是否真的清掉了未读
   */
  function markRead(conversationId: number, messageId = 0): Promise<boolean> {
    const id = Number(conversationId) || 0
    if (!id) return Promise.resolve(false)

    const conv = conversations.value.find((c) => c.id === id)
    // 只有进行中（已接管）的会话才清未读：待接待 / 已结束点开只看不清
    if (!conv || conv.status !== 'open') return Promise.resolve(false)

    const unread = conv.unread ?? 0
    const marked = lastReadId.get(id) ?? 0
    const explicitId = messageId > 0 ? messageId : 0
    const markId = explicitId || latestMessageIdOf(id)

    // 幂等闸门：既没有未读，也没有比上次标记更新的消息 → 不发请求
    if (unread <= 0 && explicitId <= marked) {
      if (markId > marked) lastReadId.set(id, markId)
      return Promise.resolve(false)
    }

    // 单飞：同一会话已有请求在途，复用同一个 Promise（完成后由 finally 清理）
    const inflight = readInflight.get(id)
    if (inflight) return inflight

    const task = (async (): Promise<boolean> => {
      try {
        const cleared = await convApi.readConversation(id)
        // 服务端返回 false（例如状态已不是 open）时不做本地清零
        if (!cleared) return false
        patchConversation(id, { unread: 0 })
        // 本地即时扣减未读统计（服务端会按同一口径减少），不额外发 GET /stats
        if (unread > 0) {
          stats.value = { ...stats.value, unread: Math.max(0, stats.value.unread - unread) }
        }
        if (markId > marked) lastReadId.set(id, markId)
        return true
      } catch {
        // 已读失败不影响主流程；下次打开会话会再试
        return false
      } finally {
        readInflight.delete(id)
      }
    })()
    readInflight.set(id, task)
    return task
  }

  /** 会话状态变化后本地即时挪动统计项（服务端的合并刷新随后会校准成同一口径） */
  function moveStat(from: ConversationStatus, to: ConversationStatus): void {
    if (from === to) return
    const next = { ...stats.value }
    if (from === 'pending') next.pending = Math.max(0, next.pending - 1)
    else if (from === 'open') next.open = Math.max(0, next.open - 1)
    else next.closed = Math.max(0, next.closed - 1)
    if (to === 'pending') next.pending += 1
    else if (to === 'open') next.open += 1
    else next.closed += 1
    stats.value = next
  }

  async function claim(conversationId: number): Promise<void> {
    const prev = conversations.value.find((c) => c.id === conversationId)
    const updated = await convApi.claimConversation(conversationId)
    upsertConversation(updated)
    // 待接待 → 进行中：本地即时反映，统计由合并后的 refreshStats 校准
    if (prev) moveStat(prev.status, updated.status)
    // 接管成功后状态变 open，才允许清未读：这里补一次已读，让待接待期间攒的未读正常归零
    if (updated.status === 'open') await markRead(conversationId)
  }

  async function transfer(conversationId: number, agentId: number): Promise<void> {
    const updated = await convApi.transferConversation(conversationId, agentId)
    upsertConversation(updated)
  }

  async function close(conversationId: number): Promise<void> {
    const prev = conversations.value.find((c) => c.id === conversationId)
    const updated = await convApi.closeConversation(conversationId)
    upsertConversation(updated)
    // 结束会话：本地即时把「进行中/待接待」挪到「已结束」，
    // 不依赖 250ms 的统计合并窗口，避免结束后统计看起来「慢半拍」
    if (prev) moveStat(prev.status, updated.status)
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

    // 同一会话已有请求在途：直接忽略，避免连按回车/重复点击产生多条相同消息
    if (sendingConvIds.has(conversationId)) return null

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
    sendingConvIds.add(conversationId)
    sending.value = sendingConvIds.size > 0
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
        // 客服刚发出回复：会话最后一条消息不再是客户消息 →
        // 列表项上的「未回复 N 秒」立即消失（不必等 conversation.updated 回来）
        unreplied_ts: 0,
        // 既然已经回复，客户的未读也算读过了：本地立即清角标（服务端同样会清零）
        unread: 0,
      })
      // 统计走合并后的 refreshStats（与随后到达的 conversation.updated 共用一次请求）
      refreshStats()
      return created || null
    } catch (err) {
      const idx = messages.value.findIndex((m) => m.id === tempId)
      if (idx >= 0) messages.value.splice(idx, 1)
      throw err
    } finally {
      sendingConvIds.delete(conversationId)
      sending.value = sendingConvIds.size > 0
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

  /**
   * 客服自己发出的消息（direction=out 或 sender.role=agent/admin）。
   * 自己发消息不需要标记已读——否则每发一条回复都会多一个 POST /read。
   */
  function isOwnMessage(message: Message): boolean {
    if (message.direction === 'out') return true
    const role = message.sender?.role
    return role === 'agent' || role === 'admin'
  }

  /** 会话快照是否完全一致（用于忽略重复投递的 conversation.updated） */
  function sameConversationSnapshot(a: Conversation, b: Conversation): boolean {
    return (
      a.status === b.status &&
      (a.unread || 0) === (b.unread || 0) &&
      (a.agent?.id ?? 0) === (b.agent?.id ?? 0) &&
      (a.last_message || '') === (b.last_message || '') &&
      (a.last_message_at || '') === (b.last_message_at || '') &&
      !!a.archived === !!b.archived &&
      (a.unreplied_ts || 0) === (b.unreplied_ts || 0)
    )
  }

  /** WebSocket 收到新消息 */
  function onMessage(message: Message): void {
    if (message.conversation_id === currentId.value) {
      upsertMessage(message)
      // 只对「客户发来的消息」置已读：幂等后同一条消息只会发 1 次 read；
      // 统计不再在这里单独刷新，交给随之而来的 conversation.updated（合并去抖）。
      if (message.direction === 'in' && !isOwnMessage(message)) {
        void markRead(message.conversation_id, message.id)
      }
      return
    }
    const target = conversations.value.find((c) => c.id === message.conversation_id)
    if (target) {
      // 同一条消息重复投递：不再重复累加未读，也不再重复刷新统计
      const counted = lastCountedMessageId.get(message.conversation_id) ?? 0
      if (message.id > 0 && message.id <= counted) return
      if (message.id > 0) lastCountedMessageId.set(message.conversation_id, message.id)
      target.unread = (target.unread || 0) + 1
      target.last_message = previewOf({ type: message.type, content: message.content })
    } else {
      void loadConversations()
    }
    // 非当前会话：本地累加未读 + 合并刷新统计（250ms 窗口内的多次调用只发 1 个请求）
    refreshStats()
  }

  /** WebSocket 收到会话新建/更新 */
  function onConversation(conversation: Conversation): void {
    const prev = conversations.value.find((c) => c.id === conversation.id)
    upsertConversation(conversation)
    // 重复投递的同一条 conversation.updated（快照完全一样）→ 会话与统计都没变化，不再刷新
    if (prev && sameConversationSnapshot(prev, conversation)) return
    refreshStats()
  }

  /**
   * 请求刷新统计：250ms 尾随防抖 + 单飞。
   * WS 事件、发送成功、会话操作会连续触发多次，这里合并成 1 个 GET /conversations/stats；
   * 若请求在途，期间的调用只置脏标记，在途结束后最多再补 1 次。
   */
  function refreshStats(): void {
    if (statsTimer) window.clearTimeout(statsTimer)
    statsTimer = window.setTimeout(() => {
      statsTimer = 0
      void flushStats()
    }, STATS_DEBOUNCE_MS)
  }

  /** 真正发出统计请求（单飞 + 脏标记补偿） */
  async function flushStats(): Promise<void> {
    if (statsInflight) {
      statsDirty = true
      return
    }
    statsInflight = true
    statsDirty = false
    try {
      await loadStats()
    } finally {
      statsInflight = false
      if (statsDirty) {
        statsDirty = false
        void flushStats()
      }
    }
  }

  /** 组件卸载时清理合并定时器，避免离开工作台后还补发统计请求 */
  function dispose(): void {
    if (statsTimer) {
      window.clearTimeout(statsTimer)
      statsTimer = 0
    }
    statsDirty = false
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
    hasMoreMessages,
    loadingMoreMessages,
    searchResults,
    searching,
    channels,
    stats,
    filters,
    total,
    listLoading,
    messageLoading,
    sending,
    sendingCurrent,
    pendingCount,
    unreadTotal,
    loadChannels,
    loadStats,
    loadConversations,
    loadMessages,
    loadMoreMessages,
    searchMessages,
    clearSearch,
    archiveConversation,
    clearAllConversations,
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
    refreshStats,
    dispose,
  }
})

/** 列表里「最后一条消息」的展示文本 */
export function previewOf(payload: Pick<SendMessagePayload, 'type' | 'content'>): string {
  const map: Record<MessageType, string> = {
    text: '',
    image: '[图片]',
    video: '[视频]',
    file: '[文件]',
    order: '[订单]',
    product: '[商品]',
    system: '',
  }
  if (payload.type === 'text') return payload.content
  return map[payload.type] || '[消息]'
}
