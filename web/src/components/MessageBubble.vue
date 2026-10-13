<template>
  <!-- 系统通知：居中灰色 pill -->
  <div v-if="isSystem" class="system-pill">
    <span>{{ text }}</span>
  </div>

  <div v-else class="bubble-row" :class="isOut ? 'is-out' : 'is-in'">
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

        <!-- 图片 -->
        <template v-else-if="message.type === 'image'">
          <img class="bubble-image" :src="text" alt="图片" />
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
import { computed } from 'vue'
import type { Message, OrderCardPayload, ProductCardPayload } from '@/types'
import { formatMessageTime, parseCardContent } from '@/utils/format'
import OrderCard from './OrderCard.vue'
import ProductCard from './ProductCard.vue'

const props = defineProps<{ message: Message }>()

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

const avatar = computed(() => props.message.sender?.avatar || '')

const fallbackName = computed(() => {
  if (props.message.sender?.name) return props.message.sender.name
  return isOut.value ? '我' : '客户'
})
</script>

<style scoped lang="scss">
.bubble-row {
  display: flex;
  gap: 8px;
  padding: 6px 0;

  &.is-out {
    flex-direction: row-reverse;
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

.bubble-text {
  white-space: pre-wrap;
}

.bubble-image {
  display: block;
  max-width: 180px;
  max-height: 200px;
  border-radius: var(--kf-radius-sm);
  cursor: zoom-in;
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
