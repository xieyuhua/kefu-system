<script setup lang="ts">
/**
 * 对话记录查询（管理后台）
 * 全局检索历史会话：支持按客户昵称 / 消息内容 / 渠道 / 客服 / 日期范围 / 状态筛选，
 * 点击可查看该会话的完整聊天记录。
 */
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import { listChannels as fetchChannels, listAgents, type Agent } from '@/shared/api/admin'
import {
  RECORD_MESSAGE_PAGE_SIZE,
  fetchConversationRecords,
  fetchRecordMessages,
  type ConversationRecord,
  type RecordMessage,
} from '@/shared/api/records'
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

/* ---------------- 查看完整聊天记录 ----------------
 * 会话可能很长（数百上千条），一次性取全会拖慢抽屉，因此分页：
 *   首屏取最近 RECORD_MESSAGE_PAGE_SIZE 条 → 顶部「加载更早消息」用 before_id 游标继续向前翻页，
 *   直到 has_more=false 为止。接口同时返回 total，抽屉里显示「共 N 条」。
 * 翻页只影响本抽屉，工作台（/chat/）的消息分页链路完全独立、未改动。
 */
const drawerVisible = ref(false)
const detailLoading = ref(false)
const detail = ref<ConversationRecord | null>(null)
const messages = ref<RecordMessage[]>([])
/** 该会话消息总数（接口返回的 DB 实际条数，不是「已加载条数」） */
const msgTotal = ref(0)
/** 还有更早的消息可加载 */
const hasMore = ref(false)
/** 正在加载更早的消息（按钮 loading + 防重复点击） */
const loadingMore = ref(false)
/** 抽屉内的消息滚动容器（翻页后据此补偿滚动高度，保证视口不跳动） */
const bodyRef = ref<HTMLElement | null>(null)

const currentRecord = computed(() => detail.value)
/** 抽屉标题上的已加载进度 */
const shownCount = computed(() => messages.value.length)

/** 记录首屏/翻页的请求序号，抽屉快速切换时丢弃过期响应 */
let fetchSeq = 0

async function openRecord(row: ConversationRecord): Promise<void> {
  const seq = ++fetchSeq
  drawerVisible.value = true
  detailLoading.value = true
  detail.value = row
  messages.value = []
  msgTotal.value = 0
  hasMore.value = false
  loadingMore.value = false
  try {
    const page = await fetchRecordMessages(row.id, { limit: RECORD_MESSAGE_PAGE_SIZE })
    if (seq !== fetchSeq) return
    messages.value = page.list || []
    msgTotal.value = page.total ?? messages.value.length
    hasMore.value = Boolean(page.has_more)
  } catch {
    if (seq === fetchSeq) ElMessage.error('聊天记录加载失败')
  } finally {
    if (seq === fetchSeq) detailLoading.value = false
  }
}

/**
 * 加载更早的消息：用当前首条消息的 id 作为 before_id 向前翻一页，追加到列表顶部。
 * 追加后补齐 scrollHeight 增量，让原本可见的那条消息停在原处（视口不跳动）。
 */
async function loadEarlier(): Promise<void> {
  if (!hasMore.value || loadingMore.value || detailLoading.value) return
  const firstID = messages.value[0]?.id
  if (!firstID) return

  const body = bodyRef.value
  const prevHeight = body?.scrollHeight ?? 0
  const prevTop = body?.scrollTop ?? 0

  loadingMore.value = true
  try {
    const recordID = Number(currentRecord.value?.id || 0)
    if (!recordID) return
    // before_id = 当前首条消息 id：接口返回 id < before_id 的上一页，保证与已有数据无缝相接
    const page = await fetchRecordMessages(recordID, { before_id: Number(firstID) })
    const older = page.list || []
    if (!older.length) {
      // 后端已无更早数据：收敛按钮状态，避免用户反复点击
      hasMore.value = false
      msgTotal.value = Math.max(msgTotal.value, messages.value.length)
      return
    }
    messages.value = [...older, ...messages.value]
    if (typeof page.total === 'number') msgTotal.value = page.total
    hasMore.value = Boolean(page.has_more)
    await nextTick()
    // 补偿新增高度：原本可见的消息位置不变，避免「加载更早」后内容整体下跳
    const el = bodyRef.value
    if (el) el.scrollTop = prevTop + (el.scrollHeight - prevHeight)
  } catch {
    ElMessage.error('更早的聊天记录加载失败')
  } finally {
    loadingMore.value = false
  }
}

