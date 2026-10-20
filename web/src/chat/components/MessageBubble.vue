<template>
  <!-- 系统通知：居中灰色 pill -->
  <div v-if="isSystem" class="system-pill" :class="{ 'is-flash': flash }" :data-msg-id="message.id">
    <span>{{ text }}</span>
  </div>

  <div
    v-else
    class="bubble-row"
    :class="[isOut ? 'is-out' : 'is-in', { 'is-flash': flash }]"
    :data-msg-id="message.id"
  >
    <div class="avatar">
      <img v-if="avatar" :src="avatar" alt="" />
      <span v-else>{{ fallbackName.slice(0, 1) }}</span>
    </div>

    <div class="bubble-col">
      <div class="bubble-meta">
        <span class="bubble-name">{{ fallbackName }}</span>
        <span class="bubble-time">{{ formatMessageTime(message.created_at) }}</span>
      </div>

      <div class="bubble" :class="{ pending: message.pending, failed: message.failed }">
        <!-- 文本 -->
        <template v-if="message.type === 'text'">
          <span class="bubble-text">{{ text }}</span>
        </template>

        <!-- 图片 / 视频 / 文件（mediaPayload 已兼容历史 URL 写法） -->
        <template v-else-if="mediaPayload">
          <MediaBubble :payload="mediaPayload" :failed="message.failed" />
        </template>

        <!-- 订单卡片 -->
        <template v-else-if="message.type === 'order'">
          <OrderCard v-if="orderPayload" :order="orderPayload" />
          <span v-else class="bubble-text">{{ text }}</span>
        </template>

        <!-- 商品卡片 -->
        <template v-else-if="message.type === 'product'">
          <ProductCard v-if="productPayload" :product="productPayload" />
          <span v-else class="bubble-text">{{ text }}</span>
        </template>

        <template v-else>
          <span class="bubble-text">{{ text }}</span>
        </template>
      </div>

      <!-- 未回复计时：贴在气泡下方（时间戳同一列），只在未回复的客户消息上出现 -->
      <div
        v-if="waitShowing"
        class="bubble-wait"
        :class="{ 'is-compact': compact }"
        :data-wait-msg-id="message.id"
      >
        <span class="bubble-wait__dot" />
        <span class="bubble-wait__text">未回复 {{ waitDuration }}</span>
      </div>

      <div v-if="message.failed" class="bubble-error">发送失败</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Message, OrderCardPayload, ProductCardPayload } from '@/shared/types'
import { formatMessageTime, formatWaitDuration, parseCardContent } from '@/shared/utils/format'
import MediaBubble, { type MediaPayload } from '@/shared/components/MediaBubble.vue'
import OrderCard from '@/shared/components/OrderCard.vue'
import ProductCard from '@/shared/components/ProductCard.vue'

const props = defineProps<{
  message: Message
  /** 搜索定位高亮：传入当前需要高亮的消息 id（变化即触发闪烁） */
  flashKey?: number
  /**
   * 未回复等待时长（毫秒），由 ChatWindow 统一计时后按消息下发；
   * 仅「尚未回复的客户消息」会拿到 > 0 的值，其它消息传 0 不渲染。
   */
  waitMs?: number
  /** 移动端紧凑样式（字号/内边距更小，避免挤压气泡） */
  compact?: boolean
}>()

const isSystem = computed(
  () => props.message.direction === 'system' || props.message.type === 'system',
)
const isOut = computed(() => props.message.direction === 'out')
const text = computed(() => props.message.content || '')

/** 是否展示「未回复」计时：只认父级下发的有效时长 */
const waitShowing = computed(() => !isSystem.value && (props.waitMs || 0) > 0)
const waitDuration = computed(() => formatWaitDuration(props.waitMs || 0))

const orderPayload = computed<OrderCardPayload | null>(() =>
  props.message.type === 'order' ? parseCardContent<OrderCardPayload>(text.value) : null,
)

const productPayload = computed<ProductCardPayload | null>(() =>
  props.message.type === 'product' ? parseCardContent<ProductCardPayload>(text.value) : null,
)

/**
 * 媒体载荷：新消息的 content 是上传接口返回的 JSON；
 * 解析失败时回退为历史写法（content 直接是 URL）。
 */
