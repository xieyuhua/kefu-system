<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  MATCH_TYPE_OPTIONS,
  createAutoReply,
  deleteAutoReply,
  listAutoReplies,
  listChannels,
  matchTypeLabel,
  updateAutoReply,
  type AutoReply,
} from '@/shared/api/admin'
import type { Channel } from '@/shared/types'
import { useIsMobile } from '@/shared/composables/useIsMobile'

/** 窄屏（≤768px）改用卡片列表 + 全屏抽屉 */
const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const list = ref<AutoReply[]>([])
const channels = ref<Channel[]>([])

const drawerVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

interface AutoReplyForm {
  name: string
  channel_id: number
  match_type: string
  keywords: string
  reply: string
  transfer_to_human: boolean
  enabled: boolean
  priority: number
}

function emptyForm(): AutoReplyForm {
  return {
    name: '',
    channel_id: 0,
    match_type: 'contain',
    keywords: '',
    reply: '',
    transfer_to_human: false,
    enabled: true,
    priority: 0,
  }
}

const form = reactive<AutoReplyForm>(emptyForm())

const rules: FormRules<AutoReplyForm> = {
  name: [{ required: true, message: '请输入规则名', trigger: 'blur' }],
  keywords: [{ required: true, message: '请输入关键词', trigger: 'blur' }],
  reply: [
    {
      validator: (_rule, value: string, callback) => {
        if (form.transfer_to_human) return callback()
        if (!value || !String(value).trim()) return callback(new Error('请输入回复内容'))
        callback()
      },
      trigger: 'blur',
    },
  ],
}

const drawerTitle = computed(() => (editingId.value === null ? '新增自动回复规则' : '编辑自动回复规则'))

/** 渠道下拉：0 = 全渠道 */
const channelOptions = computed(() => [
  { id: 0, name: '全渠道' },
  ...channels.value.map((c) => ({ id: c.id, name: c.name })),
])

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
    list.value = await listAutoReplies()
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  drawerVisible.value = true
  formRef.value?.clearValidate()
}

