<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { listProducts, type ProductQuery } from '@/shared/api/admin'
import { useIsMobile } from '@/shared/composables/useIsMobile'
import ThumbImage from '@/shared/components/ThumbImage.vue'
import type { Product } from '@/shared/types'

/** 窄屏（≤768px）改用卡片列表，避免宽表被裁切 */
const { isMobile } = useIsMobile()

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
        v-if="!isMobile"
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
              <ThumbImage :src="row.image" :size="40" :fallback="'图'" :alt="row.title" />
              <div class="goods__meta">
                <span class="goods__title kf-ellipsis">{{ row.title }}</span>
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

      <!-- 窄屏：卡片列表（商品图 / 名称 / 价格 / 库存可读） -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in list" :key="row.id" class="m-card">
          <div class="m-card__top">
            <ThumbImage :src="row.image" :size="44" :fallback="'图'" :alt="row.title" />
            <span class="m-card__title">{{ row.title }}</span>
            <span class="m-card__tags">
              <span class="price">{{ formatPrice(row.price) }}</span>
            </span>
          </div>

          <div class="m-card__sub">商品 ID {{ row.id }}</div>

          <div class="m-card__meta">
            <span>规格 {{ row.sku || '--' }}</span>
            <span>
              库存
              <b :class="{ 'low-stock': (row.stock ?? 0) <= 10 }">{{ row.stock ?? 0 }}</b>
            </span>
          </div>

          <div v-if="row.url" class="m-card__sub">{{ row.url }}</div>
        </div>

        <el-empty v-if="!loading && !list.length" description="暂无商品" :image-size="60" />
      </div>

      <div class="pager">
        <el-pagination
          background
          :small="isMobile"
          :pager-count="isMobile ? 5 : 7"
          :layout="isMobile ? 'prev, pager, next' : 'total, sizes, prev, pager, next, jumper'"
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
  min-width: 0;

  &__meta {
    display: flex;
    flex-direction: column;
    line-height: 1.3;
    min-width: 0;
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
