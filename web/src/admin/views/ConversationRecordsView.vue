<script setup lang="ts">
/**
 * 对话记录查询（管理后台）
 * 全局检索历史会话：支持按客户昵称 / 消息内容 / 渠道 / 客服 / 日期范围 / 状态筛选，
 * 点击可查看该会话的完整聊天记录。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import { listChannels as fetchChannels, listAgents, type Agent } from '@/shared/api/admin'
import { fetchConversationRecords, fetchRecordMessages, type ConversationRecord } from '@/shared/api/records'
import { useIsMobile } from '@/shared/composables/useIsMobile'
import { formatMessageTime, parseCardContent } from '@/shared/utils/format'
import ThumbImage from '@/shared/components/ThumbImage.vue'
import type { Channel } from '@/shared/types'

/** 窄屏（≤768px）切换为卡片列表；复用共享断点组合式函数，不重复监听 resize */
const { isMobile } = useIsMobile()

const loading = ref(false)
const list = ref<ConversationRecord[]>([])
const total = ref(0)
const channels = ref<Channel[]>([])
const agents = ref<Agent[]>([])

const query = reactive({
  keyword: '',
  channel_id: 0,
  status: '',
  agent_id: 0,
  range: [] as string[],
  page: 1,
  page_size: 20,
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await fetchConversationRecords({
      keyword: query.keyword || undefined,
      channel_id: query.channel_id || undefined,
      status: query.status || undefined,
      agent_id: query.agent_id || undefined,
      start: query.range[0],
      end: query.range[1],
      page: query.page,
      page_size: query.page_size,
    })
    list.value = res.list || []
    total.value = res.total || 0
  } catch {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  query.page = 1
  void load()
}

function onReset(): void {
  query.keyword = ''
  query.channel_id = 0
  query.status = ''
  query.agent_id = 0
  query.range = []
  query.page = 1
  void load()
}

function onPage(p: number): void {
  query.page = p
  void load()
}

/* ---------------- 查看完整聊天记录 ---------------- */
const drawerVisible = ref(false)
const detailLoading = ref(false)
const detail = ref<ConversationRecord | null>(null)
const messages = ref<Record<string, unknown>[]>([])

const currentRecord = computed(() => detail.value)

async function openRecord(row: ConversationRecord): Promise<void> {
  drawerVisible.value = true
  detailLoading.value = true
  detail.value = row
  messages.value = []
  try {
    const list = await fetchRecordMessages(row.id)
    messages.value = list as unknown as Record<string, unknown>[]
  } catch {
    ElMessage.error('聊天记录加载失败')
  } finally {
    detailLoading.value = false
  }
}

/** 消息内容预览：卡片类消息展示摘要，纯文本直接展示 */
const msgText = (m: Record<string, unknown>): string => {
  const type = String(m.type || 'text')
  const content = String(m.content || '')
  if (type === 'order') {
    const p = parseCardContent<{ title?: string }>(content)
    return `📦 订单：${p?.title || '订单卡片'}`
  }
  if (type === 'product') {
    const p = parseCardContent<{ title?: string }>(content)
    return `🛍 商品：${p?.title || '商品卡片'}`
  }
  if (type === 'image') return '🖼 [图片]'
  if (type === 'video') return '🎬 [视频]'
  if (type === 'file') return '📎 [文件]'
  return content
}

function senderName(m: Record<string, unknown>): string {
  const s = m.sender as { name?: string; role?: string } | null
  if (!s) return ''
  if (s.role === 'agent') return s.name || '客服'
  if (s.role === 'user') return s.name || '客户'
  return '系统'
}

function statusTag(status: string): 'success' | 'warning' | 'info' {
  if (status === 'open') return 'success'
  if (status === 'pending') return 'warning'
  return 'info'
}

function statusLabel(status: string): string {
  return status === 'open' ? '进行中' : status === 'pending' ? '未接待' : '已结束'
}

onMounted(async () => {
  void load()
  try {
    channels.value = await fetchChannels()
  } catch {
    channels.value = []
  }
  try {
    agents.value = await listAgents()
  } catch {
    agents.value = []
  }
})
</script>

<template>
  <div class="admin">
    <!-- 查询条件 -->
    <div class="admin-card">
      <div class="filters" :class="{ 'filters--mobile': isMobile }">
        <el-input
          v-model="query.keyword"
          placeholder="搜索客户昵称 / 消息内容"
          clearable
          class="filters__input"
          @keyup.enter="onSearch"
          @clear="onSearch"
        />
        <el-select v-model="query.channel_id" placeholder="全部渠道" clearable class="filters__sel">
          <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
        <el-select v-model="query.agent_id" placeholder="全部客服" clearable class="filters__sel">
          <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
        </el-select>
        <el-select v-model="query.status" placeholder="全部状态" clearable class="filters__sel">
          <el-option label="进行中" value="open" />
          <el-option label="未接待" value="pending" />
          <el-option label="已结束" value="closed" />
        </el-select>
        <el-date-picker
          v-model="query.range"
          type="daterange"
          value-format="YYYY-MM-DD"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          class="filters__date"
        />
        <div class="filters__actions">
          <el-button type="primary" :icon="Search" @click="onSearch">查询</el-button>
          <el-button :icon="Refresh" @click="onReset">重置</el-button>
        </div>
      </div>

      <el-alert type="info" :closable="false" class="tip">
        可按<b>客户昵称</b>或<b>聊天内容关键词</b>检索历史会话记录（含已归档会话）。
      </el-alert>

      <!-- 宽屏：表格 -->
      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无对话记录"
      >
        <el-table-column label="客户" min-width="180">
          <template #default="{ row }">
            <span class="customer">
              <ThumbImage :src="row.customer_avatar" :size="30" round kind="avatar" :fallback="(row.customer_name || '?').slice(0, 1)" />
              <span class="customer__name">{{ row.customer_name || '匿名客户' }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="渠道" min-width="140">
          <template #default="{ row }">{{ row.channel_name || '-' }}</template>
        </el-table-column>
        <el-table-column label="接待客服" min-width="110">
          <template #default="{ row }">{{ row.agent_name }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
            <el-tag v-if="row.archived" size="small" type="info" effect="plain">归档</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="对话概览" min-width="280">
          <template #default="{ row }">
            <div class="preview">
              <span class="preview__line">
                <i class="preview__tag">首</i>{{ row.first_message || '—' }}
              </span>
              <span class="preview__line preview__line--last">
                <i class="preview__tag preview__tag--last">末</i>{{ row.last_message || '—' }}
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="消息数" width="90" align="center">
          <template #default="{ row }">{{ row.message_count }}</template>
        </el-table-column>
        <el-table-column prop="last_message_at" label="最后对话" min-width="160" />
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openRecord(row)">查看记录</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：卡片列表（避免表格横向挤压） -->
      <div v-else v-loading="loading" class="record-list">
        <button
          v-for="row in list"
          :key="row.id"
          type="button"
          class="record-card"
          @click="openRecord(row)"
        >
          <div class="record-card__top">
            <ThumbImage
              :src="row.customer_avatar"
              :size="34"
              round
              kind="avatar"
              :fallback="(row.customer_name || '?').slice(0, 1)"
            />
            <span class="record-card__name">{{ row.customer_name || '匿名客户' }}</span>
            <span class="record-card__tags">
              <el-tag size="small" :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
              <el-tag v-if="row.archived" size="small" type="info" effect="plain">归档</el-tag>
            </span>
          </div>

          <div class="record-card__meta">
            <span class="record-card__channel">{{ row.channel_name || '-' }}</span>
            <span class="record-card__sep">·</span>
            <span>{{ row.message_count }} 条消息</span>
            <span class="record-card__time">{{ row.last_message_at }}</span>
          </div>

          <div class="record-card__msg">{{ row.last_message || '—' }}</div>

          <div class="record-card__foot">
            <span class="record-card__agent">接待：{{ row.agent_name || '-' }}</span>
            <span class="record-card__more">查看记录 ›</span>
          </div>
        </button>

        <el-empty v-if="!loading && !list.length" description="暂无对话记录" :image-size="60" />
      </div>

      <div class="pager" :class="{ 'pager--mobile': isMobile }">
        <el-pagination
          v-model:current-page="query.page"
          :page-size="query.page_size"
          :total="total"
          :small="isMobile"
          :pager-count="isMobile ? 5 : 7"
          :layout="isMobile ? 'prev, pager, next' : 'total, prev, pager, next'"
          background
          @current-change="onPage"
        />
      </div>
    </div>

    <!-- 聊天记录抽屉：窄屏占满视口宽度 -->
    <el-drawer
      v-model="drawerVisible"
      title="聊天记录"
      :size="isMobile ? '100%' : '640px'"
      class="rec-drawer"
    >
      <div v-if="currentRecord" class="rec">
        <div class="rec__head">
          <span class="rec__name">{{ currentRecord.customer_name }}</span>
          <el-tag size="small" effect="plain">{{ currentRecord.channel_name }}</el-tag>
          <el-tag size="small" :type="statusTag(currentRecord.status)">
            {{ statusLabel(currentRecord.status) }}
          </el-tag>
          <span class="rec__muted">接待：{{ currentRecord.agent_name }}</span>
        </div>
        <div class="rec__muted rec__muted--sub">
          共 {{ currentRecord.message_count }} 条 ·
          {{ currentRecord.first_message_at }} ~ {{ currentRecord.last_message_at }}
        </div>
      </div>

      <div v-loading="detailLoading" class="rec-body kf-scroll">
        <div v-for="m in messages" :key="String(m.id)" class="msg">
          <div class="msg__meta">
            <span class="msg__who">{{ senderName(m) }}</span>
            <span class="msg__time">{{ formatMessageTime(String(m.created_at || '')) }}</span>
          </div>
          <div class="msg__text">{{ msgText(m) }}</div>
        </div>
        <el-empty v-if="!detailLoading && !messages.length" description="暂无消息" :image-size="60" />
      </div>
    </el-drawer>
  </div>
</template>

<style scoped lang="scss">
@use './shared.scss';

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;

  &__input {
    width: 240px;
  }

  &__sel {
    width: 140px;
  }

  &__date {
    width: 260px;
  }

  &__actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.tip {
  margin-bottom: 12px;
}

.customer {
  display: inline-flex;
  align-items: center;
  gap: 8px;

  &__name {
    font-size: 14px;
    color: #191919;
  }
}

.preview {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.5;

  &__line {
    font-size: 12px;
    color: #7f7f7f;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__tag {
    display: inline-block;
    min-width: 16px;
    margin-right: 4px;
    padding: 0 3px;
    border-radius: 2px;
    background: #f2f3f5;
    font-size: 11px;
    font-style: normal;
    color: #a8adb3;
    text-align: center;

    &--last {
      background: #eef7f1;
      color: #07c160;
    }
  }
}

/* ---------------- 窄屏卡片列表 ---------------- */
.record-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 120px;
}

.record-card {
  display: block;
  width: 100%;
  padding: 12px;
  border: 1px solid #ebedf0;
  border-radius: 8px;
  background: #fff;
  text-align: left;
  font: inherit;
  color: #191919;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;

  &:active {
    background: #f7f8fa;
    border-color: #d9dde1;
  }

  &__top {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  &__name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    font-size: 15px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__tags {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    gap: 4px;
  }

  &__meta {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 8px;
    font-size: 12px;
    color: #7f7f7f;
  }

  &__channel {
    max-width: 45%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__sep {
    color: #c8c9cc;
  }

  &__time {
    margin-left: auto;
    color: #a8adb3;
  }

  &__msg {
    display: -webkit-box;
    margin-top: 6px;
    overflow: hidden;
    font-size: 13px;
    line-height: 1.5;
    color: #4a4a4a;
    word-break: break-word;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }

  &__foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px dashed #f0f0f0;
    font-size: 12px;
    color: #a8adb3;
  }

  &__more {
    color: #07c160;
  }
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}

.rec {
  padding-bottom: 12px;
  margin-bottom: 12px;
  border-bottom: 1px solid #f0f0f0;

  &__head {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__name {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__muted {
    font-size: 12px;
    color: #7f7f7f;

    &--sub {
      margin-top: 4px;
    }
  }
}

.rec-body {
  max-height: calc(100vh - 240px);
}

.msg {
  padding: 8px 0;
  border-bottom: 1px dashed #f5f5f5;

  &__meta {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 3px;
  }

  &__who {
    font-size: 12px;
    font-weight: 600;
    color: #4a6fa5;
  }

  &__time {
    font-size: 11px;
    color: #b2b2b2;
  }

  &__text {
    max-width: 100%;
    font-size: 13px;
    line-height: 1.6;
    color: #191919;
    word-break: break-word;
    overflow-wrap: anywhere;
  }
}

/* ---------------- 移动端适配（≤768px，与 useIsMobile 断点一致） ---------------- */
@media (max-width: 768px) {
  .filters,
  .filters--mobile {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;

    .filters__input,
    .filters__sel,
    .filters__date {
      width: 100%;
      min-width: 0;
    }

    .filters__actions {
      width: 100%;

      :deep(.el-button) {
        flex: 1 1 0;
        min-width: 0;
        margin-left: 0;
      }
    }
  }

  .tip {
    margin-bottom: 10px;
  }

  .pager,
  .pager--mobile {
    justify-content: center;
    margin-top: 12px;
  }

  /* 抽屉占满宽度后：内部可滚动、底部留安全间距 */
  .rec {
    &__head {
      flex-wrap: wrap;
      gap: 6px 8px;
    }
  }

  .rec-body {
    max-height: calc(100vh - 200px);
    padding-bottom: calc(12px + env(safe-area-inset-bottom, 0));
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .msg {
    min-width: 0;
    padding: 8px 0;

    &__text {
      max-width: 100%;
      font-size: 14px;
      line-height: 1.6;
    }
  }

  .rec-drawer :deep(.el-drawer__body) {
    overflow: hidden;
    padding: 12px;
  }
}
</style>