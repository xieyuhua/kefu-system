import { ref, type Ref } from 'vue'
import type { SocketEvent, SocketFrame } from '@/types'

export type SocketStatus = 'connecting' | 'open' | 'closed' | 'fallback'

type Handler = (data: never) => void

const MAX_RETRY_INTERVAL = 15000
const BASE_RETRY_INTERVAL = 1000

/** 连续失败达到该次数后降级为轮询（仅访客端使用） */
const FALLBACK_AFTER_FAILURES = 3

interface Session {
  status: Ref<SocketStatus>
  handlers: Map<SocketEvent, Set<Handler>>
  socket: WebSocket | null
  retryTimer: number | null
  retryCount: number
  failures: number
  manualClosed: boolean
  /** 最近的连接参数，重连时复用 */
  params: { token?: string; device?: string; guest?: Record<string, string> }
  /** 访客端降级轮询定时器 */
  fallbackTimer: number | null
  /** 离线消息游标（访客端） */
  sinceId: number
  /** 访客端轮询兜底取消息的函数（由调用方注入，避免循环依赖） */
  fallbackFetch: ((sinceId: number) => Promise<{ event: SocketEvent; data: unknown }[]>) | null
}

const sessions = new Map<string, Session>()

function newSession(): Session {
  return {
    status: ref<SocketStatus>('closed'),
    handlers: new Map(),
    socket: null,
    retryTimer: null,
    retryCount: 0,
    failures: 0,
    manualClosed: false,
    params: {},
    fallbackTimer: null,
    sinceId: 0,
    fallbackFetch: null,
  }
}

function emit(s: Session, event: SocketEvent, data: unknown): void {
  const set = s.handlers.get(event)
  if (!set) return
  set.forEach((handler) => {
    try {
      ;(handler as (payload: unknown) => void)(data)
    } catch (err) {
      console.error('[ws] handler error', event, err)
    }
  })
}

function buildURL(s: Session): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const base = `${protocol}//${window.location.host}`
  const p = s.params
  if (p.token) {
    return `${base}/ws?token=${encodeURIComponent(p.token)}&device=${p.device || 'web'}`
  }
  // 访客端：/ws/guest?channel=..&uid=..&since_id=..
  const q = new URLSearchParams()
  Object.entries(p.guest || {}).forEach(([k, v]) => {
    if (v) q.set(k, v)
  })
  if (s.sinceId > 0) q.set('since_id', String(s.sinceId))
  return `${base}/ws/guest?${q.toString()}`
}

function scheduleReconnect(key: string, s: Session): void {
  if (s.manualClosed || s.retryTimer !== null) return
  const delay = Math.min(BASE_RETRY_INTERVAL * 2 ** s.retryCount, MAX_RETRY_INTERVAL)
  s.retryTimer = window.setTimeout(() => {
    s.retryTimer = null
    open(key, s)
  }, delay)
}

/** 访客端降级轮询：WebSocket 不可用时兜底，保证消息仍能收到 */
function startFallback(key: string, s: Session): void {
  void key
  if (s.fallbackTimer !== null) return
  s.status.value = 'fallback'
  s.fallbackTimer = window.setInterval(() => {
    void runFallback(s)
  }, 3000)
  void runFallback(s)
}

function stopFallback(s: Session): void {
  if (s.fallbackTimer !== null) {
    window.clearInterval(s.fallbackTimer)
    s.fallbackTimer = null
  }
}

async function runFallback(s: Session): Promise<void> {
  if (!s.fallbackFetch) return
  try {
    const list = await s.fallbackFetch(s.sinceId)
    list.forEach((f) => {
      s.sinceId = Math.max(s.sinceId, extractID(f.data))
      emit(s, f.event, f.data)
    })
  } catch {
    // 网络异常，下个周期继续
  }
}

/** 从推送数据里取消息 id（用于推进离线游标） */
function extractID(data: unknown): number {
  if (data && typeof data === 'object' && 'id' in data) {
    const v = (data as { id: unknown }).id
    return typeof v === 'number' ? v : 0
  }
  return 0
}

