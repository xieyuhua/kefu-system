import http from './http'
import type { AxiosRequestConfig } from 'axios'
import type { MediaPayload } from '@/shared/components/MediaBubble.vue'

/**
 * 上传附件（图片 / 视频 / 文件）。
 * - 客服端：POST /api/upload
 * - 用户端：POST /api/guest/upload（无需登录，配合 channel/uid）
 *
 * 返回的载荷可直接作为消息 content 下发（JSON 字符串）。
 * onProgress 会实时回调 0~100 的整数百分比（浏览器能拿到 total 时才回调）。
 */
export async function uploadAttachment(
  file: File,
  opts?: {
    guest?: boolean
    params?: Record<string, string>
    onProgress?: (percent: number) => void
  },
): Promise<MediaPayload> {
  const form = new FormData()
  form.append('file', file)

  const url = opts?.guest ? '/guest/upload' : '/upload'
  const config: AxiosRequestConfig = {}
  if (opts?.params) config.params = opts.params
  if (opts?.onProgress) {
    config.onUploadProgress = (e) => {
      if (e.total && e.total > 0) {
        opts.onProgress?.(Math.round((e.loaded / e.total) * 100))
      }
    }
  }

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

/** 分片大小：2MB/片，大文件超过分片阈值时自动切换分片上传 */
export const CHUNK_SIZE = 2 * 1024 * 1024
/** 超过该大小才走分片上传（太小分片反而多一次 merge 往返） */
export const CHUNK_THRESHOLD = 4 * 1024 * 1024

/** 生成上传会话 ID（仅十六进制与连字符，后端做同样校验防路径穿越） */
function genUploadId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    try {
      return crypto.randomUUID().replace(/[^0-9a-fA-F-]/g, '')
    } catch {
      // 降级到时间戳随机
    }
  }
  const rand = Math.random().toString(16).slice(2)
  return `${Date.now().toString(16)}-${rand}`
}

/**
 * 分片上传（断点续传）：把文件切成 CHUNK_SIZE 的片逐片上传，
 * 任一片失败自动重试该片；已成功的片在重试时跳过。全部上传后调 merge 合并。
 * onProgress 回调整体进度（已传字节 / 总字节，0~100）。
 */
export async function uploadChunked(
  file: File,
  opts?: {
    guest?: boolean
    params?: Record<string, string>
    onProgress?: (percent: number) => void
  },
): Promise<MediaPayload> {
  const base = opts?.guest ? '/guest/upload' : '/upload'
  const query = { ...(opts?.params ?? {}) }
  const uploadId = genUploadId()
  const total = Math.ceil(file.size / CHUNK_SIZE)
  const uploaded = new Set<number>()

  const chunkBytes = (i: number): number => Math.min(CHUNK_SIZE, file.size - i * CHUNK_SIZE)
  const progress = (): void => {
    let sent = 0
    uploaded.forEach((i) => {
      sent += chunkBytes(i)
    })
    opts?.onProgress?.(Math.round((sent / file.size) * 100))
  }

  const uploadOne = async (i: number): Promise<void> => {
    if (uploaded.has(i)) return
    const start = i * CHUNK_SIZE
    const end = Math.min(start + CHUNK_SIZE, file.size)
    const form = new FormData()
    form.append('file', file.slice(start, end), file.name)
    await http.post<unknown>(`${base}/chunk`, form, {
      params: { ...query, upload_id: uploadId, index: String(i) },
    })
    uploaded.add(i)
    progress()
  }

  // 逐片串行上传，单片失败最多重试 3 次（断点续传：重试只补缺的片）
  for (let i = 0; i < total; i++) {
    let ok = false
    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        await uploadOne(i)
        ok = true
        break
      } catch {
        if (attempt === 3) throw new Error(`第 ${i + 1}/${total} 片上传失败`)
      }
    }
    if (!ok) throw new Error('上传失败')
  }

  const merged = await http.post<MediaPayload>(
    `${base}/merge`,
    {
      upload_id: uploadId,
      filename: file.name,
      ext: file.name.includes('.') ? file.name.slice(file.name.lastIndexOf('.')).toLowerCase() : '',
      size: file.size,
      total,
    },
    { params: query },
  )
  return merged
}

/** 上传前校验：不合法时返回中文错误文案。maxSize 为字节数，缺省用默认 50MB */
export function validateFile(file: File, maxSize?: number): string {
  const imageExt = /\.(jpg|jpeg|png|gif|webp|bmp|svg)$/i
  const videoExt = /\.(mp4|webm|mov|m4v|ogg)$/i
  const fileExt = /\.(pdf|doc|docx|xls|xlsx|ppt|pptx|txt|csv|zip|rar|7z|json|log)$/i
  if (!imageExt.test(file.name) && !videoExt.test(file.name) && !fileExt.test(file.name)) {
    return `不支持的文件类型：${file.name.split('.').pop() || ''}`
  }
  const limit = maxSize && maxSize > 0 ? maxSize : MAX_UPLOAD_SIZE
  if (file.size > limit) {
    return `文件过大（${humanSize(file.size)}），上限 ${humanSize(limit)}`
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
