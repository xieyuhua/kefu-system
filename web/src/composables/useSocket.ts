import { ref, type Ref } from 'vue'
import type { SocketEvent, SocketFrame } from '@/types'

export type SocketStatus = 'connecting' | 'open' | 'closed'

type Handler = (data: never) => void

const MAX_RETRY_INTERVAL = 15000
const BASE_RETRY_INTERVAL = 1000

const status: Ref<SocketStatus> = ref<SocketStatus>('closed')
const handlers = new Map<SocketEvent, Set<Handler>>()

let socket: WebSocket | null = null
let retryTimer: number | null = null
let retryCount = 0
let manualClosed = false
let currentToken = ''

function emit(event: SocketEvent, data: unknown): void {
  const set = handlers.get(event)
  if (!set) return
  set.forEach((handler) => {
    try {
      ;(handler as (payload: unknown) => void)(data)
    } catch (err) {
      console.error('[ws] handler error', event, err)
    }
  })
}

function scheduleReconnect(): void {
  if (manualClosed || retryTimer !== null) return
  const delay = Math.min(BASE_RETRY_INTERVAL * 2 ** retryCount, MAX_RETRY_INTERVAL)
  retryTimer = window.setTimeout(() => {
    retryTimer = null
    connect(currentToken)
  }, delay)
}

function connect(token: string): WebSocket | null {
  if (!token) return null
  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    return socket
  }
  currentToken = token
  manualClosed = false
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}&device=web`
  status.value = 'connecting'
  const ws = new WebSocket(url)
  socket = ws

  ws.onopen = () => {
    retryCount = 0
    status.value = 'open'
  }

  ws.onmessage = (evt: MessageEvent<string>) => {
    let frame: SocketFrame | null = null
    try {
      frame = JSON.parse(evt.data) as SocketFrame
    } catch {
      return
    }
    if (!frame || !frame.event) return
    emit(frame.event, frame.data)
  }

  ws.onerror = () => {
    // onerror 后通常会紧跟 onclose，重连放在 onclose 处理
  }

  ws.onclose = () => {
    status.value = 'closed'
    if (socket === ws) socket = null
    if (manualClosed) return
    retryCount += 1
    scheduleReconnect()
  }

  return ws
}

function close(): void {
  manualClosed = true
  if (retryTimer !== null) {
    window.clearTimeout(retryTimer)
    retryTimer = null
  }
  if (socket) {
    socket.close()
    socket = null
  }
  status.value = 'closed'
}

function parseFrame(frame: SocketFrame): void {
  emit(frame.event, frame.data)
}

/** 订阅 WebSocket 事件，返回取消订阅函数 */
function on<T>(event: SocketEvent, handler: (payload: T) => void): () => void {
  let set = handlers.get(event)
  if (!set) {
    set = new Set<Handler>()
    handlers.set(event, set)
  }
  const wrapped = handler as unknown as Handler
  set.add(wrapped)
  return () => {
    set?.delete(wrapped)
  }
}

/** WebSocket 组合式 API：单例连接，指数退避自动重连 */
export function useSocket() {
  return {
    status,
    connect,
    close,
    on,
    parseFrame,
  }
}
