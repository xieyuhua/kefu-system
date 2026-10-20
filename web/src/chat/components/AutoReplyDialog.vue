<script setup lang="ts">
/**
 * 自动回复管理对话框（客服工作台）。
 *
 * 规则是团队共用的：列表里能看到全租户的规则（含管理员建的公共规则、其他客服建的规则）。
 * 归属与权限由后端下发：
 *   - user_id = 0 → 公共规则，客服只读（编辑 / 删除置灰 + tooltip「公共规则仅管理员可修改」）；
 *   - user_id > 0 → 该客服创建，只有本人（和管理员）能改。
 *
 * 关键词匹配、转人工、命中计数等触发逻辑全部在后端，这里只做配置管理。
 */
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  MATCH_TYPE_OPTIONS,
  createAutoReply,
  deleteAutoReply,
  listAutoReplies,
  matchTypeLabel,
  updateAutoReply,
  type AutoReply,
  type AutoReplyPayload,
} from '@/shared/api/admin'
import { listChannels } from '@/shared/api/channels'
import type { Channel } from '@/shared/types'
import { useIsMobile } from '@/shared/composables/useIsMobile'

const visible = defineModel<boolean>({ default: false })

const emit = defineEmits<{
  /** 规则增删改后通知父级（工作台暂无依赖，留给后续统计刷新） */
  changed: []
}>()

const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const list = ref<AutoReply[]>([])
const channels = ref<Channel[]>([])

const formOpen = ref(false)
const editingId = ref<number | null>(null)

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
const formTitle = computed(() => (editingId.value === null ? '新增自动回复' : '编辑自动回复'))

/** 渠道下拉：0 = 全渠道 */
const channelOptions = computed(() => [
  { id: 0, name: '全渠道' },
  ...channels.value.map((c) => ({ id: c.id, name: c.name })),
])

function keywordTags(keywords: string): string[] {
  return (keywords || '')
    .split(/[,，;；\n]/)
    .map((k) => k.trim())
    .filter(Boolean)
}

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
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  formOpen.value = true
}

function openEdit(row: AutoReply): void {
  if (!row.editable) {
    ElMessage.warning('公共规则仅管理员可修改')
    return
  }
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
  formOpen.value = true
}

function closeForm(): void {
  formOpen.value = false
  editingId.value = null
  Object.assign(form, emptyForm())
}

/** 编辑某条规则时的提交体（开关切换复用） */
function payloadOf(row: AutoReply, enabled: boolean): AutoReplyPayload {
  return {
    name: row.name,
    channel_id: row.channel_id ?? 0,
    match_type: row.match_type || 'contain',
    keywords: row.keywords,
    reply: row.transfer_to_human ? '' : row.reply,
    reply_type: row.reply_type || 'text',
    transfer_to_human: !!row.transfer_to_human,
    enabled,
    priority: row.priority ?? 0,
  }
}