function open(key: string, s: Session): WebSocket | null {
  if (!s.params.token && !s.params.guest) return null
  if (s.socket && (s.socket.readyState === WebSocket.OPEN || s.socket.readyState === WebSocket.CONNECTING)) {
    return s.socket
  }
  s.manualClosed = false
  s.status.value = 'connecting'
  const ws = new WebSocket(buildURL(s))
  s.socket = ws

  ws.onopen = () => {
    s.retryCount = 0
    s.failures = 0
    s.status.value = 'open'
    stopFallback(s)
  }

  ws.onmessage = (evt: MessageEvent<string>) => {
    let frame: SocketFrame | null = null
    try {
      frame = JSON.parse(evt.data) as SocketFrame
    } catch {
      return
    }
    if (!frame || !frame.event) return
    // 推进访客端离线游标，避免重连后重复回放
    if (s.params.guest) {
      const id = extractID(frame.data)
      if (id > s.sinceId) s.sinceId = id
    }
    emit(s, frame.event, frame.data)
  }

  ws.onerror = () => {
    // onerror 后通常会紧跟 onclose，重连逻辑放在 onclose
  }

  ws.onclose = () => {
    s.status.value = 'closed'
    if (s.socket === ws) s.socket = null
    if (s.manualClosed) return
    s.retryCount += 1
    s.failures += 1
    // 访客端连续失败多次 → 降级轮询；客服端只做指数退避重连
    if (s.params.guest && s.failures >= FALLBACK_AFTER_FAILURES) {
      startFallback(key, s)
    }
    scheduleReconnect(key, s)
  }

  return ws
}

/**
 * WebSocket 组合式 API：按 key 维护独立连接（客服端 key='agent'，访客端按 channel+uid）。
 * - 自动重连（指数退避 1s → 15s）
 * - 访客端连接持续失败时自动降级为轮询，恢复后自动切回
 * - 重连时带上 since_id，服务端回放离线期间累积的消息
 */
export function useSocket(key = 'agent') {
  let s = sessions.get(key)
  if (!s) {
    s = newSession()
    sessions.set(key, s)
  }
  const session = s

  function connect(
    paramsOrToken: string | { token?: string; device?: string; guest?: Record<string, string> },
  ): WebSocket | null {
    // 兼容旧的 connect(token) 调用形式
    const params =
      typeof paramsOrToken === 'string' ? { token: paramsOrToken, device: 'web' } : paramsOrToken
    session.params = { ...session.params, ...params }
    session.manualClosed = false
    if (session.socket) session.socket.close()
    return open(key, session)
  }

  function close(): void {
    session.manualClosed = true
    if (session.retryTimer !== null) {
      window.clearTimeout(session.retryTimer)
      session.retryTimer = null
    }
    stopFallback(session)
    if (session.socket) {
      session.socket.close()
      session.socket = null
    }
    session.status.value = 'closed'
  }

  /** 设置离线游标（下次连接/重连时用于回放） */
  function setSinceId(id: number): void {
    if (id > session.sinceId) session.sinceId = id
  }

  function getSinceId(): number {
    return session.sinceId
  }

  /** 注入降级轮询的取数函数（访客端） */
  function setFallbackFetch(fn: ((sinceId: number) => Promise<{ event: SocketEvent; data: unknown }[]>) | null): void {
    session.fallbackFetch = fn
  }

  /** 订阅事件，返回取消订阅函数 */
  function on<T>(event: SocketEvent, handler: (payload: T) => void): () => void {
    let set = session.handlers.get(event)
    if (!set) {
      set = new Set<Handler>()
      session.handlers.set(event, set)
    }
    const wrapped = handler as unknown as Handler
    set.add(wrapped)
    return () => {
      set?.delete(wrapped)
    }
  }

  return {
    status: session.status,
    connect,
    close,
    on,
    setSinceId,
    getSinceId,
    setFallbackFetch,
  }
}