/** 消息角色：客户 / 客服 / 智能助手（机器人）/ 系统 —— 决定气泡的排布与配色 */
type MsgRole = 'customer' | 'agent' | 'robot' | 'system'

/**
 * 归类消息角色。优先看 sender.role（后端 buildMessageVO 已按 sender_role 归一：
 * 客服→agent、客户→user、机器人→robot、系统→system），缺失时再退回 direction。
 */
function roleOf(m: RecordMessage): MsgRole {
  const role = m.sender?.role
  if (role === 'robot') return 'robot'
  if (role === 'agent') return 'agent'
  if (role === 'system') return 'system'
  if (role === 'user' || role === 'customer') return 'customer'
  if (m.direction === 'in') return 'customer'
  if (m.direction === 'out') return 'agent'
  return 'system'
}

/** 消息内容预览：卡片类消息展示摘要，纯文本直接展示（渲染口径保持原样） */
const msgText = (m: RecordMessage): string => {
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

/** 发送者名称：保留原有「名称 + 时间」格式，仅给机器人补一个 🤖 标识 */
function senderName(m: RecordMessage): string {
  const s = m.sender
  switch (roleOf(m)) {
    case 'agent':
      return s?.name || '客服'
    case 'customer':
      return s?.name || '客户'
    case 'robot':
      return '🤖 ' + (s?.name || '智能助手')
    default:
      return s?.name || '系统'
  }
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
          popper-class="records-date-popper"
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

      <div class="pager">
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
          共 {{ msgTotal }} 条消息 · 已加载 {{ shownCount }} 条 ·
          {{ currentRecord.first_message_at }} ~ {{ currentRecord.last_message_at }}
        </div>
      </div>

      <div ref="bodyRef" v-loading="detailLoading" class="rec-body kf-scroll">
        <!-- 还有更早的消息时，列表顶部给出「加载更早消息」入口；加载完成后收敛为一条提示 -->
        <div v-if="hasMore" class="rec-more">
          <el-button size="small" plain :loading="loadingMore" @click="loadEarlier">
            加载更早消息
          </el-button>
        </div>
        <div v-else-if="messages.length" class="rec-more rec-more--done">已经是最早的一条消息</div>

        <div
          v-for="m in messages"
          :key="String(m.id)"
          class="msg"
          :class="`msg--${roleOf(m)}`"
        >
          <div class="msg__meta">
            <span class="msg__who">{{ senderName(m) }}</span>
            <span class="msg__time">{{ formatMessageTime(String(m.created_at || '')) }}</span>
          </div>
          <div class="msg__bubble">{{ msgText(m) }}</div>
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
    /* 宽屏让两个按钮直接参与 .filters 的换行计算，保持原有排布 */
    display: contents;
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

/* 「加载更早消息」入口：吸附在消息列表顶部，滚到任何位置都能直接点，不用先滚回开头 */
.rec-more {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  justify-content: center;
  padding: 8px 0;
  margin-bottom: 4px;
  background: #fff;
  border-bottom: 1px solid #f5f5f5;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.03);

  /* 已到最早一条时只是说明文案：留在列表顶部即可，不再吸附，避免遮挡上一条消息 */
  &--done {
    position: static;
    padding: 6px 0;
    border-bottom: 0;
    box-shadow: none;
    font-size: 12px;
    color: #a8adb3;
  }
}

/**
 * 消息气泡：按角色区分排布与配色（一眼看出谁说的）。
 * 配色原则：底色一律是低饱和度淡色（rgba 透明度 ≤ 0.12），正文都是深色（各自带一点色相），
 * 保证「区分明显」的同时文字与背景对比度 ≥ 4.5:1（实测 客户 13.8:1 / 客服 11.6:1 /
 * 机器人 13.4:1 / 系统 7.4:1，均满足 WCAG AA；不用白字配浅底、也不用浅灰字）。
 * 四类的底色与正文色均互不相同，系统消息额外用居中 pill（圆角 999px）作区分。
 */
.msg {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
  min-width: 0;
  margin-bottom: 10px;

  &__meta {
    display: flex;
    align-items: baseline;
    gap: 8px;
    max-width: 100%;
    font-size: 12px;
    color: #7f7f7f;
  }

  &__who {
    font-weight: 600;
    color: #4a6fa5;
  }

  &__time {
    font-size: 11px;
    color: #b2b2b2;
  }

  &__bubble {
    max-width: min(84%, 520px);
    padding: 8px 10px;
    border-radius: 8px;
    background: rgba(0, 0, 0, 0.06);
    font-size: 13px;
    line-height: 1.6;
    color: #1f2329;
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  /* 客户：靠左，浅灰底 */
  &--customer {
    align-items: flex-start;

    .msg__who {
      color: #4a6fa5;
    }

    .msg__bubble {
      background: rgba(0, 0, 0, 0.06);
      color: #1f2329;
    }
  }

  /* 客服：靠右，浅绿底 */
  &--agent {
    align-items: flex-end;

    .msg__who {
      color: #0a7a3f;
    }

    .msg__bubble {
      background: rgba(7, 193, 96, 0.1);
      color: #0d3a24;
    }
  }

  /* 智能助手 / 机器人：靠左，浅蓝底 */
  &--robot {
    align-items: flex-start;

    .msg__who {
      color: #2a5fbf;
    }

    .msg__bubble {
      background: rgba(64, 128, 255, 0.1);
      color: #10254f;
    }
  }

  /* 系统提示：居中灰色 pill */
  &--system {
    align-items: center;

    .msg__who {
      color: #5a6068;
    }

    .msg__bubble {
      max-width: 100%;
      padding: 4px 12px;
      border-radius: 999px;
      background: rgba(0, 0, 0, 0.05);
      font-size: 12px;
      color: #4a4f57;
      text-align: center;
    }
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
    .filters__sel {
      width: 100%;
      min-width: 0;
    }

    /* el-date-picker 的根节点不继承 scoped 属性，需用 :deep 命中 */
    :deep(.filters__date) {
      width: 100%;
      min-width: 0;
    }

    .filters__actions {
      display: flex;
      width: 100%;
      gap: 8px;

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

  /* 日期范围面板固定宽 646px，窄屏收敛到视口内改为横向滚动，避免整页横向溢出 */
  :global(.records-date-popper) {
    max-width: calc(100vw - 12px);
    overflow-x: auto;
    overflow-y: hidden;
  }

  /* 抽屉占满宽度后：内部可滚动、底部留安全间距 */
  .rec {
    flex: 0 0 auto;

    &__head {
      flex-wrap: wrap;
      gap: 6px 8px;
    }
  }

  /* 抽屉 body 由 Element Plus 渲染（teleport 到 body），需用 :global 命中 */
  :global(.rec-drawer .el-drawer__body) {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding: 12px;
  }

  .rec-body {
    flex: 1 1 auto;
    min-height: 0;
    /* flex 生效时由抽屉高度约束；万一 flex 未命中，仍以视口高度兜底可滚动 */
    max-height: calc(100vh - 24px);
    overflow-y: auto;
    padding-bottom: calc(12px + env(safe-area-inset-bottom, 0));
    overscroll-behavior: contain;
  }

  .msg {
    min-width: 0;

    /* 窄屏气泡不顶满：留出对侧留白，仍能看出左右区分，且绝不横向溢出 */
    &__bubble {
      max-width: 88%;
      font-size: 14px;
      line-height: 1.6;
    }

    &--system .msg__bubble {
      max-width: 100%;
      font-size: 12px;
    }
  }
}
</style>