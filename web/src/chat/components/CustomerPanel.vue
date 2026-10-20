<template>
  <aside class="customer-panel" :class="{ 'customer-panel--embedded': embedded }">
    <header class="panel-header">
      <span>客户资料</span>
      <el-button text size="small" :icon="Close" @click="emit('close')" />
    </header>

    <div v-loading="loading" class="panel-body kf-scroll">
      <div v-if="!customer" class="kf-empty">
        <span>暂无客户信息</span>
      </div>

      <template v-else>
        <div class="profile">
          <div class="profile-avatar">
            <img v-if="customer.avatar" :src="customer.avatar" alt="" />
            <span v-else>{{ (customer.nickname || '?').slice(0, 1) }}</span>
          </div>
          <div class="profile-name">{{ customer.nickname }}</div>
          <div class="profile-channel">
            <el-tag size="small" effect="light" :color="channelColor(customer.channel?.type)">
              {{ channelLabel(customer.channel?.type) }}
            </el-tag>
          </div>

          <div class="profile-tags">
            <el-tag
              v-for="t in tags"
              :key="t.id"
              size="small"
              :color="t.color || undefined"
              effect="plain"
            >
              {{ t.name }}
            </el-tag>
            <el-button
              text
              size="small"
              class="tag-add"
              title="编辑标签 / 备注"
              @click="openTagEditor"
            >
              ＋ 标签备注
            </el-button>
          </div>

          <div v-if="customer.remark" class="profile-remark">
            <span class="profile-remark__label">备注</span>
            <span class="profile-remark__text">{{ customer.remark }}</span>
          </div>

          <ul class="profile-meta">
            <li v-if="customer.phone"><span>手机</span>{{ customer.phone }}</li>
            <li v-if="customer.external_id"><span>ID</span>{{ customer.external_id }}</li>
            <li><span>归属地</span>{{ customer.city || '未知' }}</li>
            <li><span>IP</span>{{ customer.ip || '未知' }}</li>
            <li><span>订单</span>{{ customer.order_count ?? 0 }} 笔</li>
            <li><span>会话</span>{{ customer.conversation_count ?? 0 }} 次</li>
          </ul>
        </div>

        <div class="panel-section">
          <div class="kf-section-title">历史订单（{{ orders.length }}）</div>
          <div v-if="!orders.length" class="panel-empty">暂无订单</div>
          <div
            v-for="o in orders"
            :key="o.id"
            class="order-item"
            :class="{ disabled: !canSend || !orderCardEnabled }"
            :title="orderCardEnabled ? '' : orderCardHint"
            @click="sendOrder(o)"
          >
            <div class="order-thumb">
              <img v-if="o.image" :src="o.image" alt="" />
              <span v-else>📦</span>
            </div>
            <div class="order-info">
              <div class="order-title kf-ellipsis">{{ o.title }}</div>
              <div class="order-sub">
                <span>{{ formatMoney(o.amount) }}</span>
                <span>{{ o.status }}</span>
              </div>
              <div class="order-no">{{ o.order_no }}</div>
            </div>
            <el-tooltip
              :content="orderCardEnabled ? '发送给客户' : orderCardHint"
              placement="left"
            >
              <span class="send-icon">➤</span>
            </el-tooltip>
          </div>
        </div>
      </template>
    </div>

    <!-- 标签 / 备注编辑弹窗 -->
    <el-dialog
      v-model="tagEditorVisible"
      :title="`标签备注 · ${customer?.nickname || ''}`"
      width="440px"
      append-to-body
      :close-on-click-modal="false"
    >
      <!-- 备注 -->
      <div class="remark-editor">
        <div class="remark-editor__head">
          <span class="kf-section-title">客服备注</span>
          <span class="remark-editor__hint">交接班后其他客服可见</span>
        </div>
        <el-input
          v-model="remarkDraft"
          type="textarea"
          :rows="3"
          maxlength="200"
          show-word-limit
          placeholder="记录客户情况，如：已退款、偏好顺丰、脾气较急…"
        />
      </div>

      <!-- 标签选择 -->
      <div class="tag-editor">
        <div class="tag-editor__head">
          <span class="kf-section-title">客户标签</span>
          <el-input
            v-model="newTagName"
            size="small"
            placeholder="新标签，回车添加"
            class="tag-editor__new"
            @keyup.enter="onCreateTag"
          />
        </div>

        <div v-loading="tagLibLoading" class="tag-editor__list">
          <el-check-tag
            v-for="t in tagLib"
            :key="t.id"
            :checked="selectedTagIds.includes(t.id)"
            class="tag-editor__item"
            @change="toggleTag(t.id)"
          >
            {{ t.name }}
          </el-check-tag>
          <div v-if="!tagLibLoading && !tagLib.length" class="panel-empty">
            暂无标签，可在上方输入新标签回车创建
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="tagEditorVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveTagsAndRemark">
          保存
        </el-button>
      </template>
    </el-dialog>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Close } from '@element-plus/icons-vue'
