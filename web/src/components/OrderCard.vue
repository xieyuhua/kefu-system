<template>
  <div class="order-card">
    <div class="order-thumb">
      <img v-if="order.image" :src="order.image" :alt="order.title" />
      <span v-else class="thumb-placeholder">📦</span>
    </div>
    <div class="order-body">
      <div class="order-title kf-clamp-2">{{ order.title }}</div>
      <div v-if="order.sku" class="order-sku kf-ellipsis">规格：{{ order.sku }}</div>
      <div class="order-meta">
        <span class="order-amount">{{ formatMoney(order.amount) }}</span>
        <span class="order-qty">×{{ order.quantity ?? 1 }}</span>
      </div>
      <div class="order-status-row">
        <span class="order-status">{{ order.status || '未知状态' }}</span>
      </div>
      <div class="order-no kf-ellipsis">订单号 {{ order.order_no }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { OrderCardPayload } from '@/types'
import { formatMoney } from '@/utils/format'

defineProps<{ order: OrderCardPayload }>()
</script>

<style scoped lang="scss">
.order-card {
  display: flex;
  gap: 10px;
  width: 260px;
  padding: 10px;
  background: #fff;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);
}

.order-thumb {
  flex-shrink: 0;
  width: 56px;
  height: 56px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg);
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
}

.thumb-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  font-size: 22px;
}

.order-body {
  flex: 1;
  min-width: 0;
}

.order-title {
  font-size: 13px;
  line-height: 1.4;
  color: var(--kf-text);
}

.order-sku {
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.order-meta {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-top: 4px;
}

.order-amount {
  font-size: 14px;
  font-weight: 600;
  color: #fa5151;
}

.order-qty {
  font-size: 12px;
  color: var(--kf-text-sub);
}

.order-status-row {
  margin-top: 4px;
  font-size: 12px;
}

.order-status {
  color: var(--kf-primary);
}

/* 订单号独占一行，超长省略 */
.order-no {
  margin-top: 3px;
  color: var(--kf-text-weak);
}
</style>
