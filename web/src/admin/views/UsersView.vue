<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import {
  channelTypeMeta,
  getUserDetail,
  listChannels,
  listUsers,
  type UserQuery,
} from '@/shared/api/admin'
import type { Channel, Customer, CustomerDetailResult } from '@/shared/types'
import ThumbImage from '@/shared/components/ThumbImage.vue'

const loading = ref(false)
const detailLoading = ref(false)
const list = ref<Customer[]>([])
const total = ref(0)
const channels = ref<Channel[]>([])

const drawerVisible = ref(false)
const detail = ref<CustomerDetailResult | null>(null)

const query = reactive<{ keyword: string; channel_id: number | undefined; page: number; page_size: number }>({
  keyword: '',
  channel_id: undefined,
  page: 1,
  page_size: 20,
})

async function loadChannels(): Promise<void> {
  try {
    channels.value = await listChannels()
  } catch {
    channels.value = []
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const params: UserQuery = { page: query.page, page_size: query.page_size }
    const keyword = query.keyword.trim()
    if (keyword) params.keyword = keyword
    if (query.channel_id) params.channel_id = query.channel_id

    const res = await listUsers(params)
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
  query.channel_id = undefined
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

async function openDetail(row: Customer): Promise<void> {
  drawerVisible.value = true
  detail.value = null
  detailLoading.value = true
  try {
    detail.value = await getUserDetail(row.id)
  } catch {
    drawerVisible.value = false
    ElMessage.error('加载客户档案失败')
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => {
  void loadChannels()
  void load()
})
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          客户管理
          <span class="toolbar__count">共 {{ total }} 位客户</span>
        </div>
        <div class="toolbar__actions">
          <el-input
            v-model="query.keyword"
            class="search-input"
            placeholder="搜索昵称 / 手机号"
            clearable
            @keyup.enter="search"
            @clear="search"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-select
            v-model="query.channel_id"
            class="channel-select"
            placeholder="全部渠道"
            clearable
            @change="search"
          >
            <el-option v-for="ch in channels" :key="ch.id" :label="ch.name" :value="ch.id" />
          </el-select>
          <el-button :icon="Search" type="primary" @click="search">搜索</el-button>
          <el-button :icon="Refresh" @click="reset">重置</el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无客户"
      >
        <el-table-column label="客户" min-width="200">
          <template #default="{ row }">
            <div class="customer">
              <ThumbImage
                :src="row.avatar"
                :size="36"
                round
                kind="avatar"
                :fallback="(row.nickname || '?').slice(0, 1)"
                :alt="row.nickname"
              />
              <div class="customer__meta">
                <span class="customer__name kf-ellipsis">{{ row.nickname }}</span>
                <span class="customer__id">{{ row.external_id }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="手机号" min-width="130" />
        <el-table-column label="来源渠道" min-width="170">
          <template #default="{ row }">
            <span v-if="row.channel" class="channel-tag">
              <span
                class="channel-tag__icon"
                :style="{ color: channelTypeMeta(row.channel.type).color }"
              >{{ channelTypeMeta(row.channel.type).icon }}</span>
              <span class="channel-tag__label">{{ row.channel.name }}</span>
            </span>
            <span v-else class="muted">--</span>
          </template>
        </el-table-column>
        <el-table-column label="订单数" width="90" align="center">
          <template #default="{ row }">{{ row.order_count ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="会话数" width="90" align="center">
          <template #default="{ row }">{{ row.conversation_count ?? 0 }}</template>
        </el-table-column>
        <el-table-column prop="last_seen_at" label="最近来访" min-width="160" />
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
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

    <el-drawer v-model="drawerVisible" title="客户档案" size="620px">
      <div v-loading="detailLoading">
        <template v-if="detail">
          <div class="profile">
            <ThumbImage
              :src="detail.user.avatar"
              :size="56"
              round
              kind="avatar"
              :fallback="(detail.user.nickname || '?').slice(0, 1)"
              :alt="detail.user.nickname"
            />
            <div class="profile__meta">
              <div class="profile__name">
                {{ detail.user.nickname }}
                <el-tag
                  v-for="tag in detail.user.tags || []"
                  :key="tag"
                  size="small"
                  type="success"
                  effect="plain"
                >{{ tag }}</el-tag>
              </div>
              <div class="profile__sub">
                手机号 {{ detail.user.phone || '--' }} · 首次来访 {{ detail.user.created_at }}
              </div>
            </div>
          </div>

          <h3 class="section-title">历史订单（{{ detail.orders?.length || 0 }}）</h3>
          <el-table
            v-if="detail.orders && detail.orders.length"
            :data="detail.orders"
            size="small"
            border
          >
            <el-table-column prop="order_no" label="订单号" min-width="150" />
            <el-table-column label="商品" min-width="200">
              <template #default="{ row }">
                <div class="order-cell">
                  <ThumbImage :src="row.image" :size="32" :fallback="'图'" :alt="row.title" />
                  <span class="kf-ellipsis">{{ row.title }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="90" />
            <el-table-column label="金额" width="100" align="right">
              <template #default="{ row }">¥{{ Number(row.amount).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="created_at" label="下单时间" min-width="150" />
          </el-table>
          <el-empty v-else description="暂无历史订单" :image-size="60" />

          <h3 class="section-title">历史会话（{{ detail.conversations?.length || 0 }}）</h3>
          <el-table
            v-if="detail.conversations && detail.conversations.length"
            :data="detail.conversations"
            size="small"
            border
          >
            <el-table-column label="渠道" min-width="150">
              <template #default="{ row }">
                <span v-if="row.channel" class="channel-tag">
                  <span
                    class="channel-tag__icon"
                    :style="{ color: channelTypeMeta(row.channel.type).color }"
                  >{{ channelTypeMeta(row.channel.type).icon }}</span>
                  <span class="channel-tag__label">{{ row.channel.name }}</span>
                </span>
                <span v-else class="muted">--</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag
                  size="small"
                  :type="row.status === 'open' ? 'success' : row.status === 'pending' ? 'warning' : 'info'"
                >
                  {{ row.status === 'open' ? '进行中' : row.status === 'pending' ? '待接待' : '已结束' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="接待客服" width="100">
              <template #default="{ row }">{{ row.agent?.name || '--' }}</template>
            </el-table-column>
            <el-table-column label="消息数" width="80" align="center">
              <template #default="{ row }">{{ row.message_count ?? 0 }}</template>
            </el-table-column>
            <el-table-column prop="summary" label="摘要" min-width="150" show-overflow-tooltip />
            <el-table-column prop="created_at" label="发起时间" min-width="150" />
          </el-table>
          <el-empty v-else description="暂无历史会话" :image-size="60" />
        </template>
      </div>
    </el-drawer>
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

.channel-select {
  width: 180px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.customer {
  display: flex;
  align-items: center;
  gap: 8px;

  &__meta {
    display: flex;
    flex-direction: column;
    line-height: 1.3;
  }

  &__name {
    color: #191919;
  }

  &__id {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.muted {
  color: #7f7f7f;
}

.profile {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #e5e5e5;

  &__name {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__sub {
    margin-top: 4px;
    font-size: 12px;
    color: #7f7f7f;
  }
}

.section-title {
  margin: 20px 0 10px;
  font-size: 14px;
  font-weight: 600;
  color: #191919;
}

.order-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
</style>
