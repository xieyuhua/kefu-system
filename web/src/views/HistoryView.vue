<template>
  <div class="history-page">
    <aside class="history-sidebar">
      <div class="sidebar-head">
        <span class="sidebar-title">历史档案</span>
        <el-button text size="small" :icon="Back" title="返回工作台" @click="goWorkbench" />
      </div>

      <div class="sidebar-search">
        <el-input
          v-model="keyword"
          placeholder="搜索昵称 / 手机"
          clearable
          :prefix-icon="Search"
          @keyup.enter="reload"
          @clear="reload"
        />
        <el-select v-model="channelId" placeholder="全部渠道" clearable class="channel-select">
          <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
      </div>

      <div v-loading="loading" class="customer-list kf-scroll">
        <div
          v-for="u in users"
          :key="u.id"
          class="customer-item"
          :class="{ active: currentId === u.id }"
          @click="open(u)"
        >
          <div class="c-avatar">
            <img v-if="u.avatar" :src="u.avatar" alt="" />
            <span v-else>{{ (u.nickname || '?').slice(0, 1) }}</span>
          </div>
          <div class="c-main">
            <div class="c-line">
              <span class="c-name kf-ellipsis">{{ u.nickname }}</span>
              <el-tag size="small" effect="light" :color="channelColor(u.channel?.type)">
                {{ channelLabel(u.channel?.type) }}
              </el-tag>
            </div>
            <div class="c-line c-meta">
              <span>订单 {{ u.order_count ?? 0 }}</span>
              <span>会话 {{ u.conversation_count ?? 0 }}</span>
            </div>
          </div>
        </div>
        <el-empty v-if="!loading && !users.length" description="暂无客户" :image-size="64" />

        <footer class="sidebar-foot">
          <el-pagination
            small
            layout="prev, pager, next"
            :current-page="page"
            :page-size="pageSize"
            :total="total"
            @current-change="onPage"
          />
        </footer>
      </div>
    </aside>

    <main class="history-main">
      <div class="kf-empty">
        <span class="empty-icon">📁</span>
        <span>点击左侧客户查看历史订单与历史会话</span>
      </div>
    </main>

    <el-drawer
      v-model="drawer"
      :title="detail?.user?.nickname ? `${detail.user.nickname} 的历史档案` : '客户档案'"
      size="420px"
    >
      <div v-loading="detailLoading" class="drawer-body">
        <template v-if="detail">
          <div class="drawer-profile">
            <div class="p-avatar">
              <img v-if="detail.user?.avatar" :src="detail.user.avatar" alt="" />
              <span v-else>{{ (detail.user?.nickname || '?').slice(0, 1) }}</span>
            </div>
            <div class="p-info">
              <div class="p-name">{{ detail.user?.nickname }}</div>
              <div class="p-row">渠道：{{ detail.user?.channel?.name || '未知' }}</div>
              <div class="p-row">手机：{{ detail.user?.phone || '未提供' }}</div>
              <div class="p-row">注册：{{ detail.user?.created_at || '-' }}</div>
            </div>
          </div>

          <div v-if="detail.user?.tags?.length" class="p-tags">
            <el-tag v-for="t in detail.user.tags" :key="t" size="small" type="warning" effect="plain">
              {{ t }}
            </el-tag>
          </div>

          <div class="drawer-section">
            <div class="kf-section-title">历史订单（{{ detail.orders?.length || 0 }}）</div>
            <div v-if="!detail.orders?.length" class="panel-empty">暂无订单</div>
            <div v-for="o in detail.orders || []" :key="o.id" class="d-order">
              <div class="d-thumb">
                <img v-if="o.image" :src="o.image" alt="" />
                <span v-else>📦</span>
              </div>
              <div class="d-body">
                <div class="d-title kf-ellipsis">{{ o.title }}</div>
                <div class="d-sub">
                  <span class="price">{{ formatMoney(o.amount) }}</span>
                  <span>×{{ o.quantity ?? 1 }}</span>
                  <span>{{ o.status }}</span>
                </div>
                <div class="d-no">{{ o.order_no }} · {{ o.created_at }}</div>
              </div>
            </div>
          </div>

          <div class="drawer-section">
            <div class="kf-section-title">历史会话（{{ detail.conversations?.length || 0 }}）</div>
            <div v-if="!detail.conversations?.length" class="panel-empty">暂无会话</div>
            <div v-for="c in detail.conversations || []" :key="c.id" class="d-conv">
              <div class="d-conv-head">
                <el-tag size="small" effect="light" :color="channelColor(c.channel?.type)">
                  {{ channelLabel(c.channel?.type) }}
                </el-tag>
                <el-tag size="small" effect="plain" :type="c.status === 'open' ? 'success' : 'info'">
                  {{ statusLabel(c.status) }}
                </el-tag>
                <span class="d-conv-time">{{ c.created_at }}</span>
              </div>
              <div class="d-conv-summary">
                {{ c.summary || '（无摘要）' }} · {{ c.message_count ?? 0 }} 条消息
              </div>
              <div class="d-conv-foot">
                接待：{{ c.agent?.name || '未分配' }}
                <span v-if="c.closed_at"> · 结束于 {{ c.closed_at }}</span>
              </div>
            </div>
          </div>
        </template>

        <div v-else-if="!detailLoading" class="kf-empty">
          <span>暂无数据</span>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Back, Search } from '@element-plus/icons-vue'
