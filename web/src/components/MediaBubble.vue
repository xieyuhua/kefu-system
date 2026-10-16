<script setup lang="ts">
/**
 * 媒体消息气泡：图片（点击放大）、视频（点击全屏播放）、文件（可下载）。
 * 客服端与用户端共用本组件。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { Download, Document, VideoPlay } from '@element-plus/icons-vue'

/** 图片 / 视频 / 文件 的载荷（由后端 upload 接口返回后随消息 content 下发） */
export interface MediaPayload {
  url: string
  kind: 'image' | 'video' | 'file'
  name?: string
  size?: number
  ext?: string
  /** 视频封面（可选） */
  poster?: string
}

const props = defineProps<{
  payload: MediaPayload
  failed?: boolean
}>()

const imageViewerVisible = ref(false)
const videoFullscreen = ref(false)
const videoError = ref(false)
/** 全屏播放器的 video ref：打开时挂载，需等 nextTick 后才能 focus/play */
const fsVideoRef = ref<HTMLVideoElement | null>(null)

const isImage = computed(() => props.payload.kind === 'image')
const isVideo = computed(() => props.payload.kind === 'video')
const isFile = computed(() => props.payload.kind === 'file')

const fileName = computed(() => props.payload.name || decodeURI(props.payload.url.split('/').pop() || '文件'))
const fileSize = computed(() => formatSize(props.payload.size))
const fileExt = computed(() => (props.payload.ext || '').replace('.', '').toUpperCase())

function formatSize(size?: number): string {
  if (!size || size <= 0) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function onFileClick(): void {
  // 用 a[download] 触发浏览器下载
  const a = document.createElement('a')
  a.href = props.payload.url
  a.download = fileName.value
  a.target = '_blank'
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

/** 打开视频全屏播放器（客服端与用户端都支持） */
async function openVideoFullscreen(): Promise<void> {
  if (videoError.value) return
  videoFullscreen.value = true
  await nextTick()
  // 自动播放；被浏览器拦截时忽略（用户可手动点播放）
  fsVideoRef.value?.play().catch(() => undefined)
}

/** 关闭全屏并停止播放 */
function closeVideoFullscreen(): void {
  videoFullscreen.value = false
  const v = fsVideoRef.value
  if (v) {
    v.pause()
    // 重置进度，下次打开从头播放
    v.currentTime = 0
  }
}

/** ESC 关闭 */
function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape' && videoFullscreen.value) closeVideoFullscreen()
}

watch(
  () => videoFullscreen,
  (open) => {
    if (typeof document === 'undefined') return
    // 打开时锁定页面滚动，避免背景跟着滚
    document.body.style.overflow = open ? 'hidden' : ''
    if (open) window.addEventListener('keydown', onKeydown)
    else window.removeEventListener('keydown', onKeydown)
  },
)
</script>

<template>
  <!-- 图片：点击放大 -->
  <div v-if="isImage" class="media media--image">
    <img
      class="media__img"
      :src="payload.url"
      :alt="payload.name || '图片'"
      loading="lazy"
      @click="imageViewerVisible = true"
    />
    <div class="media__zoom-hint">点击查看大图</div>
    <el-image-viewer
      v-if="imageViewerVisible"
      :url-list="[payload.url]"
      hide-on-click-modal
      teleported
      @close="imageViewerVisible = false"
    />
  </div>

  <!-- 视频：缩略播放器，点击进入全屏 -->
  <div v-else-if="isVideo" class="media media--video">
    <template v-if="!videoError">
      <video
        class="media__video"
        :src="payload.url"
        :poster="payload.poster"
        controls
        preload="metadata"
        playsinline
        @error="videoError = true"
      />
      <button type="button" class="media__fs-btn" title="全屏播放" @click="openVideoFullscreen">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2.2">
          <path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3" />
        </svg>
      </button>
    </template>
    <div v-else class="media__video-fallback">
      <VideoPlay class="media__icon" />
      <span class="media__name">{{ payload.name || '视频' }}</span>
      <el-button size="small" type="primary" plain @click="onFileClick">下载观看</el-button>
    </div>
  </div>

  <!-- 文件：可下载 -->
  <div v-else-if="isFile" class="media media--file" role="button" @click="onFileClick">
    <div class="media__file-icon">
      <Document class="media__icon" />
      <span v-if="fileExt" class="media__file-ext">{{ fileExt }}</span>
    </div>
    <div class="media__file-info">
      <div class="media__file-name kf-ellipsis" :title="fileName">{{ fileName }}</div>
      <div v-if="fileSize" class="media__file-size">{{ fileSize }}</div>
    </div>
    <el-icon class="media__download"><Download /></el-icon>
  </div>

  <span v-else class="media__text">{{ payload.url }}</span>

  <!-- 视频全屏播放器（客服端 / 用户端通用）。
       必须放在 v-if / v-else 链之外，否则会打断 else 分支。 -->
  <Teleport to="body">
    <div v-if="videoFullscreen" class="fs-player" @click.self="closeVideoFullscreen">
      <video
        ref="fsVideoRef"
        class="fs-player__video"
        :src="payload.url"
        :poster="payload.poster"
        controls
        autoplay
        playsinline
        webkit-playsinline
        @click.stop
      />
      <button
        type="button"
        class="fs-player__close"
        title="关闭（Esc）"
        @click="closeVideoFullscreen"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2.2">
          <path d="M18 6 6 18M6 6l12 12" />
        </svg>
      </button>
    </div>
  </Teleport>
</template>

<style scoped lang="scss">
.media {
  &--image {
    position: relative;
    cursor: zoom-in;
    border-radius: 6px;
    overflow: hidden;
    line-height: 0;

    &:hover .media__zoom-hint {
      opacity: 1;
    }
  }

  /* 视频：缩略播放器 + 全屏按钮 */
  &--video {
    position: relative;
    display: inline-block;
    max-width: 100%;
    border-radius: 6px;
    overflow: hidden;
    line-height: 0;

    &:hover .media__fs-btn {
      opacity: 1;
    }
  }
}

/* 视频右上角的全屏按钮 */
.media__fs-btn {
  position: absolute;
  top: 6px;
  right: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: none;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.18s, background 0.18s;

  &:hover {
    background: rgba(0, 0, 0, 0.78);
  }

  /* 触屏设备没有 hover，常驻显示 */
  @media (hover: none) {
    opacity: 1;
  }
}

/* ---------------- 视频全屏播放器 ---------------- */
.fs-player {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.92);
  /* 移动端全屏时避让刘海屏/手势条 */
  padding: env(safe-area-inset-top, 0) env(safe-area-inset-right, 0)
    env(safe-area-inset-bottom, 0) env(safe-area-inset-left, 0);

  &__video {
    max-width: 100%;
    max-height: 100%;
    background: #000;
    outline: none;
  }

  &__close {
    position: absolute;
    top: calc(16px + env(safe-area-inset-top, 0));
    right: calc(16px + env(safe-area-inset-right, 0));
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.16);
    color: #fff;
    cursor: pointer;
    transition: background 0.18s;

    &:hover {
      background: rgba(255, 255, 255, 0.28);
    }
  }
}

