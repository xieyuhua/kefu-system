<template>
  <el-dialog
    v-model="visible"
    title="发送订单卡片"
    width="560px"
    append-to-body
    @open="onOpen"
  >
    <el-input
      v-model="keyword"
      placeholder="搜索订单号 / 商品名"
      clearable
      :prefix-icon="Search"
      class="picker-search"
      @keyup.enter="reload"
    />

    <div v-loading="loading" class="picker-list kf-scroll">
      <div
        v-for="o in orders"
        :key="o.id"
        class="picker-item"
        :class="{ active: selected?.id === o.id }"
        @click="selected = o"
      >
        <div class="item-thumb">
          <img v-if="o.image" :src="o.image" alt="" />
          <span v-else>📦</span>
        </div>
        <div class="item-body">
          <div class="item-title kf-ellipsis">{{ o.title }}</div>
          <div class="item-sub">
            <span>{{ formatMoney(o.amount) }}</span>
            <span>{{ o.status }}</span>
          </div>
          <div class="item-no">{{ o.order_no }}</div>
        </div>
      </div>
      <el-empty v-if="!loading && !orders.length" description="暂无订单" :image-size="64" />
    </div>

    <template #footer>
      <el-button size="small" @click="visible = false">取消</el-button>
      <el-button size="small" type="primary" :disabled="!selected" @click="confirm">
        发送
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { listOrders } from '@/api/orders'
import type { Order } from '@/types'
import { formatMoney } from '@/utils/format'

const props = defineProps<{ modelValue: boolean; userId?: number }>()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  pick: [order: Order]
}>()

const visible = ref(props.modelValue)
const orders = ref<Order[]>([])
const selected = ref<Order | null>(null)
const loading = ref(false)
const keyword = ref('')

watch(
  () => props.modelValue,
  (v) => {
    visible.value = v
  },
)
watch(visible, (v) => emit('update:modelValue', v))

async function reload(): Promise<void> {
  loading.value = true
  try {
    const res = await listOrders({
      user_id: props.userId,
      keyword: keyword.value || undefined,
      page: 1,
      page_size: 50,
    })
    orders.value = res?.list || []
  } finally {
    loading.value = false
  }
}

function onOpen(): void {
  selected.value = null
  void reload()
}

function confirm(): void {
  if (!selected.value) return
  emit('pick', selected.value)
  visible.value = false
}
</script>

<style scoped lang="scss">
.picker-search {
  margin-bottom: 10px;
}

.picker-list {
  max-height: 320px;
}

.picker-item {
  display: flex;
  gap: 10px;
  padding: 8px;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);
  cursor: pointer;

  & + .picker-item {
    margin-top: 8px;
  }

  &.active {
    border-color: var(--kf-primary);
    background: var(--kf-primary-light);
  }
}

.item-thumb {
  flex-shrink: 0;
  width: 48px;
  height: 48px;
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

.item-body {
  flex: 1;
  min-width: 0;
}

.item-title {
  font-size: 13px;
}

.item-sub {
  display: flex;
  justify-content: space-between;
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.item-no {
  margin-top: 2px;
  font-size: 11px;
  color: var(--kf-text-weak);
}
</style>
