<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  createChannelType,
  deleteChannelType,
  listChannelTypes,
  updateChannelType,
  type ChannelTypeItem,
} from '@/shared/api/admin'
import { useIsMobile } from '@/shared/composables/useIsMobile'

/** 窄屏（≤768px）改用卡片列表 */
const { isMobile } = useIsMobile()

const ICON_PRESETS = ['💬', '🏢', '🌐', '📱', '📣', '🎵', '🤖', '📒', '✉️', '⚡', '🛍', '🎮']

const loading = ref(false)
const submitting = ref(false)
const list = ref<ChannelTypeItem[]>([])

const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

interface ChannelTypeForm {
  code: string
  name: string
  color: string
  icon: string
  sort: number
}

function emptyForm(): ChannelTypeForm {
  return { code: '', name: '', color: '#07C160', icon: '💬', sort: 0 }
}

const form = reactive<ChannelTypeForm>(emptyForm())

const rules: FormRules<ChannelTypeForm> = {
  code: [{ required: true, message: '请输入类型标识 code', trigger: 'blur' }],
  name: [{ required: true, message: '请输入类型名称', trigger: 'blur' }],
}

async function load(): Promise<void> {
  loading.value = true
  try {
    list.value = await listChannelTypes()
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  dialogVisible.value = true
  formRef.value?.clearValidate()
}

function openEdit(row: ChannelTypeItem): void {
  editingId.value = row.id
  Object.assign(form, {
    code: row.code,
    name: row.name,
    color: row.color || '#07C160',
    icon: row.icon || '💬',
    sort: row.sort ?? 0,
  })
  dialogVisible.value = true
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
    code: form.code.trim(),
    name: form.name.trim(),
    color: form.color,
    icon: form.icon.trim(),
    sort: Number(form.sort) || 0,
  }

  submitting.value = true
  try {
    if (editingId.value === null) {
      await createChannelType(payload)
      ElMessage.success('渠道类型已创建')
    } else {
      await updateChannelType(editingId.value, payload)
      ElMessage.success('渠道类型已更新')
    }
    dialogVisible.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function remove(row: ChannelTypeItem): Promise<void> {
  // used_count > 0 时后端直接报错，这里先给出明确提示
  if ((row.used_count ?? 0) > 0) {
    ElMessage.warning(`「${row.name}」已被 ${row.used_count} 个渠道使用，无法删除`)
    return
  }
  try {
    await ElMessageBox.confirm(`删除渠道类型「${row.name}」后不可恢复，确认删除？`, '删除类型', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteChannelType(row.id)
    ElMessage.success('渠道类型已删除')
    await load()
  } catch (err) {
    if (err instanceof Error && err.message) ElMessage.error(err.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          渠道类型库
          <span class="toolbar__count">共 {{ list.length }} 个类型</span>
        </div>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增类型</el-button>
      </div>

      <div class="tip">
        渠道类型完全自定义、不限制枚举：新增任意类型后，渠道管理页即可选用它来新建渠道。
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无渠道类型"
      >
        <el-table-column label="图标" width="70" align="center">
          <template #default="{ row }">
            <span class="type-icon" :style="{ color: row.color }">{{ row.icon || '💬' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="名称" min-width="160">
          <template #default="{ row }">
            <span class="type-name">{{ row.name }}</span>
            <el-tag v-if="row.built_in" size="small" effect="plain" class="type-tag">内置</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="标识 code" min-width="140" show-overflow-tooltip />
        <el-table-column label="配色" width="90" align="center">
          <template #default="{ row }">
            <span class="type-color">
              <i :style="{ background: row.color }"></i>
              <span>{{ row.color }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="80" align="center" sortable />
        <el-table-column label="引用渠道数" width="110" align="center">
          <template #default="{ row }">
            <span :class="{ 'in-use': (row.used_count ?? 0) > 0 }">{{ row.used_count ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="160" />
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button
              link
              type="danger"
              :disabled="!!row.built_in || (row.used_count ?? 0) > 0"
              @click="remove(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 窄屏：渠道类型卡片列表 -->
      <div v-else v-loading="loading" class="m-list">
        <div v-for="row in list" :key="row.id" class="m-card">
          <div class="m-card__top">
            <span class="type-icon" :style="{ color: row.color }">{{ row.icon || '💬' }}</span>
            <span class="m-card__title">{{ row.name }}</span>
            <span class="m-card__tags">
              <el-tag v-if="row.built_in" size="small" effect="plain">内置</el-tag>
            </span>
          </div>

          <div class="m-card__sub">{{ row.code }}</div>

          <div class="m-card__meta">
            <span class="type-color">
              <i :style="{ background: row.color }"></i>
              <span>{{ row.color }}</span>
            </span>
            <span>排序 {{ row.sort }}</span>
            <span :class="{ 'in-use': (row.used_count ?? 0) > 0 }">
              引用 {{ row.used_count ?? 0 }}
            </span>
          </div>

          <div class="m-card__meta">
            <span>创建 {{ row.created_at }}</span>
          </div>

          <div class="m-card__foot">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button
              link
              type="danger"
              :disabled="!!row.built_in || (row.used_count ?? 0) > 0"
              @click="remove(row)"
            >
              删除
            </el-button>
          </div>
        </div>

        <el-empty v-if="!loading && !list.length" description="暂无渠道类型" :image-size="60" />
      </div>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId === null ? '新增渠道类型' : '编辑渠道类型'"
      width="480px"
      class="type-dialog"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isMobile ? '80px' : '100px'"
      >
        <el-form-item label="类型标识" prop="code">
          <el-input
            v-model="form.code"
            placeholder="英文标识，如 xiaohongshu；也可直接填中文"
            :disabled="editingId !== null"
          />
        </el-form-item>
        <el-form-item label="类型名称" prop="name">
          <el-input v-model="form.name" placeholder="如：小红书" maxlength="30" show-word-limit />
        </el-form-item>
        <el-form-item label="图标">
          <div class="icon-row">
            <el-input v-model="form.icon" class="icon-input" placeholder="填 emoji，如 📒" maxlength="8" />
            <div class="icon-presets">
              <button
                v-for="e in ICON_PRESETS"
                :key="e"
                type="button"
                class="icon-preset"
                :class="{ 'is-on': form.icon === e }"
                @click="form.icon = e"
              >
                {{ e }}
              </button>
            </div>
          </div>
        </el-form-item>
        <el-form-item label="配色">
          <el-color-picker v-model="form.color" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :max="9999" />
          <div class="form-tip">数字越小越靠前</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>
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

.type-icon {
  font-size: 18px;
  line-height: 1;
}

.type-name {
  font-size: 14px;
  color: #191919;
}

.type-tag {
  margin-left: 6px;
}

.type-color {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #7f7f7f;

  i {
    display: inline-block;
    width: 14px;
    height: 14px;
    border-radius: 3px;
    border: 1px solid #e5e5e5;
  }
}

.in-use {
  color: #e6162d;
  font-weight: 600;
}

.icon-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.icon-input {
  width: 160px;
}

.icon-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.icon-preset {
  width: 30px;
  height: 30px;
  border: 1px solid #e5e5e5;
  border-radius: 4px;
  background: #fff;
  font-size: 16px;
  cursor: pointer;

  &.is-on {
    border-color: #07c160;
    background: rgba(7, 193, 96, 0.08);
  }
}

.form-tip {
  width: 100%;
  font-size: 12px;
  color: #7f7f7f;
}

/* ---------------- 移动端（≤768px，与 useIsMobile 断点一致） ---------------- */
@media (max-width: 768px) {
  .toolbar {
    margin-bottom: 10px;
  }

  .type-color {
    gap: 4px;
  }

  /* 图标预设按钮在窄屏放大到 34px，便于点按 */
  .icon-preset {
    width: 34px;
    height: 34px;
  }

  .icon-input {
    width: 100%;
  }
}
</style>
