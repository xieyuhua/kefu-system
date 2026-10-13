<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { listProducts, type ProductQuery } from '../../api/admin'
import type { Product } from '../../types'

const loading = ref(false)
const list = ref<Product[]>([])
const total = ref(0)

const query = reactive<{ keyword: string; page: number; page_size: number }>({
  keyword: '',
  page: 1,
  page_size: 20,
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const params: ProductQuery = { page: query.page, page_size: query.page_size }
    const keyword = query.keyword.trim()
    if (keyword) params.keyword = keyword

    const res = await listProducts(params)
    list.value = res.list || []
    total.value = res.total || 0
  } catch {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function search(): void {
  query.page = 1
  void load()
}

function reset(): void {
  query.keyword = ''
  query.page = 1
  void load()
}

function onPageChange(page: number): void {
  query.page = page
  void load()
}

function onSizeChange(size: number): void {
  query.page_size = size
  query.page = 1
  void load()
}

function formatPrice(value: number | undefined): string {
  return `¥${Number(value || 0).toFixed(2)}`
}

onMounted(load)
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          商品管理
          <span class="toolbar__count">共 {{ total }} 个商品</span>
        </div>
        <div class="toolbar__actions">
          <el-input
            v-model="query.keyword"
            class="search-input"
            placeholder="搜索商品名称 / 商品 ID"
            clearable
            @keyup.enter="search"
            @clear="search"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-button :icon="Search" type="primary" @click="search">搜索</el-button>
          <el-button :icon="Refresh" @click="reset">重置</el-button>
        </div>
      </div>

      <div class="tip">这些商品用于客服在工作台向客户发送「商品卡片」。</div>

      <el-table
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无商品"
      >
        <el-table-column label="商品" min-width="260">
          <template #default="{ row }">
            <div class="goods">
              <img v-if="row.image" class="goods__img" :src="row.image" :alt="row.title" />
              <div class="goods__meta">
                <span class="goods__title">{{ row.title }}</span>
                <span class="goods__id">{{ row.id }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="sku" label="规格" min-width="120" show-overflow-tooltip />
        <el-table-column label="价格" width="110" align="right">
          <template #default="{ row }">
            <span class="price">{{ formatPrice(row.price) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="库存" width="90" align="center">
          <template #default="{ row }">
            <span :class="{ 'low-stock': (row.stock ?? 0) <= 10 }">{{ row.stock ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column label="商品链接" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="muted">{{ row.url || '--' }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next, jumper"
          :current-page="query.page"
          :page-size="query.page_size"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
@use './shared.scss';

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;

  &__title {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
    white-space: nowrap;
  }

  &__count {
    margin-left: 8px;
    font-size: 12px;
    font-weight: 400;
    color: #7f7f7f;
  }

  &__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
}

.search-input {
  width: 240px;
}

.tip {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: #f5f5f5;
  border-radius: 4px;
  font-size: 12px;
  color: #7f7f7f;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.goods {
  display: flex;
  align-items: center;
  gap: 8px;

  &__img {
    width: 40px;
    height: 40px;
    border-radius: 4px;
    object-fit: cover;
    background: #f5f5f5;
    flex: 0 0 40px;
  }

  &__meta {
    display: flex;
    flex-direction: column;
    line-height: 1.3;
  }

  &__title {
    color: #191919;
  }

  &__id {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.price {
  color: #191919;
  font-weight: 600;
}

.low-stock {
  color: #e6162d;
  font-weight: 600;
}

.muted {
  color: #7f7f7f;
}
</style>