.media__img {
  display: block;
  /* 自适应：不超过最大宽高，也不小于自然尺寸，避免小图被拉伸变形 */
  max-width: 200px;
  max-height: 200px;
  width: auto;
  height: auto;
  border-radius: 6px;
  object-fit: contain;
  background: #f2f3f5;
}

.media__zoom-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.4);
  color: #fff;
  font-size: 12px;
  line-height: 1.5;
  opacity: 0;
  transition: opacity 0.18s;
  pointer-events: none;
}

.media__video {
  display: block;
  max-width: 240px;
  max-height: 240px;
  width: auto;
  height: auto;
  border-radius: 6px;
  background: #000;
}

.media__video-fallback {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 18px 14px;
  border-radius: 6px;
  background: #f7f7f7;
  min-width: 160px;
}

.media__file {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 200px;
  max-width: 260px;
  padding: 8px 10px;
  border-radius: 6px;
  background: #f7f7f7;
  cursor: pointer;
  line-height: 1.4;

  &:hover {
    background: #eef0f2;
  }
}

.media__file-icon {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: 6px;
  background: #fff;
  color: #4a6fa5;
}

.media__file-ext {
  position: absolute;
  bottom: -1px;
  right: -2px;
  padding: 0 2px;
  border-radius: 2px;
  background: #4a6fa5;
  color: #fff;
  font-size: 9px;
  line-height: 12px;
}

.media__file-info {
  flex: 1;
  min-width: 0;
}

.media__file-name {
  font-size: 13px;
  color: #191919;
}

.media__file-size {
  font-size: 11px;
  color: #7f7f7f;
}

.media__download {
  flex-shrink: 0;
  color: #7f7f7f;
}

.media__icon {
  width: 18px;
  height: 18px;
}

.media__name {
  font-size: 12px;
  color: #7f7f7f;
}

.media__text {
  font-size: 13px;
  color: var(--kf-text);
}
</style>
