<template>
  <aside class="customer-panel" :class="{ 'customer-panel--embedded': embedded }">
    <header class="panel-header">
      <span>客户资料</span>
      <el-button text size="small" :icon="Close" @click="emit('close')" />
    </header>

    <div v-loading="loading" class="panel-body kf-scroll">
      <div v-if="!customer" class="kf-empty">
        <span>暂无客户信息</span>
      </div>

      <template v-else>
        <div class="profile">
          <div class="profile-avatar">
            <img v-if="customer.avatar" :src="customer.avatar" alt="" />
            <span v-else>{{ (customer.nickname || '?').slice(0, 1) }}</span>
          </div>
          <div class="profile-name">{{ customer.nickname }}</div>
          <div class="profile-channel">
            <el-tag size="small" effect="light" :color="channelColor(customer.channel?.type)">
              {{ channelLabel(customer.channel?.type) }}
            </el-tag>
          </div>

          <div v-if="tags.length" class="profile-tags">
            <el-tag v-for="t in tags" :key="t" size="small" type="warning" effect="plain">
              {{ t }}
            </el-tag>
          </div>

          <ul class="profile-meta">
            <li v-if="customer.phone"><span>手机</span>{{ customer.phone }}</li>
            <li v-if="customer.city"><span>归属地</span>{{ customer.city }}</li>
            <li v-if="customer.ip"><span>IP</span>{{ customer.ip }}</li>
            <li v-if="customer.external_id"><span>ID</span>{{ customer.external_id }}</li>
            <li><span>订单</span>{{ customer.order_count ?? 0 }} 笔</li>
            <li><span>会话</span>{{ customer.conversation_count ?? 0 }} 次</li>
          </ul>
        </div>

        <div class="panel-section">
          <div class="kf-section-title">历史订单（{{ orders.length }}）</div>
          <div v-if="!orders.length" class="panel-empty">暂无订单</div>
          <div
            v-for="o in orders"
            :key="o.id"
            class="order-item"
            :class="{ disabled: !canSend }"
            @click="sendOrder(o)"
          >
            <div class="order-thumb">
              <img v-if="o.image" :src="o.image" alt="" />
              <span v-else>📦</span>
            </div>
            <div class="order-info">
              <div class="order-title kf-ellipsis">{{ o.title }}</div>
              <div class="order-sub">
                <span>{{ formatMoney(o.amount) }}</span>
                <span>{{ o.status }}</span>
              </div>
              <div class="order-no">{{ o.order_no }}</div>
            </div>
            <el-tooltip content="发送给客户" placement="left">
              <span class="send-icon">➤</span>
            </el-tooltip>
          </div>
        </div>
      </template>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Close } from '@element-plus/icons-vue'
import type { Customer, Order } from '@/types'
import { channelColor, channelLabel, formatMoney } from '@/utils/format'

const props = defineProps<{
  customer: Customer | null
  orders: Order[]
  loading?: boolean
  canSend?: boolean
  /** 嵌入抽屉时：去掉固定栏样式，自适应容器高度 */
  embedded?: boolean
}>()

const emit = defineEmits<{
  close: []
  'send-order': [order: Order]
}>()

const customer = computed(() => props.customer)
const orders = computed(() => props.orders)
const tags = computed(() => props.customer?.tags || [])

function sendOrder(order: Order): void {
  if (!props.canSend) return
  emit('send-order', order)
}
</script>

<style scoped lang="scss">
.customer-panel {
  display: flex;
  flex-direction: column;
  width: var(--kf-panel-width);
  min-width: var(--kf-panel-width);
  background: #fff;
  border-left: 1px solid var(--kf-border);
}

/* 嵌入抽屉（移动端）：去掉固定宽度与边框，自适应容器 */
.customer-panel--embedded {
  width: 100%;
  min-width: 0;
  height: 100%;
  border-left: none;
}

/* ---------------- 平板及以下：客户资料不再占用固定栏 ---------------- */
@media (max-width: 1200px) {
  .customer-panel {
    display: none;
  }

  /* 抽屉内的实例不受此规则影响 */
  .customer-panel--embedded {
    display: flex;
  }
}

/* ---------------- 移动端 ---------------- */
@media (max-width: 768px) {
  .panel-header {
    height: 44px;
  }

  .profile-avatar {
    width: 52px;
    height: 52px;
  }
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--kf-header-height);
  padding: 0 8px 0 14px;
  border-bottom: 1px solid var(--kf-border);
  font-size: 14px;
  font-weight: 600;
}

.panel-body {
  flex: 1;
}

.profile {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 18px 14px 14px;
  border-bottom: 1px solid var(--kf-border-light);
}

.profile-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: var(--kf-radius);
  background: var(--kf-bg-active);
  color: var(--kf-text-sub);
  font-size: 22px;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.profile-name {
  margin-top: 10px;
  font-size: 15px;
  font-weight: 600;
}

.profile-channel {
  margin-top: 6px;

  :deep(.el-tag) {
    color: #fff;
    border: none;
  }
}

.profile-tags {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  margin-top: 8px;
}

.profile-meta {
  width: 100%;
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  font-size: 13px;

  li {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 0;
    border-bottom: 1px dashed var(--kf-border-light);

    span {
      color: var(--kf-text-sub);
    }
  }
}

.panel-section {
  padding: 12px 14px;
}

.panel-empty {
  padding: 12px 0;
  color: var(--kf-text-weak);
  font-size: 12px;
  text-align: center;
}

.order-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);
  cursor: pointer;

  & + .order-item {
    margin-top: 8px;
  }

  &:hover {
    background: var(--kf-bg-hover);
  }

  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.order-thumb {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.order-info {
  flex: 1;
  min-width: 0;
}

.order-title {
  font-size: 13px;
}

.order-sub {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.order-no {
  margin-top: 2px;
  font-size: 11px;
  color: var(--kf-text-weak);
}

.send-icon {
  flex-shrink: 0;
  color: var(--kf-primary);
  font-size: 12px;
}
</style>
