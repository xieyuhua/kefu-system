import dayjs from 'dayjs'
import type { ChannelType, ConversationStatus, MessageType } from '@/shared/types'

/** 渠道配色（参考企业微信客服视觉规范） */
export const CHANNEL_COLORS: Record<ChannelType, string> = {
  wechat: '#07C160',
  wecom: '#2C7BE5',
  web: '#4A90D9',
  app: '#F5A623',
  weibo: '#E6162D',
  douyin: '#161823',
  robot: '#8B5CF6',
}

/** 渠道图标（emoji 占位，避免引入新依赖） */
export const CHANNEL_ICONS: Record<ChannelType, string> = {
  wechat: '💬',
  wecom: '🏢',
  web: '🌐',
  app: '📱',
  weibo: '🅦',
  douyin: '🎵',
  robot: '🤖',
}

export const CHANNEL_LABELS: Record<ChannelType, string> = {
  wechat: '微信',
  wecom: '企业微信',
  web: '网页',
  app: 'APP',
  weibo: '微博',
  douyin: '抖音',
  robot: '机器人',
}

export const STATUS_LABELS: Record<ConversationStatus, string> = {
  pending: '待接待',
  open: '进行中',
  closed: '已结束',
}

const TYPE_LABELS: Record<MessageType, string> = {
  text: '文本',
  image: '图片',
  video: '视频',
  file: '文件',
  order: '订单',
  product: '商品',
  system: '系统',
}

export function messageTypeLabel(type: MessageType): string {
  return TYPE_LABELS[type] || '消息'
}

export function channelColor(type: ChannelType | undefined): string {
  return type ? CHANNEL_COLORS[type] || '#7F7F7F' : '#7F7F7F'
}

export function channelIcon(type: ChannelType | undefined): string {
  return type ? CHANNEL_ICONS[type] || '💬' : '💬'
}

export function channelLabel(type: ChannelType | undefined): string {
  return type ? CHANNEL_LABELS[type] || type : '未知渠道'
}

export function statusLabel(status: ConversationStatus | undefined): string {
  return status ? STATUS_LABELS[status] || status : '未知状态'
}

/** 会话列表时间展示：今天 HH:mm，昨天 昨天，更早 MM-DD */
export function formatListTime(value: string | undefined): string {
  if (!value) return ''
  const d = dayjs(value.replace(/-/g, '/'))
  if (!d.isValid()) return ''
  const now = dayjs()
  if (d.isSame(now, 'day')) return d.format('HH:mm')
  if (d.isSame(now.subtract(1, 'day'), 'day')) return '昨天'
  if (d.isSame(now, 'year')) return d.format('MM-DD')
  return d.format('YYYY-MM-DD')
}

/** 聊天气泡时间：HH:mm（跨天补日期） */
export function formatMessageTime(value: string | undefined): string {
  if (!value) return ''
  const d = dayjs(value.replace(/-/g, '/'))
  if (!d.isValid()) return ''
  const now = dayjs()
  if (d.isSame(now, 'day')) return d.format('HH:mm')
  if (d.isSame(now, 'year')) return d.format('MM-DD HH:mm')
  return d.format('YYYY-MM-DD HH:mm')
}

/**
 * 消息创建时间（Unix 毫秒）：优先用后端补充的 created_ts，
 * 缺失时回退解析 created_at（`YYYY-MM-DD HH:mm:ss`，按本地时间解析）。
 * 解析失败返回 0，调用方据此放弃计时，避免出现天文数字。
 */
export function messageTimestampMs(message: {
  created_ts?: number
  created_at?: string
}): number {
  const ts = message.created_ts
  if (typeof ts === 'number' && Number.isFinite(ts) && ts > 0) return ts
  const raw = message.created_at
  if (!raw) return 0
  // 统一替换成 /，避免 Safari 对「YYYY-MM-DD HH:mm:ss」解析出 Invalid Date
  const d = dayjs(raw.replace(/-/g, '/'))
  return d.isValid() ? d.valueOf() : 0
}

/**
 * 未回复等待时长文案：不足 1 分钟显示「38 秒」，超过显示「2 分 05 秒」。
 * 只做秒级取整（向下），配合每秒刷新的计时使用。
 */
export function formatWaitDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  // 按量级自动切换单位，最多保留两级（秒 / 分 / 小时 / 天），避免出现「3600 秒」这种难读的数字
  if (total < 60) return `${total} 秒`

  const minutes = Math.floor(total / 60)
  if (minutes < 60) {
    const seconds = total % 60
    return seconds > 0 ? `${minutes} 分 ${seconds} 秒` : `${minutes} 分`
  }

  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    const restMinutes = minutes % 60
    return restMinutes > 0 ? `${hours} 小时 ${restMinutes} 分` : `${hours} 小时`
  }

  const days = Math.floor(hours / 24)
  const restHours = hours % 24
  return restHours > 0 ? `${days} 天 ${restHours} 小时` : `${days} 天`
}

/** 在线状态圆点颜色：在线 #07C160，离线 #b2b2b2 */
export function onlineColor(online: boolean | undefined): string {
  return online ? '#07C160' : '#b2b2b2'
}

/** 在线状态提示文案：在线 / 离线 · 最后活跃 x 分钟前 */
export function onlineHint(online: boolean | undefined, lastSeenAt?: string): string {
  if (online) return '在线'
  if (!lastSeenAt) return '离线'
  const d = dayjs(lastSeenAt.replace(/-/g, '/'))
  if (!d.isValid()) return '离线'
  const mins = dayjs().diff(d, 'minute')
  if (mins < 1) return '离线 · 最后活跃刚刚'
  if (mins < 60) return `离线 · 最后活跃 ${mins} 分钟前`
  const hours = dayjs().diff(d, 'hour')
  if (hours < 24) return `离线 · 最后活跃 ${hours} 小时前`
  const days = dayjs().diff(d, 'day')
  if (days < 30) return `离线 · 最后活跃 ${days} 天前`
  return '离线'
}

export function formatMoney(value: number | undefined): string {
  if (value === undefined || value === null || Number.isNaN(value)) return '--'
  return `¥${Number(value).toFixed(2)}`
}

/** 安全解析卡片消息 content（type=order/product 时为 JSON 字符串） */
export function parseCardContent<T>(content: string): T | null {
  try {
    const parsed = JSON.parse(content) as T
    return parsed ?? null
  } catch {
    return null
  }
}
