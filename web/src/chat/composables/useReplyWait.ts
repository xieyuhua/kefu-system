/**
 * 「未回复计时」：客服工作台里，最后一条客户消息之后若没有客服回复，
 * 就从这条客户消息的时间开始每秒累加，供气泡下方显示「未回复 N 秒」。
 *
 * 设计要点：
 *  - 判定纯函数化（isCustomerMessage / isAgentReply / findPendingCustomerMessage），
 *    便于单测与复用，不依赖任何 Vue 状态。
 *  - 整个聊天区只跑一个 setInterval（不是每条消息各开一个），
 *    没有未回复消息时立即停表；组件卸载、切换会话时一并清理。
 */
import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import type { Message } from '@/shared/types'
import { messageTimestampMs } from '@/shared/utils/format'

/** 计时刷新间隔：每秒一次 */
const TICK_MS = 1000

/**
 * 是否「客户消息」：direction 为 in，或发送者角色为 user（客户）。
 * 系统提示（转人工、结束会话、离线等）两边都不算，跳过。
 */
export function isCustomerMessage(message: Message): boolean {
  if (message.direction === 'system' || message.type === 'system') return false
  return message.direction === 'in' || message.sender?.role === 'user'
}

/**
 * 是否「客服回复」：direction 为 out 且发送者角色是客服（agent / admin）。
 * 机器人回复的 role 是 robot、系统消息是 system，都不算客服回复——
 * 它们并没有真的把会话接过去，客户仍在等人工。
 * sender 为空只出现在客服自己刚发出、还没拿到服务端回包的乐观消息上，
 * 这条消息就是客服的回复，立即停表（避免「明明回了还在计时」）。
 */
export function isAgentReply(message: Message): boolean {
  if (message.direction !== 'out') return false
  const role = message.sender?.role
  if (!role) return true
  return role === 'agent' || role === 'admin'
}

/**
 * 找出「尚未回复的客户消息」，没有则返回 null。
 *
 * 从列表末尾往前扫：
 *   - 先遇到客服回复 → 说明最后一条客户消息之后已有回复，不存在未回复；
 *   - 先遇到客户消息 → 它就是最后一条客户消息且其后无回复，即未回复起点。
 * 单趟 O(n)，不依赖额外的排序或时间字段。
 */
export function findPendingCustomerMessage(list: Message[]): Message | null {
  for (let i = list.length - 1; i >= 0; i--) {
    const m = list[i]
    if (isAgentReply(m)) return null
    if (isCustomerMessage(m)) return m
  }
  return null
}

export interface ReplyWaitOptions {
  /** 当前会话已加载的消息（升序） */
  messages: Ref<Message[]>
  /** 当前会话 id：切换会话即重新计算 */
  conversationId: Ref<number | undefined>
  /** 是否允许计时，例如会话已结束就停表（默认允许） */
  active?: () => boolean
}

export function useReplyWait(options: ReplyWaitOptions) {
  /** 尚未回复的客户消息（无则 null） */
  const pendingMessage = computed<Message | null>(() => {
    if (!options.conversationId.value) return null
    if (options.active && !options.active()) return null
    return findPendingCustomerMessage(options.messages.value)
  })

  /** 计时起点（Unix 毫秒），0 表示不计时 */
  const startMs = computed(() =>
    pendingMessage.value ? messageTimestampMs(pendingMessage.value) : 0,
  )

  /** 需要显示计时的消息 id（0 表示不显示） */
  const pendingId = computed(() => (startMs.value > 0 && pendingMessage.value ? pendingMessage.value.id : 0))

  /** 当前时间：由唯一的一个 setInterval 每秒推进，驱动 waitMs 重算 */
  const now = ref(Date.now())
  let timer: number | null = null

  function stopTicker(): void {
    if (timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
  }

  function startTicker(): void {
    stopTicker()
    // 起始就对齐一次，避免刚出现计时时数字滞后接近 1 秒
    now.value = Date.now()
    timer = window.setInterval(() => {
      now.value = Date.now()
    }, TICK_MS)
  }

  /** 已等待毫秒数；没有未回复消息时为 0 */
  const waitMs = computed(() => (startMs.value > 0 ? Math.max(0, now.value - startMs.value) : 0))

  // 起点变化 = 出现/切换/消失未回复消息（新消息到达、会话切换、客服回复都走这里）
  watch(
    startMs,
    (value) => {
      if (value > 0) {
        startTicker()
      } else {
        stopTicker()
        now.value = Date.now()
      }
    },
    { immediate: true },
  )

  onBeforeUnmount(stopTicker)

  return { pendingMessage, pendingId, waitMs }
}