import { getCustomer, listCustomers } from '@/api/users'
import { listChannels } from '@/api/channels'
import type { Channel, Customer, CustomerDetailResult } from '@/types'
import { channelColor, channelLabel, formatMoney, statusLabel } from '@/utils/format'

const router = useRouter()

const users = ref<Customer[]>([])
const channels = ref<Channel[]>([])
const keyword = ref('')
const channelId = ref<number | undefined>(undefined)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const currentId = ref(0)

const drawer = ref(false)
const detail = ref<CustomerDetailResult | null>(null)
const detailLoading = ref(false)

async function reload(): Promise<void> {
  loading.value = true
  try {
    const res = await listCustomers({
      keyword: keyword.value || undefined,
      channel_id: channelId.value,
      page: page.value,
      page_size: pageSize.value,
    })
    users.value = res?.list || []
    total.value = res?.total || 0
  } catch {
    users.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function onPage(p: number): void {
  page.value = p
  void reload()
}

async function open(user: Customer): Promise<void> {
  currentId.value = user.id
  drawer.value = true
  detail.value = null
  detailLoading.value = true
  try {
    detail.value = await getCustomer(user.id)
  } catch {
    detail.value = null
  } finally {
    detailLoading.value = false
  }
}

function goWorkbench(): void {
  void router.push('/chat/')
}

onMounted(() => {
  void reload()
  void listChannels()
    .then((list) => {
      channels.value = list || []
    })
    .catch(() => {
      channels.value = []
    })
})
</script>

<style scoped lang="scss">
.history-page {
  display: flex;
  height: 100%;
  background: var(--kf-bg);
}

.history-sidebar {
  display: flex;
  flex-direction: column;
  width: var(--kf-list-width);
  min-width: 260px;
  background: #fff;
  border-right: 1px solid var(--kf-border);
}

.sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--kf-header-height);
  padding: 0 8px 0 14px;
  border-bottom: 1px solid var(--kf-border);
}

.sidebar-title {
  font-size: 15px;
  font-weight: 600;
}

.sidebar-search {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--kf-border-light);
}

.customer-list {
  flex: 1;
}

.customer-item {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  cursor: pointer;
  border-bottom: 1px solid var(--kf-border-light);

  &:hover {
    background: var(--kf-bg-hover);
  }

  &.active {
    background: var(--kf-bg-active);
  }
}

.c-avatar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg-active);
  color: var(--kf-text-sub);
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.c-main {
  flex: 1;
  min-width: 0;
}

.c-line {
  display: flex;
  align-items: center;
  gap: 8px;

  & + .c-line {
    margin-top: 4px;
  }

  :deep(.el-tag) {
    color: #fff;
    border: none;
  }
}

.c-name {
  font-size: 14px;
  font-weight: 500;
}

.c-meta {
  font-size: 12px;
  color: var(--kf-text-sub);
  gap: 12px;
}

.sidebar-foot {
  display: flex;
  justify-content: center;
  padding: 8px 0;
  border-top: 1px solid var(--kf-border-light);
}

.history-main {
  flex: 1;
}

.empty-icon {
  font-size: 40px;
  opacity: 0.5;
}

.drawer-body {
  height: 100%;
}

.drawer-profile {
  display: flex;
  gap: 12px;
}

.p-avatar {
  flex-shrink: 0;
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

.p-info {
  min-width: 0;
}

.p-name {
  font-size: 15px;
  font-weight: 600;
}

.p-row {
  margin-top: 3px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.p-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 10px;
}

.drawer-section {
  margin-top: 18px;
}

.panel-empty {
  padding: 12px 0;
  color: var(--kf-text-weak);
  font-size: 12px;
  text-align: center;
}

.d-order,
.d-conv {
  display: flex;
  gap: 10px;
  padding: 8px;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);

  & + .d-order,
  & + .d-conv {
    margin-top: 8px;
  }
}

.d-thumb {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg);
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.d-body {
  flex: 1;
  min-width: 0;
}

.d-title {
  font-size: 13px;
}

.d-sub {
  display: flex;
  gap: 8px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.price {
  color: #fa5151;
  font-weight: 600;
}

.d-no {
  margin-top: 2px;
  font-size: 11px;
  color: var(--kf-text-weak);
}

.d-conv {
  flex-direction: column;
  gap: 6px;
}

.d-conv-head {
  display: flex;
  align-items: center;
  gap: 6px;

  :deep(.el-tag) {
    color: #fff;
    border: none;
  }
}

.d-conv-time {
  margin-left: auto;
  font-size: 11px;
  color: var(--kf-text-weak);
}

.d-conv-summary {
  font-size: 13px;
}

.d-conv-foot {
  font-size: 12px;
  color: var(--kf-text-sub);
}
</style>
