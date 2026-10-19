<template>
  <el-dialog v-model="visible" title="发送商品卡片" width="560px" append-to-body @open="onOpen">
    <el-input
      v-model="keyword"
      placeholder="搜索商品名"
      clearable
      :prefix-icon="Search"
      class="picker-search"
      @keyup.enter="reload"
    />

    <div v-loading="loading" class="picker-list kf-scroll">
      <div
        v-for="p in products"
        :key="p.id"
        class="picker-item"
        :class="{ active: selected?.id === p.id }"
        @click="selected = p"
      >
        <div class="item-thumb">
          <img v-if="p.image" :src="p.image" alt="" />
          <span v-else>🛍</span>
        </div>
        <div class="item-body">
          <div class="item-title kf-ellipsis">{{ p.title }}</div>
          <div class="item-sub">
            <span class="price">{{ formatMoney(p.price) }}</span>
            <span>库存 {{ p.stock }}</span>
          </div>
          <div class="item-no">SKU {{ p.id }} · {{ p.sku }}</div>
        </div>
      </div>
      <el-empty v-if="!loading && !products.length" description="暂无商品" :image-size="64" />
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
import { listProducts } from '@/shared/api/products'
import type { Product } from '@/shared/types'
import { formatMoney } from '@/shared/utils/format'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  pick: [product: Product]
}>()

const visible = ref(props.modelValue)
const products = ref<Product[]>([])
const selected = ref<Product | null>(null)
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
    const res = await listProducts({ keyword: keyword.value || undefined, page: 1, page_size: 50 })
    products.value = res?.list || []
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

.price {
  color: #fa5151;
  font-weight: 600;
}

.item-no {
  margin-top: 2px;
  font-size: 11px;
  color: var(--kf-text-weak);
}
</style>
