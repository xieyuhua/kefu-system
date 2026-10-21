<script setup lang="ts">
/**
 * 知识库管理（后台端）。
 *
 * 这里维护的是「AI 应答的知识来源」：机器人未命中自动回复规则时，
 * 后端会用问题去检索这些条目（标题 + 标签 + 正文），把命中的片段拼进提示词交给大模型。
 *
 * 所以本页的三件事直接影响回答质量：
 *   1. 标题写清「这条知识解决什么问题」（检索按标题命中加权最高）；
 *   2. 标签用逗号分隔的关键词（如 退货,退款），问题里出现标签时指向性最强；
 *   3. 正文写可直接复述给客户的话术，模型会照它组织回答。
 *
 * 停用（enabled = false）的条目不会参与检索：临时下架政策时改这里即可。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Refresh, Search } from '@element-plus/icons-vue'
import {
  createKnowledge,
  deleteKnowledge,
  listKnowledge,
  updateKnowledge,
  type KnowledgeDoc,
} from '@/shared/api/admin'
import { useIsMobile } from '@/shared/composables/useIsMobile'

const { isMobile } = useIsMobile()

const loading = ref(false)
const submitting = ref(false)
const list = ref<KnowledgeDoc[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')

const drawerVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

interface KnowledgeForm {
  title: string
  content: string
  tags: string
  enabled: boolean
}

function emptyForm(): KnowledgeForm {
  return { title: '', content: '', tags: '', enabled: true }
}

const form = reactive<KnowledgeForm>(emptyForm())

const rules: FormRules<KnowledgeForm> = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  content: [{ required: true, message: '请输入知识内容', trigger: 'blur' }],
}

const drawerTitle = computed(() => (editingId.value === null ? '新增知识' : '编辑知识'))

/** 标签在列表里拆成一个个小标签展示（空标签返回空数组） */
function tagList(tags: string): string[] {
  return (tags || '')
    .split(/[,，;；\n]/)
    .map((t) => t.trim())
    .filter(Boolean)
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const paged = await listKnowledge({
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    list.value = paged.list || []
    total.value = paged.total || 0
  } catch {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

/** 搜索：回到第一页再查 */
function search(): void {
  page.value = 1
  void load()
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  drawerVisible.value = true
  formRef.value?.clearValidate()
}

function openEdit(row: KnowledgeDoc): void {
  editingId.value = row.id
  Object.assign(form, {
    title: row.title,
    content: row.content,
    tags: row.tags || '',
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
    tags: form.tags.trim(),
    enabled: form.enabled,
  }

  submitting.value = true
  try {
    if (editingId.value === null) {
      await createKnowledge(payload)
      ElMessage.success('知识已创建')
    } else {
      await updateKnowledge(editingId.value, payload)
      ElMessage.success('知识已更新')
    }
    drawerVisible.value = false
    await load()
  } catch {
    // 错误已由拦截器提示
  } finally {
    submitting.value = false
  }
}

/** 列表里的启用开关：直接落库（失败则重新拉取，避免状态与服务端不一致） */
async function toggleEnabled(row: KnowledgeDoc, value: boolean): Promise<void> {
  try {
    await updateKnowledge(row.id, {
      title: row.title,
      content: row.content,
      tags: row.tags || '',
      enabled: value,
    })
    row.enabled = value
    ElMessage.success(value ? '已启用' : '已停用（不再参与检索）')
  } catch {
    await load()
  }
}

async function remove(row: KnowledgeDoc): Promise<void> {
  try {
    await ElMessageBox.confirm(`删除「${row.title}」后不可恢复，确认删除？`, '删除知识', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await deleteKnowledge(row.id)
    ElMessage.success('已删除')
    // 删掉当前页最后一条时回退一页，避免停在空页
    if (list.value.length === 1 && page.value > 1) page.value -= 1
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
          知识库
          <span class="toolbar__count">共 {{ total }} 条</span>
        </div>
        <div class="toolbar__actions">
          <el-input
            v-model="keyword"
            class="search-input"
            size="small"
            clearable
            placeholder="搜索标题 / 内容 / 标签"
            data-test="knowledge-search"
            @keyup.enter="search"
            @clear="search"
          />
          <el-button size="small" :icon="Search" data-test="knowledge-search-btn" @click="search">
            搜索
          </el-button>
          <el-button size="small" :icon="Refresh" @click="load">刷新</el-button>
          <el-button
            type="primary"
            size="small"
            :icon="Plus"
            data-test="knowledge-create"
            @click="openCreate"
          >
            新增知识
          </el-button>
        </div>
      </div>

      <div class="tip">
        这里的条目是<b>机器人 AI 应答的资料库</b>：客户问题未命中自动回复规则时，
        后端会检索这些内容并交给大模型组织回答。标题写清「解决什么问题」、标签写清同义关键词，
        检索命中率最高；停用的条目不会参与检索。
      </div>

      <el-table
        v-if="!isMobile"
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无知识条目"
      >
        <el-table-column prop="title" label="标题" min-width="160" show-overflow-tooltip />
        <el-table-column label="标签" min-width="140">
          <template #default="{ row }">
            <span v-if="tagList(row.tags).length" class="tag-row">
              <el-tag
                v-for="tag in tagList(row.tags)"
                :key="tag"
                size="small"
                effect="plain"
                class="tag-row__item"
              >
                {{ tag }}
              </el-tag>
            </span>
            <span v-else class="muted">--</span>
          </template>
        </el-table-column>
        <el-table-column label="内容" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="content">{{ row.content }}</span>
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
        <el-table-column label="更新时间" width="170" align="center">
          <template #default="{ row }">
            <span class="muted">{{ row.updated_at || '--' }}</span>
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
        <div v-for="row in list" :key="row.id" class="m-card" data-test="knowledge-card">
          <div class="m-card__top">
            <span class="m-card__title">{{ row.title }}</span>
            <span class="m-card__tags">
              <el-tag
                v-for="tag in tagList(row.tags)"
                :key="tag"
                size="small"
                effect="plain"
                class="tag-row__item"
              >
                {{ tag }}
              </el-tag>
            </span>
          </div>

          <div class="m-field">
            <span class="m-field__label">内容</span>
            <span class="m-field__value">{{ row.content }}</span>
          </div>

          <div class="m-field">
            <span class="m-field__label">更新时间</span>
            <span class="m-field__value muted">{{ row.updated_at || '--' }}</span>
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

        <el-empty v-if="!loading && !list.length" description="暂无知识条目" :image-size="60" />
      </div>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50]"
          :layout="isMobile ? 'prev, pager, next' : 'total, sizes, prev, pager, next'"
          :pager-count="isMobile ? 5 : 7"
          small
          background
          @current-change="load"
          @size-change="search"
        />
      </div>
    </div>

    <el-drawer
      v-model="drawerVisible"
      :title="drawerTitle"
      :size="isMobile ? '100%' : '520px'"
      class="knowledge-drawer"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isMobile ? '64px' : '76px'"
      >
        <el-form-item label="标题" prop="title">
          <el-input
            v-model="form.title"
            placeholder="如：退货政策 / 发货时效"
            maxlength="128"
            show-word-limit
            data-test="knowledge-title"
          />
        </el-form-item>

        <el-form-item label="标签">
          <el-input
            v-model="form.tags"
            placeholder="逗号分隔，如：退货,退款,售后"
            maxlength="128"
            show-word-limit
            data-test="knowledge-tags"
          />
          <div class="form-tip">客户问题里出现标签时，这条知识的检索分最高</div>
        </el-form-item>

        <el-form-item label="内容" prop="content">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="8"
            maxlength="5000"
            show-word-limit
            placeholder="写可直接复述给客户的口径，例如：支持 7 天无理由退货，签收后 7 天内可申请……"
            data-test="knowledge-content"
          />
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" data-test="knowledge-enabled" />
          <div class="form-tip">停用后不参与 AI 检索（临时下架政策时用）</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-foot">
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="submitting"
            data-test="knowledge-submit"
            @click="submit"
          >
            保存
          </el-button>
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
  gap: 8px;
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

  &__actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }
}

.search-input {
  width: 220px;
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

.muted {
  color: #909399;
}

.tag-row {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;

  &__item {
    margin: 0;
  }
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

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

/* ---------------- 移动端（≤768px，与 useIsMobile 断点一致） ---------------- */
@media (max-width: 768px) {
  .toolbar {
    flex-direction: column;
    align-items: flex-start;
    margin-bottom: 10px;
  }

  .toolbar__actions {
    width: 100%;
    justify-content: flex-start;
  }

  /* 搜索框占满剩余宽度，按钮换行不撑出横向滚动 */
  .search-input {
    width: 100%;
    min-width: 0;
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

  .pager {
    justify-content: center;
  }

  /* 全屏抽屉：表单可滚动，底部按钮固定可见 */
  :global(.knowledge-drawer .el-drawer__body) {
    padding: 12px 16px;
    overflow-y: auto;
  }

  :global(.knowledge-drawer .el-drawer__footer) {
    padding: 10px 16px calc(10px + env(safe-area-inset-bottom, 0));
    border-top: 1px solid #f0f0f0;
  }

  :global(.knowledge-drawer .el-form-item) {
    margin-bottom: 14px;
  }
}
</style>
