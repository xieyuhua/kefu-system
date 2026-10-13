import dayjs from 'dayjs'
import type { ChannelType, ConversationStatus, MessageType } from '@/types'

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
