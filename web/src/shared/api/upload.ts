import http from './http'
import type { MediaPayload } from '@/shared/components/MediaBubble.vue'

/**
 * 上传附件（图片 / 视频 / 文件）。
 * - 客服端：POST /api/upload
 * - 用户端：POST /api/guest/upload（无需登录，配合 channel/uid）
 *
 * 返回的载荷可直接作为消息 content 下发（JSON 字符串）。
 */
export async function uploadAttachment(
  file: File,
  opts?: { guest?: boolean; params?: Record<string, string> },
): Promise<MediaPayload> {
  const form = new FormData()
  form.append('file', file)

  const url = opts?.guest ? '/guest/upload' : '/upload'
  const config = opts?.params ? { params: opts.params } : undefined

  return http.post<MediaPayload>(url, form, config)
}

/** 文件大小格式化（上传前提示用） */
export function humanSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

/** 单文件大小上限（与后端一致） */
export const MAX_UPLOAD_SIZE = 50 * 1024 * 1024

/** 上传前校验：不合法时返回中文错误文案 */
export function validateFile(file: File): string {
  const imageExt = /\.(jpg|jpeg|png|gif|webp|bmp|svg)$/i
  const videoExt = /\.(mp4|webm|mov|m4v|ogg)$/i
  const fileExt = /\.(pdf|doc|docx|xls|xlsx|ppt|pptx|txt|csv|zip|rar|7z|json|log)$/i
  if (!imageExt.test(file.name) && !videoExt.test(file.name) && !fileExt.test(file.name)) {
    return `不支持的文件类型：${file.name.split('.').pop() || ''}`
  }
  if (file.size > MAX_UPLOAD_SIZE) {
    return `文件过大（${humanSize(file.size)}），上限 ${humanSize(MAX_UPLOAD_SIZE)}`
  }
  if (file.size === 0) return '文件内容为空'
  return ''
}

/** 依据文件名后缀判断类别 */
export function kindOfFile(file: File): 'image' | 'video' | 'file' {
  if (/\.(jpg|jpeg|png|gif|webp|bmp|svg)$/i.test(file.name)) return 'image'
  if (/\.(mp4|webm|mov|m4v|ogg)$/i.test(file.name)) return 'video'
  return 'file'
}