function openEdit(row: AutoReply): void {
  editingId.value = row.id
  Object.assign(form, {
    name: row.name,
    channel_id: row.channel_id ?? 0,
    match_type: row.match_type || 'contain',
    keywords: row.keywords || '',
    reply: row.reply || '',
    transfer_to_human: !!row.transfer_to_human,
    enabled: !!row.enabled,
    priority: row.priority ?? 0,
  })
  drawerVisible.value = true
  formRef.value?.clearValidate()
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  const payload = {
    name: form.name.trim(),
    channel_id: form.channel_id,
    match_type: form.match_type,
    keywords: form.keywords.trim(),
    // 转人工时回复内容留空
    reply: form.transfer_to_human ? '' : form.reply.trim(),
    reply_type: 'text',
    transfer_to_human: form.transfer_to_human,
    enabled: form.enabled,
    priority: Number(form.priority) || 0,
  }

  submitting.value = true
  try {
    if (editingId.value === null) {
      await createAutoReply(payload)
      ElMessage.success('规则已创建')
    } else {
      await updateAutoReply(editingId.value, payload)
      ElMessage.success('规则已更新')
    }
    drawerVisible.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(row: AutoReply, value: boolean): Promise<void> {
  try {
    await updateAutoReply(row.id, {
      name: row.name,
      channel_id: row.channel_id,
      match_type: row.match_type,
      keywords: row.keywords,
      reply: row.reply,
      reply_type: row.reply_type,
      transfer_to_human: row.transfer_to_human,
      enabled: value,
      priority: row.priority,
    })
    row.enabled = value
    ElMessage.success(value ? '规则已启用' : '规则已停用')
  } catch {
    await load()
  }
}

async function remove(row: AutoReply): Promise<void> {
  try {
    await ElMessageBox.confirm(`删除规则「${row.name}」后不可恢复，确认删除？`, '删除规则', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  await deleteAutoReply(row.id)
  ElMessage.success('规则已删除')
  await load()
}

function keywordTags(keywords: string): string[] {
  return (keywords || '')
    .split(',')
    .map((k) => k.trim())
    .filter(Boolean)
}

onMounted(() => {
  void load()
  void loadChannels()
})
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          自动回复
          <span class="toolbar__count">共 {{ list.length }} 条规则</span>
        </div>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增规则</el-button>
      </div>

      <div class="tip">
        命中规则即由机器人自动回复；开启「转人工」的规则命中后会把会话转入人工接待池并通知客服。
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无自动回复规则"
      >
        <el-table-column prop="name" label="规则名" min-width="130" show-overflow-tooltip />
        <el-table-column label="适用渠道" min-width="150">
          <template #default="{ row }">
            <el-tag v-if="!row.channel_id" size="small" type="success">全渠道</el-tag>
            <span v-else>{{ row.channel_name || '--' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="匹配方式" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small">{{ matchTypeLabel(row.match_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="关键词" min-width="200">
          <template #default="{ row }">
            <span v-if="!keywordTags(row.keywords).length" class="muted">--</span>
            <span v-else class="kw-list">
              <el-tag v-for="kw in keywordTags(row.keywords)" :key="kw" size="small" effect="plain">
                {{ kw }}
              </el-tag>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="回复内容" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.transfer_to_human" class="muted">—（转人工）</span>
            <span v-else>{{ row.reply || '--' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="转人工" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.transfer_to_human" size="small" type="warning">转人工</el-tag>
            <span v-else class="muted">否</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              @update:model-value="(val: boolean) => toggleEnabled(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="priority" label="优先级" width="90" align="center" sortable />
        <el-table-column prop="hit_count" label="命中次数" width="100" align="center" sortable />
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：规则卡片列表 -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in list" :key="row.id" class="m-card">
          <div class="m-card__top">
            <span class="m-card__title">{{ row.name }}</span>
            <span class="m-card__tags">
              <el-tag v-if="!row.channel_id" size="small" type="success">全渠道</el-tag>
              <el-tag v-else size="small" effect="plain">{{ row.channel_name || '--' }}</el-tag>
            </span>
          </div>

          <div class="m-card__meta">
            <el-tag size="small">匹配 {{ matchTypeLabel(row.match_type) }}</el-tag>
            <el-tag v-if="row.transfer_to_human" size="small" type="warning">转人工</el-tag>
            <span>优先级 {{ row.priority }}</span>
            <span>命中 {{ row.hit_count ?? 0 }}</span>
          </div>

          <div class="m-field">
            <span class="m-field__label">关键词</span>
            <span class="m-field__value">
              <span v-if="!keywordTags(row.keywords).length" class="muted">--</span>
              <span v-else class="kw-list">
                <el-tag v-for="kw in keywordTags(row.keywords)" :key="kw" size="small" effect="plain">
                  {{ kw }}
                </el-tag>
              </span>
            </span>
          </div>

          <div class="m-field">
            <span class="m-field__label">回复</span>
            <span class="m-field__value">
              <span v-if="row.transfer_to_human" class="muted">—（转人工）</span>
              <span v-else>{{ row.reply || '--' }}</span>
            </span>
          </div>

          <div class="m-card__foot m-card__foot--between">
            <span class="enable-row">
              <span class="enable-row__label">启用</span>
              <el-switch
                :model-value="row.enabled"
                @update:model-value="(val: boolean) => toggleEnabled(row, val)"
              />
            </span>
            <span class="card-actions">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link type="danger" @click="remove(row)">删除</el-button>
            </span>
          </div>
        </div>

        <el-empty v-if="!loading && !list.length" description="暂无自动回复规则" :image-size="60" />
      </div>
    </div>

    <el-drawer
      v-model="drawerVisible"
      :title="drawerTitle"
      :size="isMobile ? '100%' : '480px'"
      class="reply-drawer"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isMobile ? '88px' : '110px'"
      >
        <el-form-item label="规则名" prop="name">
          <el-input v-model="form.name" placeholder="如：发货咨询" maxlength="30" show-word-limit />
        </el-form-item>

        <el-form-item label="适用渠道" prop="channel_id">
          <el-select v-model="form.channel_id" placeholder="请选择适用渠道" style="width: 100%">
            <el-option
              v-for="opt in channelOptions"
              :key="opt.id"
              :label="opt.name"
              :value="opt.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="匹配方式" prop="match_type">
          <el-select v-model="form.match_type" style="width: 100%">
            <el-option
              v-for="opt in MATCH_TYPE_OPTIONS"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="关键词" prop="keywords">
          <el-input
            v-model="form.keywords"
            type="textarea"
            :rows="2"
            placeholder="如：发货,物流,快递"
          />
          <div class="form-tip">多个关键词用英文逗号分隔</div>
        </el-form-item>

        <el-form-item label="回复内容" prop="reply">
          <el-input
            v-model="form.reply"
            type="textarea"
            :rows="3"
            :disabled="form.transfer_to_human"
            :placeholder="form.transfer_to_human ? '转人工规则无需回复内容' : '命中后由机器人自动发送'"
          />
        </el-form-item>

        <el-form-item label="是否转人工">
          <el-switch v-model="form.transfer_to_human" />
          <span v-if="form.transfer_to_human" class="form-tip form-tip--inline">
            命中后直接转人工接待
          </span>
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="优先级" prop="priority">
          <el-input-number v-model="form.priority" :min="0" :max="9999" />
          <div class="form-tip">数字越大越优先匹配</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-foot">
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
  margin-bottom: 12px;

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
}

.tip {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: #f5f5f5;
  border-radius: 4px;
  font-size: 12px;
  color: #7f7f7f;
}

.kw-list {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}

.form-tip {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: #7f7f7f;

  &--inline {
    width: auto;
    margin-left: 10px;
    color: #e6a23c;
  }
}

.drawer-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.muted {
  color: #7f7f7f;
}

/* ---------------- 移动端（≤768px，与 useIsMobile 断点一致） ---------------- */
@media (max-width: 768px) {
  .toolbar {
    margin-bottom: 10px;
  }

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

  .kw-list {
    gap: 4px;
  }

  /* 全屏抽屉：表单可滚动，底部按钮固定可见 */
  :global(.reply-drawer .el-drawer__body) {
    padding: 12px 16px;
    overflow-y: auto;
  }

  :global(.reply-drawer .el-drawer__footer) {
    padding: 10px 16px calc(10px + env(safe-area-inset-bottom, 0));
    border-top: 1px solid #f0f0f0;
  }

  :global(.reply-drawer .el-form-item) {
    margin-bottom: 14px;
  }
}
</style>