import type { Customer, Order } from '@/shared/types'
import { channelColor, channelLabel, formatMoney } from '@/shared/utils/format'
import { createTag, listTags, setCustomerTags, updateCustomerRemark, type Tag } from '@/shared/api/tags'

const props = defineProps<{
  customer: Customer | null
  orders: Order[]
  loading?: boolean
  canSend?: boolean
  /** 嵌入抽屉时：去掉固定栏样式，自适应容器高度 */
  embedded?: boolean
  /**
   * 当前会话所属渠道是否支持订单卡片（后台渠道配置）。
   * 不支持时历史订单上的「发送」置灰并提示原因；默认 true。
   */
  orderCardEnabled?: boolean
}>()

const emit = defineEmits<{
  close: []
  'send-order': [order: Order]
  /** 标签/备注保存后通知父级刷新客户资料 */
  updated: [customer: Customer]
}>()

const customer = computed(() => props.customer)
const orders = computed(() => props.orders)
const tags = computed(() => props.customer?.tags || [])
/** 订单卡片是否可用（未传视为可用，兼容旧调用点） */
const orderCardEnabled = computed(() => props.orderCardEnabled !== false)
const orderCardHint = '该渠道未开启订单卡片'

/* ---------------- 标签 / 备注编辑 ---------------- */
const tagEditorVisible = ref(false)
const tagLibLoading = ref(false)
const tagLib = ref<Tag[]>([])
const selectedTagIds = ref<number[]>([])
const remarkDraft = ref('')
const newTagName = ref('')
const saving = ref(false)

async function openTagEditor(): Promise<void> {
  if (!props.customer) return
  tagEditorVisible.value = true
  remarkDraft.value = props.customer.remark || ''
  selectedTagIds.value = (props.customer.tags || []).map((t) => t.id)
  newTagName.value = ''
  // 懒加载标签库
  if (!tagLib.value.length) {
    tagLibLoading.value = true
    try {
      tagLib.value = await listTags()
    } catch {
      tagLib.value = []
    } finally {
      tagLibLoading.value = false
    }
  }
}

function toggleTag(id: number): void {
  const i = selectedTagIds.value.indexOf(id)
  if (i >= 0) selectedTagIds.value.splice(i, 1)
  else selectedTagIds.value.push(id)
}

async function onCreateTag(): Promise<void> {
  const name = newTagName.value.trim()
  if (!name) return
  if (tagLib.value.some((t) => t.name === name)) {
    ElMessage.warning('标签已存在')
    return
  }
  try {
    const created = await createTag({ name })
    tagLib.value.push(created)
    selectedTagIds.value.push(created.id)
    newTagName.value = ''
    ElMessage.success(`已创建标签「${name}」`)
  } catch {
    // 错误已由拦截器提示
  }
}

async function saveTagsAndRemark(): Promise<void> {
  if (!props.customer) return
  saving.value = true
  try {
    // 备注与标签分别保存（备注失败不阻塞标签）
    const remarkChanged = remarkDraft.value !== (props.customer.remark || '')
    const tagIdsChanged =
      selectedTagIds.value.slice().sort().join(',') !==
      (props.customer.tags || []).map((t) => t.id).slice().sort().join(',')

    let newTags = props.customer.tags || []
    if (tagIdsChanged) {
      newTags = await setCustomerTags(props.customer.id, selectedTagIds.value)
    }
    if (remarkChanged) {
      await updateCustomerRemark(props.customer.id, remarkDraft.value.trim())
    }

    const updated: Customer = {
      ...props.customer,
      tags: newTags,
      remark: remarkDraft.value.trim(),
    }
    emit('updated', updated)
    ElMessage.success('已保存，交接班后其他客服可见')
    tagEditorVisible.value = false
  } catch {
    // 错误已由拦截器提示
  } finally {
    saving.value = false
  }
}