async function submit(): Promise<void> {
  const name = form.name.trim()
  const keywords = form.keywords.trim()
  if (!name) {
    ElMessage.warning('请输入规则名')
    return
  }
  if (!keywords) {
    ElMessage.warning('请输入关键词')
    return
  }
  if (!form.transfer_to_human && !form.reply.trim()) {
    ElMessage.warning('请输入回复内容（转人工规则可留空）')
    return
  }

  const payload: AutoReplyPayload = {
    name,
    channel_id: form.channel_id,
    match_type: form.match_type,
    keywords,
    // 转人工规则不需要回复内容
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
    closeForm()
    await load()
    emit('changed')
  } catch {
    // 错误已由拦截器提示（如「公共规则仅管理员可修改」）
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(row: AutoReply, value: boolean): Promise<void> {
  if (!row.editable) {
    ElMessage.warning('公共规则仅管理员可修改')
    await load()
    return
  }
  try {
    await updateAutoReply(row.id, payloadOf(row, value))
    row.enabled = value
    ElMessage.success(value ? '规则已启用' : '规则已停用')
    emit('changed')
  } catch {
    await load()
  }
}

async function remove(row: AutoReply): Promise<void> {
  if (!row.editable) {
    ElMessage.warning('公共规则仅管理员可修改')
    return
  }
  try {
    await ElMessageBox.confirm(`删除规则「${row.name}」后不可恢复，确认删除？`, '删除规则', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteAutoReply(row.id)
    ElMessage.success('规则已删除')
    if (editingId.value === row.id) closeForm()
    await load()
    emit('changed')
  } catch {
    // 错误已由拦截器提示
  }
}

watch(visible, (open) => {
  if (!open) {
    closeForm()
    return
  }
  void load()
  if (!channels.value.length) void loadChannels()
})
</script>

<template>
  <el-dialog
    v-model="visible"
    title="自动回复管理"
    :width="isMobile ? '94%' : '620px'"
    :close-on-click-modal="false"
    append-to-body
    class="auto-reply-dialog"
  >
    <div class="ar-toolbar">
      <span class="ar-toolbar__count">共 {{ list.length }} 条规则（团队共用）</span>
      <el-button type="primary" size="small" :icon="Plus" @click="openCreate">新增</el-button>
    </div>

    <div class="ar-tip">
      客户消息命中关键词即自动回复；勾「转人工」的规则命中后会把会话转入待接待并通知客服。
      公共规则（管理员建）仅可查看，改 / 删需到管理后台。
    </div>

    <div v-if="formOpen" class="ar-form">
      <div class="ar-form__title">{{ formTitle }}</div>
      <el-form
        :model="form"
        :label-position="isMobile ? 'top' : 'right'"
        :label-width="isMobile ? undefined : '76px'"
        @submit.prevent
      >
        <el-form-item label="规则名" required>
          <el-input v-model="form.name" maxlength="30" show-word-limit placeholder="如：发货咨询" />
        </el-form-item>
        <el-form-item label="渠道">
          <el-select v-model="form.channel_id" style="width: 100%">
            <el-option v-for="opt in channelOptions" :key="opt.id" :label="opt.name" :value="opt.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="匹配">
          <el-select v-model="form.match_type" style="width: 100%">
            <el-option
              v-for="opt in MATCH_TYPE_OPTIONS"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词" required>
          <el-input
            v-model="form.keywords"
            type="textarea"
            :rows="2"
            placeholder="如：发货,物流,快递（多个用逗号分隔）"
          />
        </el-form-item>
        <el-form-item label="回复内容">
          <el-input
            v-model="form.reply"
            type="textarea"
            :rows="3"
            :disabled="form.transfer_to_human"
            :placeholder="form.transfer_to_human ? '转人工规则无需回复内容' : '命中后由机器人自动发送'"
          />
        </el-form-item>
        <el-form-item label="转人工">
          <el-switch v-model="form.transfer_to_human" />
          <span class="ar-form__tip">命中后直接转人工接待</span>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="0" :max="9999" controls-position="right" />
          <span class="ar-form__tip">数字越大越优先匹配</span>
        </el-form-item>
      </el-form>
      <div class="ar-form__actions">
        <el-button size="small" @click="closeForm">取消</el-button>
        <el-button type="primary" size="small" :loading="submitting" @click="submit">保存</el-button>
      </div>
    </div>

    <div v-loading="loading" class="ar-list kf-scroll">
      <div v-if="!list.length && !loading" class="ar-empty">
        还没有自动回复规则，点「新增」建一条关键词规则
      </div>
      <div v-for="row in list" :key="row.id" class="ar-item" :class="{ 'is-off': !row.enabled }">
        <div class="ar-item__main">
          <div class="ar-item__head">
            <span class="ar-item__name kf-ellipsis">{{ row.name }}</span>
            <el-tag v-if="row.shared" size="small" type="info" effect="plain">公共</el-tag>
            <el-tag v-else size="small" effect="plain">{{ row.owner || '我的' }}</el-tag>
            <el-tag v-if="row.transfer_to_human" size="small" type="warning" effect="plain">转人工</el-tag>
          </div>
          <div class="ar-item__meta">
            <el-tag size="small" effect="plain">{{ matchTypeLabel(row.match_type) }}</el-tag>
            <el-tag size="small" type="success" effect="plain">
              {{ row.channel_id ? row.channel_name || '指定渠道' : '全渠道' }}
            </el-tag>
            <span class="ar-item__hit">命中 {{ row.hit_count ?? 0 }} 次</span>
          </div>
          <div class="ar-item__kw">
            <el-tag v-for="kw in keywordTags(row.keywords)" :key="kw" size="small" effect="plain">
              {{ kw }}
            </el-tag>
            <span v-if="!keywordTags(row.keywords).length" class="ar-item__muted">--</span>
          </div>
          <div v-if="!row.transfer_to_human" class="ar-item__reply">{{ row.reply }}</div>
        </div>
        <div class="ar-item__side">
          <el-tooltip
            content="公共规则仅管理员可修改"
            placement="top"
            :disabled="row.editable"
          >
            <span class="ar-item__switch">
              <el-switch
                :model-value="row.enabled"
                :disabled="!row.editable"
                @update:model-value="(val: boolean) => toggleEnabled(row, val)"
              />
            </span>
          </el-tooltip>
          <div class="ar-item__actions">
            <el-tooltip content="公共规则仅管理员可修改" placement="top" :disabled="row.editable">
              <span>
                <el-button link type="primary" :disabled="!row.editable" @click="openEdit(row)">
                  编辑
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip content="公共规则仅管理员可修改" placement="top" :disabled="row.editable">
              <span>
                <el-button link type="danger" :disabled="!row.editable" @click="remove(row)">
                  删除
                </el-button>
              </span>
            </el-tooltip>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.ar-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;

  &__count {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.ar-tip {
  margin-bottom: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  background: #f4f6f8;
  font-size: 12px;
  line-height: 1.6;
  color: #5f6b7a;
}

.ar-form {
  margin-bottom: 12px;
  padding: 10px 12px;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  background: #fafbfc;

  &__title {
    margin-bottom: 8px;
    font-size: 13px;
    font-weight: 600;
    color: #191919;
  }

  &__tip {
    margin-left: 8px;
    font-size: 12px;
    color: #9c9c9c;
  }

  &__actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }

  :deep(.el-form-item) {
    margin-bottom: 10px;
  }
}

.ar-list {
  max-height: 42vh;
  overflow-y: auto;
}

.ar-empty {
  padding: 24px 8px;
  text-align: center;
  font-size: 13px;
  color: #9c9c9c;
}

.ar-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 4px;
  border-bottom: 1px solid #f2f3f5;

  &:last-child {
    border-bottom: none;
  }

  &.is-off {
    opacity: 0.62;
  }

  &__main {
    flex: 1;
    min-width: 0;
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  &__name {
    font-size: 14px;
    font-weight: 600;
    color: #191919;
    max-width: 50%;
  }

  &__meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-top: 6px;
    font-size: 12px;
    color: #7f7f7f;
  }

  &__hit {
    color: #9c9c9c;
  }

  &__kw {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 6px;
  }

  &__muted {
    color: #b2b2b2;
  }

  &__reply {
    margin-top: 6px;
    font-size: 13px;
    line-height: 1.6;
    color: #5f6b7a;
    word-break: break-word;
    white-space: pre-wrap;
    max-height: 3.2em;
    overflow: hidden;
  }

  &__side {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 6px;
    flex-shrink: 0;
  }

  &__actions {
    display: flex;
    align-items: center;
    gap: 2px;
  }
}

/* ---------------- 移动端（≤768px） ---------------- */
@media (max-width: 768px) {
  .ar-list {
    max-height: 38vh;
  }

  .ar-item {
    gap: 6px;

    &__name {
      max-width: 42%;
    }

    &__actions {
      flex-direction: column;
      align-items: flex-end;
      gap: 0;

      :deep(.el-button) {
        padding: 2px 0;
        height: auto;
      }
    }
  }

  .ar-form {
    padding: 8px 10px;

    &__tip {
      display: block;
      margin: 2px 0 0;
    }
  }
}

/* 窄屏不出现横向溢出 */
:global(.auto-reply-dialog) {
  max-width: 100%;
}

:global(.auto-reply-dialog .el-dialog__body) {
  padding: 12px 16px;
  overflow-x: hidden;
}
</style>
