<script setup lang="ts">
/**
 * 快捷回复管理对话框（客服工作台）。
 *
 * 能力：
 *   - 列表：标题 + 内容预览 + 归属标签（公共 = 租户共享，所有人可见但只读）；
 *   - 新增 / 编辑：标题、内容、排序、启用；
 *   - 删除：自己的私有项可删，公共项与别人的私有项按钮置灰 + tooltip 说明。
 *
 * 数据来自 /api/quick-replies（我的私有 + 租户共享），后端已按账号过滤，
 * 前端只按 editable 决定按钮是否可用（越权请求后端同样会以中文业务错误拒绝）。
 */
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  createQuickReply,
  deleteQuickReply,
  listQuickReplies,
  updateQuickReply,
  type QuickReply,
} from '@/shared/api/quickReplies'
import { useIsMobile } from '@/shared/composables/useIsMobile'

const visible = defineModel<boolean>({ default: false })

const emit = defineEmits<{
  /** 列表发生增删改后通知父级刷新（输入框弹层里的快捷回复同步更新） */
  changed: []
}>()

/** 窄屏（≤768px）：对话框收窄、表单项纵向排列 */
const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const list = ref<QuickReply[]>([])

/** 表单区是否展开（新增 / 编辑共用同一块内联表单，避免再套一层弹窗） */
const formOpen = ref(false)
const editingId = ref<number | null>(null)

interface QuickReplyForm {
  title: string
  content: string
  sort: number
  enabled: boolean
}

function emptyForm(): QuickReplyForm {
  return { title: '', content: '', sort: 0, enabled: true }
}

const form = reactive<QuickReplyForm>(emptyForm())

const formTitle = computed(() => (editingId.value === null ? '新增快捷回复' : '编辑快捷回复'))

/** 列表里没有可用项时的空态文案 */
const emptyText = computed(() =>
  loading.value ? '加载中…' : '还没有快捷回复，点「新增」写一条常用话术',
)