function sendOrder(order: Order): void {
  if (!props.canSend) return
  // 渠道未开启订单卡片：历史订单上的「发送」置灰，这里再兜一层
  if (!orderCardEnabled.value) {
    ElMessage.warning(orderCardHint)
    return
  }
  emit('send-order', order)
}

// 切换客户时关闭弹窗，避免状态串到下一个客户
watch(
  () => props.customer?.id,
  () => {
    tagEditorVisible.value = false
  },
)
</script>

<style scoped lang="scss">
.customer-panel {
  display: flex;
  flex-direction: column;
  width: var(--kf-panel-width);
  min-width: var(--kf-panel-width);
  background: #fff;
  border-left: 1px solid var(--kf-border);
}

/* 嵌入抽屉（移动端）：去掉固定宽度与边框，自适应容器 */
.customer-panel--embedded {
  width: 100%;
  min-width: 0;
  height: 100%;
  border-left: none;
}

/* ---------------- 平板及以下：客户资料不再占用固定栏 ---------------- */
@media (max-width: 1200px) {
  .customer-panel {
    display: none;
  }

  /* 抽屉内的实例不受此规则影响 */
  .customer-panel--embedded {
    display: flex;
  }
}

/* ---------------- 移动端 ---------------- */
@media (max-width: 768px) {
  .panel-header {
    height: 44px;
  }

  .profile-avatar {
    width: 52px;
    height: 52px;
  }
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--kf-header-height);
  padding: 0 8px 0 14px;
  border-bottom: 1px solid var(--kf-border);
  font-size: 14px;
  font-weight: 600;
}

.panel-body {
  flex: 1;
}

.profile {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 18px 14px 14px;
  border-bottom: 1px solid var(--kf-border-light);
}

.profile-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: var(--kf-radius);
  background: var(--kf-bg-active);
  color: var(--kf-text-sub);
  font-size: 22px;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.profile-name {
  margin-top: 10px;
  font-size: 15px;
  font-weight: 600;
}

.profile-channel {
  margin-top: 6px;

  :deep(.el-tag) {
    color: #fff;
    border: none;
  }
}

.profile-tags {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  align-items: center;
  gap: 4px;
  margin-top: 8px;
}

.tag-add {
  height: auto;
  padding: 2px 6px;
  font-size: 12px;
  color: var(--kf-primary);
}

.profile-remark {
  width: 100%;
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: var(--kf-radius-sm);
  background: #fff5f5;
  border: 1px solid #ffd9d9;
  font-size: 12px;
  line-height: 1.6;
  text-align: left;

  &__label {
    margin-right: 6px;
    color: #e6162d;
    font-weight: 600;
  }

  &__text {
    color: #e6162d;
    font-weight: 600;
    word-break: break-all;
  }
}

.profile-meta {
  width: 100%;
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  font-size: 13px;

  li {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 0;
    border-bottom: 1px dashed var(--kf-border-light);

    span {
      color: var(--kf-text-sub);
    }
  }
}

.panel-section {
  padding: 12px 14px;
}

.panel-empty {
  padding: 12px 0;
  color: var(--kf-text-weak);
  font-size: 12px;
  text-align: center;
}

.order-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--kf-border);
  border-radius: var(--kf-radius);
  cursor: pointer;

  & + .order-item {
    margin-top: 8px;
  }

  &:hover {
    background: var(--kf-bg-hover);
  }

  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.order-thumb {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: var(--kf-radius-sm);
  background: var(--kf-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.order-info {
  flex: 1;
  min-width: 0;
}

.order-title {
  font-size: 13px;
}

.order-sub {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.order-no {
  margin-top: 2px;
  font-size: 11px;
  color: var(--kf-text-weak);
}

.send-icon {
  flex-shrink: 0;
  color: var(--kf-primary);
  font-size: 12px;
}

/* ---------------- 标签 / 备注编辑弹窗 ---------------- */
.remark-editor {
  margin-bottom: 16px;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  &__hint {
    font-size: 11px;
    color: var(--kf-text-weak);
  }
}

.tag-editor {
  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 10px;
  }

  &__new {
    width: 180px;
  }

  &__list {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    min-height: 40px;
  }

  &__item {
    margin-right: 0;
  }
}
</style>
