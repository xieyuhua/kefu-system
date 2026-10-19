<template>
  <div class="product-card">
    <div class="product-thumb">
      <img v-if="product.image" :src="product.image" :alt="product.title" />
      <span v-else class="thumb-placeholder">🛍</span>
    </div>
    <div class="product-body">
      <div class="product-title kf-clamp-2">{{ product.title }}</div>
      <div v-if="product.sku" class="product-sku kf-ellipsis">规格：{{ product.sku }}</div>
      <div class="product-meta">
        <span class="product-price">{{ formatMoney(product.price) }}</span>
        <span class="product-stock">库存 {{ product.stock ?? 0 }}</span>
      </div>
      <div class="product-sku-no">SKU {{ product.id }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ProductCardPayload } from '@/shared/types'
import { formatMoney } from '@/shared/utils/format'

defineProps<{ product: ProductCardPayload }>()
</script>

<style scoped lang="scss">
.product-card {
  display: flex;
  gap: 10px;
  width: 260px;
  padding: 10px;
  background: #fff;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);
}

.product-thumb {
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

.product-body {
  flex: 1;
  min-width: 0;
}

.product-title {
  font-size: 13px;
  line-height: 1.4;
  color: var(--kf-text);
}

.product-sku {
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.product-meta {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-top: 4px;
}

.product-price {
  font-size: 14px;
  font-weight: 600;
  color: #fa5151;
}

.product-stock {
  font-size: 12px;
  color: var(--kf-text-sub);
}

.product-sku-no {
  margin-top: 4px;
  font-size: 12px;
  color: var(--kf-text-weak);
}
</style>
