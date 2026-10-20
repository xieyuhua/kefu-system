<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import {
  channelTypeMeta,
  createAgent,
  deleteAgent,
  listAgents,
  listChannels,
  updateAgent,
  type Agent,
  type AgentPayload,
} from '@/shared/api/admin'
import type { Channel, Role } from '@/shared/types'
import { useIsMobile } from '@/shared/composables/useIsMobile'

/** 窄屏（≤768px）改用卡片列表 + 全屏抽屉 */
const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const agents = ref<Agent[]>([])
const channels = ref<Channel[]>([])

const drawerVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

interface AgentForm {
  account: string
  password: string
  name: string
  role: Role
  max_concurrent: number
  channel_ids: number[]
}

function emptyForm(): AgentForm {
  return { account: '', password: '', name: '', role: 'agent', max_concurrent: 5, channel_ids: [] }
}

const form = reactive<AgentForm>(emptyForm())

const rules = computed<FormRules<AgentForm>>(() => ({
  account: [{ required: true, message: '请输入登录账号', trigger: 'blur' }],
  name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  max_concurrent: [{ required: true, message: '请输入最大同时接待数', trigger: 'blur' }],
  password: [
    {
      required: editingId.value === null,
      message: '新增子账号必须设置初始密码',
      trigger: 'blur',
    },
  ],
  channel_ids: [
    {
      validator: (_rule, value: number[], callback) => {
        if (!value || value.length === 0) {
          callback(new Error('请至少选择 1 个接待渠道'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}))

const drawerTitle = computed(() => (editingId.value === null ? '新增子账号' : '编辑子账号'))

const channelOptions = computed(() =>
  channels.value.map((c) => ({ id: c.id, name: c.name, type: c.type, enabled: c.enabled })),
)

const onlineCount = computed(() => agents.value.filter((a) => a.online).length)

async function loadChannels(): Promise<void> {
  channels.value = await listChannels()
}

async function load(): Promise<void> {
  loading.value = true
  try {
    agents.value = await listAgents()
  } finally {
    loading.value = false
  }
}

async function refreshAll(): Promise<void> {
  await Promise.all([loadChannels(), load()])
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  drawerVisible.value = true
}

function openEdit(row: Agent): void {
  editingId.value = row.id
  Object.assign(form, {
    account: row.account,
    password: '',
    name: row.name,
    role: row.role,
    max_concurrent: row.max_concurrent,
    channel_ids: (row.channels || []).map((c) => c.id),
  })
  drawerVisible.value = true
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  const payload: AgentPayload = {
    account: form.account.trim(),
    name: form.name.trim(),
    role: form.role,
    max_concurrent: form.max_concurrent,
    channel_ids: [...form.channel_ids],
  }
  const pwd = form.password.trim()
  if (pwd) {
    payload.password = pwd
  }

  submitting.value = true
  try {
    if (editingId.value === null) {
      await createAgent(payload)
      ElMessage.success('子账号已创建')
    } else {
      await updateAgent(editingId.value, payload)
      ElMessage.success('子账号已更新')
    }
    drawerVisible.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

/**
 * 启用/禁用子账号。
 * 注意：API.md §4 的 PUT 请求体未列出 enabled 字段，此处按约定提交，
 * 若后端未支持则该开关不会持久化（行数据缺少该字段时按启用处理）。
 */
async function toggleEnabled(row: Agent, value: boolean): Promise<void> {
  try {
    await updateAgent(row.id, { enabled: value })
    row.enabled = value
    ElMessage.success(value ? '子账号已启用' : '子账号已禁用')
  } catch {
    await load()
  }
}

async function resetPassword(row: Agent): Promise<void> {
  let newPassword = ''
  try {
    const res = await ElMessageBox.prompt(
      `请输入「${row.name}（${row.account}）」的新密码，至少 6 位`,
      '重置密码',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputType: 'password',
        inputValidator: (val: string) => {
          if (!val || val.length < 6) return '密码至少 6 位'
          return true
        },
      },
    )
    newPassword = res.value.trim()
  } catch {
    return
  }

  await updateAgent(row.id, { password: newPassword })
  ElMessage.success('密码已重置')
}

async function remove(row: Agent): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `删除子账号「${row.name}（${row.account}）」后其历史会话将保留但无法再登录，确认删除？`,
      '删除子账号',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  await deleteAgent(row.id)
  ElMessage.success('子账号已删除')
  await load()
}

function statusText(row: Agent): string {
  if (row.status === 'online') return '在线'
  if (row.status === 'busy') return '忙碌'
  return '离线'
}

function statusType(row: Agent): 'success' | 'warning' | 'info' {
  if (row.status === 'online') return 'success'
  if (row.status === 'busy') return 'warning'
  return 'info'
}

onMounted(refreshAll)
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          子账号管理
          <span class="toolbar__count">
            共 {{ agents.length }} 个子账号，{{ onlineCount }} 人在线
          </span>
        </div>
        <div class="toolbar__actions">
          <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增子账号</el-button>
        </div>
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="agents"
        stripe
        border
        empty-text="暂无子账号"
      >
        <el-table-column prop="account" label="账号" min-width="120" />
        <el-table-column prop="name" label="姓名" min-width="110" />
        <el-table-column label="角色" width="90">
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'warning' : 'info'" size="small">
              {{ row.role === 'admin' ? '管理员' : '客服' }}
            </el-tag>
          </template>
        </el-table-column>

        <!-- ★ 每个子账号当前接待的渠道，以标签展示 ★ -->
        <el-table-column label="可接待渠道" min-width="240">
          <template #default="{ row }">
            <div v-if="row.channels && row.channels.length" class="channel-tags">
              <el-tag
                v-for="ch in row.channels"
                :key="ch.id"
                size="small"
                class="channel-tag"
                :color="channelTypeMeta(ch.type).color"
                effect="dark"
              >
                <span class="channel-tag__icon">{{ channelTypeMeta(ch.type).icon }}</span>
                <span class="channel-tag__label">{{ ch.name }}</span>
              </el-tag>
            </div>
            <span v-else class="muted">未分配</span>
          </template>
        </el-table-column>

        <el-table-column label="最大并发" width="100" align="center">
          <template #default="{ row }">{{ row.max_concurrent }}</template>
        </el-table-column>
        <el-table-column label="当前接待" width="100" align="center">
          <template #default="{ row }">{{ row.active_count ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="在线状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusType(row)" size="small">{{ statusText(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled !== false"
              @update:model-value="(val: boolean) => toggleEnabled(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="primary" @click="resetPassword(row)">重置密码</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：子账号卡片列表 -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in agents" :key="row.id" class="m-card">
          <div class="m-card__top">
            <span class="m-card__title">{{ row.name }}</span>
            <span class="m-card__tags">
              <el-tag size="small" :type="statusType(row)" effect="plain">{{ statusText(row) }}</el-tag>
              <el-tag :type="row.role === 'admin' ? 'warning' : 'info'" size="small" effect="plain">
                {{ row.role === 'admin' ? '管理员' : '客服' }}
              </el-tag>
            </span>
          </div>

          <div class="m-card__sub">账号 {{ row.account }}</div>

          <div class="m-card__meta">
            <span>当前接待 {{ row.active_count ?? 0 }} / {{ row.max_concurrent }}</span>
          </div>

          <div class="m-field">
            <span class="m-field__label">可接待</span>
            <span class="m-field__value">
              <span v-if="row.channels && row.channels.length" class="channel-tags">
                <el-tag
                  v-for="ch in row.channels"
                  :key="ch.id"
                  size="small"
                  class="channel-tag"
                  :color="channelTypeMeta(ch.type).color"
                  effect="dark"
                >
                  <span class="channel-tag__icon">{{ channelTypeMeta(ch.type).icon }}</span>
                  <span class="channel-tag__label">{{ ch.name }}</span>
                </el-tag>
              </span>
              <span v-else class="muted">未分配</span>
            </span>
          </div>

          <div class="m-card__foot m-card__foot--between">
            <span class="enable-row">
              <span class="enable-row__label">启用</span>
              <el-switch
                :model-value="row.enabled !== false"
                @update:model-value="(val: boolean) => toggleEnabled(row, val)"
              />
            </span>
            <span class="card-actions">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link type="primary" @click="resetPassword(row)">重置密码</el-button>
              <el-button link type="danger" @click="remove(row)">删除</el-button>
            </span>
          </div>
        </div>

        <el-empty v-if="!loading && !agents.length" description="暂无子账号" :image-size="60" />
      </div>
    </div>

    <el-drawer
      v-model="drawerVisible"
      :title="drawerTitle"
      :size="isMobile ? '100%' : '480px'"
      class="agent-drawer"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isMobile ? '88px' : '120px'"
      >
        <el-form-item label="账号" prop="account">
          <el-input v-model="form.account" placeholder="登录账号，如 kf01" />
        </el-form-item>
        <el-form-item label="姓名" prop="name">
          <el-input v-model="form.name" placeholder="如：小李" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-radio-group v-model="form.role">
            <el-radio value="agent">客服</el-radio>
            <el-radio value="admin">管理员</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="editingId === null ? '初始密码，至少 6 位' : '留空表示不修改'"
          />
        </el-form-item>
        <el-form-item label="最大同时接待数" prop="max_concurrent">
          <el-input-number v-model="form.max_concurrent" :min="1" :max="100" />
        </el-form-item>

        <!-- ★ 可接待渠道：多选，决定该子账号能接待哪些渠道的会话 ★ -->
        <el-form-item label="可接待渠道" prop="channel_ids">
          <el-select
            v-model="form.channel_ids"
            multiple
            collapse-tags
            collapse-tags-tooltip
            filterable
            placeholder="请选择该子账号可接待的渠道"
            style="width: 100%"
          >
            <el-option
              v-for="ch in channelOptions"
              :key="ch.id"
              :label="ch.name"
              :value="ch.id"
              :disabled="!ch.enabled"
            >
              <span class="channel-tag">
                <span class="channel-tag__icon" :style="{ color: channelTypeMeta(ch.type).color }">
                  {{ channelTypeMeta(ch.type).icon }}
                </span>
                <span class="channel-tag__label">{{ ch.name }}</span>
              </span>
            </el-option>
          </el-select>
          <div class="form-tip">
            仅勾选的渠道会话会分配给该子账号；已停用的渠道不可选择。
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-footer">
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped lang="scss">
@use './shared.scss';

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  &__title {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__count {
    margin-left: 8px;
    font-size: 12px;
    font-weight: 400;
    color: #7f7f7f;
  }

  &__actions {
    display: flex;
    gap: 8px;
  }
}

.channel-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.muted {
  color: #7f7f7f;
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  color: #7f7f7f;
  line-height: 1.5;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

/* ---------------- 移动端（≤768px，与 useIsMobile 断点一致） ---------------- */
@media (max-width: 768px) {
  .toolbar {
    margin-bottom: 10px;
  }

  /* 卡片底部：启用开关 + 操作按钮分两行也不挤 */
  .enable-row {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .enable-row__label {
    font-size: 12px;
    color: #7f7f7f;
  }

  .card-actions {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .channel-tags {
    gap: 4px;
  }

  /* 全屏抽屉：表单区可滚动，底部按钮不遮挡内容 */
  :global(.agent-drawer .el-drawer__body) {
    padding: 12px 16px;
    overflow-y: auto;
  }

  :global(.agent-drawer .el-drawer__footer) {
    padding: 10px 16px calc(10px + env(safe-area-inset-bottom, 0));
    border-top: 1px solid #f0f0f0;
  }

  /* label 宽度改由 :label-width 响应式设置，这里只调间距 */
  :global(.agent-drawer .el-form-item) {
    margin-bottom: 14px;
  }
}
</style>
