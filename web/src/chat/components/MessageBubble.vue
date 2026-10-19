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

      <div v-if="message.failed" class="bubble-error">发送失败</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Message, OrderCardPayload, ProductCardPayload } from '@/shared/types'
import { formatMessageTime, parseCardContent } from '@/shared/utils/format'
import MediaBubble, { type MediaPayload } from '@/shared/components/MediaBubble.vue'
import OrderCard from '@/shared/components/OrderCard.vue'
import ProductCard from '@/shared/components/ProductCard.vue'

const props = defineProps<{
  message: Message
  /** 搜索定位高亮：传入当前需要高亮的消息 id（变化即触发闪烁） */
  flashKey?: number
}>()

const isSystem = computed(
  () => props.message.direction === 'system' || props.message.type === 'system',
)
const isOut = computed(() => props.message.direction === 'out')
const text = computed(() => props.message.content || '')

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
