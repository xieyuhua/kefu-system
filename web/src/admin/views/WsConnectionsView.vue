<template>
  <div class="ws-page">
    <div class="ws-toolbar">
      <el-radio-group v-model="kind" size="small" @change="load">
        <el-radio-button label="">全部</el-radio-button>
        <el-radio-button label="agent">客服端</el-radio-button>
        <el-radio-button label="guest">访客端</el-radio-button>
      </el-radio-group>

      <span class="ws-stat">
        共 <b>{{ total }}</b> 个连接（客服 {{ agents }} · 访客 {{ guests }}）
      </span>

      <div class="ws-spacer"></div>

      <el-checkbox v-model="autoRefresh" size="small">每 5 秒自动刷新</el-checkbox>
      <el-button size="small" :icon="Refresh" @click="load">刷新</el-button>
      <el-button
        size="small"
        type="danger"
        plain
        :disabled="!selected.length"
        @click="kickSelected"
      >
        断开选中（{{ selected.length }}）
      </el-button>
    </div>

    <el-table
      v-loading="loading"
      :data="rows"
      size="small"
      class="ws-table"
      @selection-change="onSelect"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column label="类型" width="90">
        <template #default="{ row }">
          <el-tag :type="row.kind === 'agent' ? 'success' : 'info'" size="small" effect="plain">
            {{ row.kind === 'agent' ? '客服端' : '访客端' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="标识" min-width="180">
        <template #default="{ row }">
          <span v-if="row.kind === 'agent'">客服 #{{ row.user_id }}</span>
          <span v-else>会话 #{{ row.conversation_id }}</span>
          <span class="ws-dim"> · 连接 #{{ row.id }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="device" label="端" width="90" />
      <el-table-column prop="remote_addr" label="来源地址" min-width="160" show-overflow-tooltip />
      <el-table-column label="在线时长" width="110">
        <template #default="{ row }">{{ fmtDuration(row.duration_seconds) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="90" fixed="right">
        <template #default="{ row }">
          <el-button size="small" text type="danger" @click="kickOne(row)">断开</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && !rows.length" description="当前没有长连接" :image-size="70" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { kickWsConnections, listWsConnections, type WsConnection } from '@/shared/api/ws'

const rows = ref<WsConnection[]>([])
const total = ref(0)
const agents = ref(0)
const guests = ref(0)
const loading = ref(false)
const kind = ref('')
const autoRefresh = ref(true)
const selected = ref<WsConnection[]>([])

let timer: number | null = null

function fmtDuration(sec: number): string {
  if (sec < 60) return `${sec} 秒`
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m} 分 ${sec % 60} 秒`
  const h = Math.floor(m / 60)
  return `${h} 小时 ${m % 60} 分`
}

function onSelect(list: WsConnection[]): void {
  selected.value = list
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await listWsConnections(kind.value)
    rows.value = res.list || []
    total.value = res.total || 0
    agents.value = res.agents || 0
    guests.value = res.guests || 0
  } catch {
    // 失败提示由 http 层统一弹出，这里保持列表原样
  } finally {
    loading.value = false
  }
}

async function kickOne(row: WsConnection): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确认断开这条连接（${row.kind === 'agent' ? '客服 #' + row.user_id : '会话 #' + row.conversation_id}）？`,
      '断开连接',
      { type: 'warning', confirmButtonText: '断开', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  const res = await kickWsConnections([row.id])
  ElMessage.success(`已断开 ${res.closed} 个连接`)
  await load()
}

async function kickSelected(): Promise<void> {
  const ids = selected.value.map((r) => r.id)
  if (!ids.length) return
  try {
    await ElMessageBox.confirm(`确认断开选中的 ${ids.length} 个连接？`, '批量断开', {
      type: 'warning',
      confirmButtonText: '断开',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  const res = await kickWsConnections(ids)
  ElMessage.success(`已断开 ${res.closed} 个连接`)
  await load()
}

function startTimer(): void {
  stopTimer()
  timer = window.setInterval(() => {
    if (autoRefresh.value && !loading.value) void load()
  }, 5000)
}

function stopTimer(): void {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (on) => (on ? startTimer() : stopTimer()))

onMounted(() => {
  void load()
  startTimer()
})

onBeforeUnmount(stopTimer)
</script>

<style scoped lang="scss">
.ws-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ws-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.ws-stat {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.ws-spacer {
  flex: 1;
}

.ws-dim {
  color: var(--el-text-color-placeholder);
  font-size: 12px;
}

.ws-table {
  width: 100%;
}
</style>