<script setup lang="ts">
/**
 * 全屏图片预览：移动端双指捏合放大/缩小、单指拖拽平移、双击切换缩放；
 * PC 端滚轮缩放。客服端与用户端共用。
 *
 * 交互约定：
 *  - 缩放范围 1~5 倍；
 *  - scale = 1 时居中，不可拖拽；scale > 1 时单指可拖动；
 *  - 缩小回到 1 倍时自动回正居中；
 *  - 点击遮罩或右上角关闭按钮退出。
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  visible: boolean
  url: string
  alt?: string
}>()

const emit = defineEmits<{
  close: []
}>()

const MIN_SCALE = 1
const MAX_SCALE = 5

const scale = ref(1)
const tx = ref(0)
const ty = ref(0)

/* ---------------- 手势状态 ---------------- */
interface TouchPoint {
  x: number
  y: number
}

let pinchDist0 = 0
let pinchScale0 = 1
let pinchCenter0: TouchPoint = { x: 0, y: 0 }
let dragStart: TouchPoint | null = null
let dragTx0 = 0
let dragTy0 = 0
/** 移动端双击检测：上次 tap 的时间与位置 */
let lastTapTime = 0
let lastTapPos: TouchPoint = { x: 0, y: 0 }
/** 防抖：触摸检测与浏览器合成的 dblclick 可能先后都触发，短时间只执行一次 */
let lastZoomAt = 0

const style = computed(() => ({
  transform: `translate(${tx.value}px, ${ty.value}px) scale(${scale.value})`,
  transition: dragging.value ? 'none' : 'transform 0.22s ease',
}))

/** 拖拽中禁用过渡，否则手指移动时图片会「飘」 */
const dragging = ref(false)

function clampScale(v: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, v))
}

function reset(): void {
  scale.value = 1
  tx.value = 0
  ty.value = 0
  dragging.value = false
}

/* ---------------- 触摸手势 ---------------- */
function distance(a: Touch, b: Touch): number {
  return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY)
}

function midpoint(a: Touch, b: Touch): TouchPoint {
  return { x: (a.clientX + b.clientX) / 2, y: (a.clientY + b.clientY) / 2 }
}

function onTouchStart(e: TouchEvent): void {
  const touches = e.touches
  if (touches.length === 2) {
    // 双指捏合开始
    pinchDist0 = distance(touches[0], touches[1])
    pinchScale0 = scale.value
    pinchCenter0 = midpoint(touches[0], touches[1])
    dragging.value = true
    dragStart = null
  } else if (touches.length === 1) {
    // 单指拖拽（放大后才允许）
    dragStart = { x: touches[0].clientX, y: touches[0].clientY }
    dragTx0 = tx.value
    dragTy0 = ty.value
    dragging.value = scale.value > 1
  }
}

function onTouchMove(e: TouchEvent): void {
  e.preventDefault()
  const touches = e.touches
  if (touches.length === 2) {
    const d = distance(touches[0], touches[1])
    if (pinchDist0 > 0) {
      scale.value = clampScale((pinchScale0 * d) / pinchDist0)
    }
    // 围绕捏合中点缩放：保持手指下的内容点大致不动
    const c = midpoint(touches[0], touches[1])
    if (scale.value > 1) {
      tx.value += (c.x - pinchCenter0.x)
      ty.value += (c.y - pinchCenter0.y)
    } else {
      tx.value = 0
      ty.value = 0
    }
    pinchCenter0 = c
  } else if (touches.length === 1 && dragStart && scale.value > 1) {
    tx.value = dragTx0 + (touches[0].clientX - dragStart.x)
    ty.value = dragTy0 + (touches[0].clientY - dragStart.y)
  }
}

function onTouchEnd(e: TouchEvent): void {
  const touches = e.touches
  if (touches.length === 0) {
    dragStart = null
    dragging.value = false
    if (scale.value <= MIN_SCALE) reset()
    // 移动端双击检测：两次轻点间隔 < 320ms 且位移 < 40px
    const t = e.changedTouches[0]
    if (t) {
      const now = Date.now()
      const pos: TouchPoint = { x: t.clientX, y: t.clientY }
      if (now - lastTapTime < 320 && Math.hypot(pos.x - lastTapPos.x, pos.y - lastTapPos.y) < 40) {
        toggleZoom()
        lastTapTime = 0
      } else {
        lastTapTime = now
        lastTapPos = pos
      }
    }
  } else if (touches.length === 1) {
    // 从双指变单指：重新校准拖拽起点，避免跳变
    dragStart = { x: touches[0].clientX, y: touches[0].clientY }
    dragTx0 = tx.value
    dragTy0 = ty.value
  }
}

/** 双击 / 双指 tap：放大 2.5 倍或还原居中 */
function toggleZoom(): void {
  const now = Date.now()
  if (now - lastZoomAt < 350) return
  lastZoomAt = now
  if (scale.value > 1.2) reset()
  else {
    scale.value = 2.5
    tx.value = 0
    ty.value = 0
  }
}

/* ---------------- PC 滚轮缩放 ---------------- */
function onWheel(e: WheelEvent): void {
  e.preventDefault()
  const delta = e.deltaY > 0 ? -0.15 : 0.15
  const next = clampScale(scale.value + delta)
  if (next <= 1) {
    reset()
    return
  }
  scale.value = next
}

/* ---------------- 打开 / 关闭 ---------------- */
watch(
  () => props.visible,
  (open) => {
    if (open) {
      reset()
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
  },
)

onBeforeUnmount(() => {
  document.body.style.overflow = ''
})

function close(): void {
  emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="img-preview"
      @touchstart.passive="onTouchStart"
      @touchmove="onTouchMove"
      @touchend="onTouchEnd"
      @dblclick="toggleZoom"
      @wheel="onWheel"
      @click.self="close"
    >
      <img
        class="img-preview__img"
        :src="url"
        :alt="alt || '图片预览'"
        :style="style"
        draggable="false"
        @click.stop
      />
      <button type="button" class="img-preview__close" title="关闭" @click="close">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2.2">
          <path d="M18 6 6 18M6 6l12 12" />
        </svg>
      </button>
      <div class="img-preview__hint">双指缩放 · 双击还原</div>
    </div>
  </Teleport>
</template>

<style scoped lang="scss">
.img-preview {
  position: fixed;
  inset: 0;
  z-index: 3200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.94);
  /* 关键：阻止浏览器把双指/拖动识别为页面滚动或缩放 */
  touch-action: none;
  overflow: hidden;
}

.img-preview__img {
  max-width: 100%;
  max-height: 100%;
  transform-origin: center center;
  user-select: none;
  -webkit-user-drag: none;
  cursor: grab;
  transition: transform 0.22s ease;
}

.img-preview__close {
  position: absolute;
  top: calc(14px + env(safe-area-inset-top, 0));
  right: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.16);
  color: #fff;
  cursor: pointer;
  transition: background 0.18s;

  &:hover {
    background: rgba(255, 255, 255, 0.3);
  }
}

.img-preview__hint {
  position: absolute;
  bottom: calc(16px + env(safe-area-inset-bottom, 0));
  left: 50%;
  transform: translateX(-50%);
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.85);
  font-size: 12px;
  pointer-events: none;
}
</style>