async function load(): Promise<void> {
  loading.value = true
  try {
    list.value = await listQuickReplies()
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  // 新增排到末尾，省得和已有话术抢同一个 sort
  form.sort = list.value.length ? Math.max(...list.value.map((q) => q.sort)) + 10 : 10
  formOpen.value = true
}

function openEdit(row: QuickReply): void {
  if (!row.editable) {
    ElMessage.warning(row.shared ? '公共话术仅管理员可修改' : '只能修改自己的快捷回复')
    return
  }
  editingId.value = row.id
  Object.assign(form, {
    title: row.title,
    content: row.content,
    sort: row.sort ?? 0,
    enabled: !!row.enabled,
  })
  formOpen.value = true
}

function closeForm(): void {
  formOpen.value = false
  editingId.value = null
  Object.assign(form, emptyForm())
}

async function submit(): Promise<void> {
  const title = form.title.trim()
  const content = form.content.trim()
  if (!title) {
    ElMessage.warning('请输入标题')
    return
  }
  if (!content) {
    ElMessage.warning('请输入回复内容')
    return
  }

  submitting.value = true
  try {
    const payload = {
      title,
      content,
      sort: Number(form.sort) || 0,
      enabled: form.enabled,
    }
    if (editingId.value === null) {
      await createQuickReply(payload)
      ElMessage.success('快捷回复已创建')
    } else {
      await updateQuickReply(editingId.value, payload)
      ElMessage.success('快捷回复已更新')
    }
    closeForm()
    await load()
    emit('changed')
  } catch {
    // 错误已由 http 拦截器提示（如「只能修改自己的快捷回复」）
  } finally {
    submitting.value = false
  }
}

async function remove(row: QuickReply): Promise<void> {
  if (!row.editable) {
    ElMessage.warning(row.shared ? '公共话术仅管理员可修改' : '只能删除自己的快捷回复')
    return
  }
  try {
    await ElMessageBox.confirm(`删除「${row.title}」后不可恢复，确认删除？`, '删除快捷回复', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteQuickReply(row.id)
    ElMessage.success('快捷回复已删除')
    if (editingId.value === row.id) closeForm()
    await load()
    emit('changed')
  } catch {
    // 错误已由拦截器提示
  }
}

/** 打开对话框时刷新一次（别处改过也能看到最新数据） */
watch(visible, (open) => {
  if (!open) {
    closeForm()
    return
  }
  void load()
})
</script>

<template>
  <el-dialog
    v-model="visible"
    title="快捷回复管理"
    :width="isMobile ? '94%' : '560px'"
    :close-on-click-modal="false"
    append-to-body
    class="quick-reply-dialog"
  >
    <div class="qr-toolbar">
      <span class="qr-toolbar__count">共 {{ list.length }} 条（含公共话术）</span>
      <el-button type="primary" size="small" :icon="Plus" @click="openCreate">新增</el-button>
    </div>

    <!-- 内联表单：新增 / 编辑共用，窄屏字段纵向排列 -->
    <div v-if="formOpen" class="qr-form">
      <div class="qr-form__title">{{ formTitle }}</div>
      <el-form
        :model="form"
        :label-position="isMobile ? 'top' : 'right'"
        :label-width="isMobile ? undefined : '64px'"
        @submit.prevent
      >
        <el-form-item label="标题" required>
          <el-input v-model="form.title" maxlength="32" show-word-limit placeholder="如：问候语" />
        </el-form-item>
        <el-form-item label="内容" required>
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="3"
            maxlength="500"
            show-word-limit
            placeholder="点选后插入输入框的正文"
          />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :max="9999" controls-position="right" />
          <span class="qr-form__tip">数字小的排前面</span>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
          <span class="qr-form__tip">停用后不在工作台弹层里出现</span>
        </el-form-item>
      </el-form>
      <div class="qr-form__actions">
        <el-button size="small" @click="closeForm">取消</el-button>
        <el-button type="primary" size="small" :loading="submitting" @click="submit">保存</el-button>
      </div>
    </div>

    <div v-loading="loading" class="qr-list kf-scroll">
      <div v-if="!list.length && !loading" class="qr-empty">{{ emptyText }}</div>
      <div v-for="row in list" :key="row.id" class="qr-item" :class="{ 'is-off': !row.enabled }">
        <div class="qr-item__main">
          <div class="qr-item__head">
            <span class="qr-item__title kf-ellipsis">{{ row.title }}</span>
            <el-tag v-if="row.shared" size="small" type="info" effect="plain">公共</el-tag>
            <el-tag v-else size="small" effect="plain">{{ row.owner || '我的' }}</el-tag>
            <el-tag v-if="!row.enabled" size="small" type="warning" effect="plain">已停用</el-tag>
          </div>
          <div class="qr-item__content">{{ row.content }}</div>
        </div>
        <div class="qr-item__actions">
          <el-tooltip
            :content="row.shared ? '公共话术仅管理员可修改' : '只能修改自己的快捷回复'"
            placement="top"
            :disabled="row.editable"
          >
            <span>
              <el-button link type="primary" :disabled="!row.editable" @click="openEdit(row)">
                编辑
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip
            :content="row.shared ? '公共话术仅管理员可修改' : '只能删除自己的快捷回复'"
            placement="top"
            :disabled="row.editable"
          >
            <span>
              <el-button link type="danger" :disabled="!row.editable" @click="remove(row)">
                删除
              </el-button>
            </span>
          </el-tooltip>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.qr-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;

  &__count {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.qr-form {
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

.qr-list {
  max-height: 44vh;
  overflow-y: auto;
}

.qr-empty {
  padding: 24px 8px;
  text-align: center;
  font-size: 13px;
  color: #9c9c9c;
}

.qr-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 4px;
  border-bottom: 1px solid #f2f3f5;

  &:last-child {
    border-bottom: none;
  }

  &.is-off {
    opacity: 0.6;
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

  &__title {
    font-size: 14px;
    font-weight: 600;
    color: #191919;
    max-width: 46%;
  }

  &__content {
    margin-top: 4px;
    font-size: 13px;
    line-height: 1.6;
    color: #5f6b7a;
    word-break: break-word;
    white-space: pre-wrap;
    max-height: 3.2em;
    overflow: hidden;
  }

  &__actions {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    gap: 2px;
  }
}

/* ---------------- 移动端（≤768px） ---------------- */
@media (max-width: 768px) {
  .qr-list {
    max-height: 42vh;
  }

  .qr-item {
    gap: 6px;

    &__title {
      max-width: 40%;
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

  .qr-form {
    padding: 8px 10px;

    &__tip {
      display: block;
      margin: 2px 0 0;
    }
  }
}

/* 对话框本体：窄屏时不出现横向溢出 */
:global(.quick-reply-dialog) {
  max-width: 100%;
}

:global(.quick-reply-dialog .el-dialog__body) {
  padding: 12px 16px;
  overflow-x: hidden;
}
</style>
