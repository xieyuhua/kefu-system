<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { listOrders, type OrderQuery } from '@/shared/api/admin'
import { useIsMobile } from '@/shared/composables/useIsMobile'
import ThumbImage from '@/shared/components/ThumbImage.vue'
import type { Order } from '@/shared/types'

/** 窄屏（≤768px）改用卡片列表，避免宽表被裁切 */
const { isMobile } = useIsMobile()

const loading = ref(false)
const list = ref<Order[]>([])
const total = ref(0)

const query = reactive<{ keyword: string; user_id: number | undefined; page: number; page_size: number }>({
  keyword: '',
  user_id: undefined,
  page: 1,
  page_size: 20,
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const params: OrderQuery = { page: query.page, page_size: query.page_size }
    const keyword = query.keyword.trim()
    if (keyword) params.keyword = keyword
    if (query.user_id) params.user_id = query.user_id

    const res = await listOrders(params)
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
  query.user_id = undefined
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

/** 订单状态 → 标签语义色（API 的 status 为中文字符串，此处做常见值映射） */
function statusType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  if (!status) return 'info'
  if (status.includes('完成')) return 'success'
  if (status.includes('待付款') || status.includes('待发货')) return 'warning'
  if (status.includes('退款') || status.includes('关闭') || status.includes('取消')) return 'danger'
  return 'info'
}

function formatAmount(value: number | undefined): string {
  return `¥${Number(value || 0).toFixed(2)}`
}

onMounted(load)
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          订单管理
          <span class="toolbar__count">共 {{ total }} 笔订单</span>
        </div>
        <div class="toolbar__actions">
          <el-input
            v-model="query.keyword"
            class="search-input"
            placeholder="搜索订单号 / 商品名"
            clearable
            @keyup.enter="search"
            @clear="search"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-input
            v-model.number="query.user_id"
            class="user-input"
            placeholder="按客户 ID 过滤"
            clearable
            @keyup.enter="search"
            @clear="search"
          />
          <el-button :icon="Search" type="primary" @click="search">搜索</el-button>
          <el-button :icon="Refresh" @click="reset">重置</el-button>
        </div>
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无订单"
      >
        <el-table-column prop="order_no" label="订单号" min-width="160" />
        <el-table-column label="商品" min-width="260">
          <template #default="{ row }">
            <div class="goods">
              <ThumbImage :src="row.image" :size="36" :fallback="'图'" :alt="row.title" />
              <span class="goods__title kf-ellipsis">{{ row.title }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="sku" label="规格" min-width="120" show-overflow-tooltip />
        <el-table-column label="数量" width="70" align="center">
          <template #default="{ row }">{{ row.quantity ?? 1 }}</template>
        </el-table-column>
        <el-table-column label="金额" width="110" align="right">
          <template #default="{ row }">
            <span class="amount">{{ formatAmount(row.amount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="statusType(row.status)">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="下单时间" min-width="160" />
        <el-table-column prop="paid_at" label="支付时间" min-width="160">
          <template #default="{ row }">
            <span>{{ row.paid_at || '--' }}</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：卡片列表（订单号 / 商品 / 金额 / 状态一屏可读） -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in list" :key="row.id" class="m-card">
          <div class="m-card__top">
            <ThumbImage :src="row.image" :size="40" :fallback="'图'" :alt="row.title" />
            <span class="m-card__title">{{ row.title }}</span>
            <span class="m-card__tags">
              <el-tag size="small" :type="statusType(row.status)">{{ row.status }}</el-tag>
            </span>
          </div>

          <div class="m-card__sub">{{ row.order_no }}</div>

          <div class="m-card__meta">
            <span>规格 {{ row.sku || '--' }}</span>
            <span>数量 {{ row.quantity ?? 1 }}</span>
            <span class="amount">{{ formatAmount(row.amount) }}</span>
          </div>

          <div class="m-card__meta">
            <span>下单 {{ row.created_at }}</span>
            <span>支付 {{ row.paid_at || '--' }}</span>
          </div>
        </div>

        <el-empty v-if="!loading && !list.length" description="暂无订单" :image-size="60" />
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
  margin-bottom: 16px;

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
  width: 220px;
}

.user-input {
  width: 160px;
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

  &__title {
    color: var(--kf-text, #191919);
    min-width: 0;
  }
}

.amount {
  color: #191919;
  font-weight: 600;
}
</style>
