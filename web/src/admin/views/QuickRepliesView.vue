<script setup lang="ts">
/**
 * 快捷回复管理（后台端）。
 *
 * 这里维护的是「租户公共话术」（user_id = 0）：所有客服在工作台都能看到，但只有管理员能改。
 * 列表同时会带出客服自己建的私有话术（user_id > 0），管理员可以统一维护 / 清理。
 * 客服在工作台增删改自己的话术走 /api/quick-replies，与本页共用同一套数据。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  createTenantQuickReply,
  deleteTenantQuickReply,
  listTenantQuickReplies,
  updateTenantQuickReply,
  type QuickReply,
} from '@/shared/api/quickReplies'
import { useIsMobile } from '@/shared/composables/useIsMobile'

const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const list = ref<QuickReply[]>([])

const drawerVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

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

const rules: FormRules<QuickReplyForm> = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  content: [{ required: true, message: '请输入回复内容', trigger: 'blur' }],
}

const drawerTitle = computed(() => (editingId.value === null ? '新增公共话术' : '编辑快捷回复'))
/** 正在编辑的话术归属（编辑已有项时只做展示，不允许在此页转移归属） */
const editingOwner = computed(() => {
  if (editingId.value === null) return '公共'
  const row = list.value.find((q) => q.id === editingId.value)
  if (!row) return '公共'
  return row.shared ? '公共' : row.owner || '客服私有'
})

async function load(): Promise<void> {
  loading.value = true
  try {
    list.value = await listTenantQuickReplies()
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  // 公共话术排到末尾，避免和已有话术抢同一个 sort
  form.sort = list.value.length ? Math.max(...list.value.map((q) => q.sort)) + 10 : 10
  drawerVisible.value = true
  formRef.value?.clearValidate()
}

function openEdit(row: QuickReply): void {
  editingId.value = row.id
  Object.assign(form, {
    title: row.title,
    content: row.content,
    sort: row.sort ?? 0,
    enabled: !!row.enabled,
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
    title: form.title.trim(),
    content: form.content.trim(),
    sort: Number(form.sort) || 0,
    enabled: form.enabled,
  }

  submitting.value = true
  try {
    if (editingId.value === null) {
      // 管理员新建默认落到公共话术（user_id = 0）
      await createTenantQuickReply({ ...payload, user_id: 0 })
      ElMessage.success('公共话术已创建')
    } else {
      await updateTenantQuickReply(editingId.value, payload)
      ElMessage.success('快捷回复已更新')
    }
    drawerVisible.value = false
    await load()
  } catch {
    // 错误已由拦截器提示
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(row: QuickReply, value: boolean): Promise<void> {
  try {
    await updateTenantQuickReply(row.id, {
      title: row.title,
      content: row.content,
      sort: row.sort,
      enabled: value,
    })
    row.enabled = value
    ElMessage.success(value ? '已启用' : '已停用')
  } catch {
    await load()
  }
}

async function remove(row: QuickReply): Promise<void> {
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
    await deleteTenantQuickReply(row.id)
    ElMessage.success('已删除')
    await load()
  } catch {
    // 错误已由拦截器提示
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          快捷回复
          <span class="toolbar__count">共 {{ list.length }} 条</span>
        </div>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增公共话术</el-button>
      </div>

      <div class="tip">
        这里维护的是<b>公共话术</b>：所有客服在工作台的「快捷回复」里都能看到并一键插入，但只有管理员能改。
        客服自己新增的话术归其本人所有（列表里显示创建人），管理员同样可以在这里统一维护。
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无快捷回复"
      >
        <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
        <el-table-column label="内容" min-width="280" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="content">{{ row.content }}</span>
          </template>
        </el-table-column>
        <el-table-column label="归属" width="110" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.shared" size="small" type="info" effect="plain">公共</el-tag>
            <span v-else>{{ row.owner || '--' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="80" align="center" sortable />
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              @update:model-value="(val: boolean) => toggleEnabled(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：卡片列表 -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in list" :key="row.id" class="m-card">
          <div class="m-card__top">
            <span class="m-card__title">{{ row.title }}</span>
            <span class="m-card__tags">
              <el-tag v-if="row.shared" size="small" type="info" effect="plain">公共</el-tag>
              <el-tag v-else size="small" effect="plain">{{ row.owner || '--' }}</el-tag>
            </span>
          </div>

          <div class="m-field">
            <span class="m-field__label">内容</span>
            <span class="m-field__value">{{ row.content }}</span>
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

        <el-empty v-if="!loading && !list.length" description="暂无快捷回复" :image-size="60" />
      </div>
    </div>

    <el-drawer
      v-model="drawerVisible"
      :title="drawerTitle"
      :size="isMobile ? '100%' : '440px'"
      class="quick-reply-drawer"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isMobile ? '72px' : '88px'"
      >
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="如：问候语" maxlength="32" show-word-limit />
        </el-form-item>

        <el-form-item label="归属">
          <span class="readonly">{{ editingOwner }}</span>
          <div class="form-tip">在此页新建的话术一律是公共话术；客服的私有话术请让本人在工作台维护。</div>
        </el-form-item>

        <el-form-item label="内容" prop="content">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="4"
            maxlength="500"
            show-word-limit
            placeholder="客服点选后插入输入框的正文"
          />
        </el-form-item>

        <el-form-item label="排序" prop="sort">
          <el-input-number v-model="form.sort" :min="0" :max="9999" />
          <div class="form-tip">数字小的排前面</div>
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
          <div class="form-tip">停用后不在工作台快捷回复弹层里出现</div>
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
  line-height: 1.6;
  color: #7f7f7f;

  b {
    color: #191919;
  }
}

.content {
  color: #5f6b7a;
}

.readonly {
  color: #191919;
}

.form-tip {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: #7f7f7f;
}

.drawer-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
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

  /* 全屏抽屉：表单可滚动，底部按钮固定可见 */
  :global(.quick-reply-drawer .el-drawer__body) {
    padding: 12px 16px;
    overflow-y: auto;
  }

  :global(.quick-reply-drawer .el-drawer__footer) {
    padding: 10px 16px calc(10px + env(safe-area-inset-bottom, 0));
    border-top: 1px solid #f0f0f0;
  }

  :global(.quick-reply-drawer .el-form-item) {
    margin-bottom: 14px;
  }
}
</style>