const mediaPayload = computed<MediaPayload | null>(() => {
  if (props.message.type !== 'image' && props.message.type !== 'video' && props.message.type !== 'file') {
    return null
  }
  const raw = text.value
  if (!raw) return null
  if (raw.startsWith('{')) {
    const p = parseCardContent<MediaPayload>(raw)
    if (p && p.url) return p
    return null
  }
  // 历史消息：直接是 dataURL 或 /uploads 路径
  return { url: raw, kind: props.message.type === 'image' ? 'image' : props.message.type === 'video' ? 'video' : 'file' }
})

const avatar = computed(() => props.message.sender?.avatar || '')

const fallbackName = computed(() => {
  if (props.message.sender?.name) return props.message.sender.name
  return isOut.value ? '我' : '客户'
})

/** 搜索结果定位时短暂高亮 */
const flash = ref(false)
let flashTimer = 0

watch(
  () => props.flashKey,
  (val) => {
    window.clearTimeout(flashTimer)
    if (!val) {
      flash.value = false
      return
    }
    flash.value = true
    flashTimer = window.setTimeout(() => {
      flash.value = false
    }, 1600)
  },
)

onBeforeUnmount(() => {
  window.clearTimeout(flashTimer)
})
</script>

<style scoped lang="scss">
.bubble-row {
  display: flex;
  gap: 8px;
  padding: 6px 0;
  scroll-margin-top: 60px;
  border-radius: 6px;

  &.is-out {
    flex-direction: row-reverse;
  }
}

/* 搜索定位命中：短暂高亮 */
.bubble-row.is-flash {
  animation: kf-flash 1.6s ease;
}

.system-pill.is-flash {
  animation: kf-flash 1.6s ease;
}

@keyframes kf-flash {
  0%,
  60% {
    background: rgba(7, 193, 96, 0.16);
  }
  100% {
    background: transparent;
  }
}

.avatar {
  flex-shrink: 0;
  width: 34px;
  height: 34px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg-active);
  color: var(--kf-text-sub);
  font-size: 13px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
}

.bubble-col {
  display: flex;
  flex-direction: column;
  max-width: 62%;
}

.is-out .bubble-col {
  align-items: flex-end;
}

.bubble-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
  font-size: 12px;
  color: var(--kf-text-weak);
}

.bubble {
  position: relative;
  padding: 8px 12px;
  border-radius: var(--kf-radius);
  background: #fff;
  color: var(--kf-text);
  font-size: 14px;
  line-height: 1.5;
  word-break: break-word;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);

  &.pending {
    opacity: 0.6;
  }

  &.failed {
    border: 1px solid var(--kf-red);
  }
}

.is-out .bubble {
  background: var(--kf-primary);
  color: #fff;
}

/* 媒体消息（图片/视频/文件）：去掉气泡底色与阴影 */
.bubble:has(.media) {
  padding: 4px;
  background: transparent;
  box-shadow: none;

  &.failed {
    border: 1px solid var(--kf-red);
  }
}

.is-out .bubble:has(.media) {
  background: transparent;
}

.bubble-text {
  white-space: pre-wrap;
}

.bubble-error {
  margin-top: 4px;
  font-size: 12px;
  color: var(--kf-red);
}

/* ---------------- 未回复计时 ---------------- */
/* 位置：气泡下方（气泡外侧），与 .bubble-meta 时间戳同一列 */
.bubble-wait {
  display: inline-flex;
  align-self: flex-start;
  align-items: center;
  gap: 5px;
  margin-top: 4px;
  padding: 1px 9px;
  border-radius: 9px;
  background: rgba(255, 143, 31, 0.12);
  color: var(--kf-orange);
  font-size: 12px;
  line-height: 18px;
  font-weight: 500;
  white-space: nowrap;
  /* 等宽数字：每秒跳动时宽度不抖 */
  font-variant-numeric: tabular-nums;

  &__dot {
    flex-shrink: 0;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--kf-orange);
    /* 呼吸点：仅作「正在计时」的视觉提示，不影响数字本身 */
    animation: kf-wait-pulse 1.6s ease-in-out infinite;
  }

  &.is-compact {
    padding: 1px 7px;
    font-size: 11px;
    line-height: 16px;
    gap: 4px;
  }
}

@keyframes kf-wait-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.25;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bubble-wait__dot {
    animation: none;
  }
}

.system-pill {
  display: flex;
  justify-content: center;
  padding: 8px 0;

  span {
    display: inline-block;
    max-width: 70%;
    padding: 4px 12px;
    border-radius: 10px;
    background: rgba(0, 0, 0, 0.06);
    color: var(--kf-text-sub);
    font-size: 12px;
    line-height: 1.4;
    text-align: center;
  }
}
</style>
