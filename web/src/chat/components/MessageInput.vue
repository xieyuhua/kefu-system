<template>
  <div class="message-input">
    <div class="toolbar">
      <el-popover placement="top-start" trigger="click" width="280">
        <template #reference>
          <el-button text size="small" title="表情">😊</el-button>
        </template>
        <div class="emoji-grid">
          <span
            v-for="e in EMOJIS"
            :key="e"
            class="emoji"
            @click="insert(e)"
            >{{ e }}</span
          >
        </div>
      </el-popover>

      <el-button text size="small" title="发送图片 / 视频 / 文件" :icon="Paperclip" @click="pickMedia" />

      <!-- PC 端：订单 / 商品 / 快捷回复 / 搜索 平铺展示。
           渠道关闭了对应卡片能力时按钮置灰，并用 tooltip 说明原因。 -->
      <template v-if="!compact">
        <!-- 渠道关闭了对应卡片能力时，入口直接隐藏（不占用位置，也不给用户「点了没反应」的困惑） -->
        <span v-if="orderCardEnabled" class="toolbar-btn">
          <el-button text size="small" title="订单" @click="emit('pick-order')">🧾 订单</el-button>
        </span>
        <span v-if="productCardEnabled" class="toolbar-btn">
          <el-button text size="small" title="商品" @click="emit('pick-product')">🛍 商品</el-button>
        </span>

        <!-- 快捷回复：数据来自 /api/quick-replies（我的私有 + 租户共享），
             点条目插入输入框；「管理」打开自己的话术管理对话框。 -->
        <el-popover
          v-model:visible="quickPopoverOpen"
          placement="top-start"
          trigger="click"
          width="300"
          @show="ensureQuickReplies()"
        >
          <template #reference>
            <el-button text size="small" title="快捷回复">⚡ 快捷回复</el-button>
          </template>
          <div class="quick-panel">
            <div class="quick-panel__head">
              <span class="quick-panel__title">快捷回复</span>
              <el-button link type="primary" size="small" @click="openQuickManage">管理</el-button>
            </div>
            <div v-if="quickLoading" class="quick-empty">加载中…</div>
            <div v-else-if="!quickList.length" class="quick-empty">
              还没有快捷回复，点「管理」新增一条
            </div>
            <div v-else class="quick-list">
              <button
                v-for="q in quickList"
                :key="q.id"
                type="button"
                class="quick-item"
                @click="useQuick(q)"
              >
                <span class="quick-item__head">
                  <span class="quick-item__title kf-ellipsis">{{ q.title }}</span>
                  <el-tag v-if="q.shared" size="small" type="info" effect="plain">公共</el-tag>
                </span>
                <span class="quick-item__content">{{ q.content }}</span>
              </button>
            </div>
          </div>
        </el-popover>

        <!-- 搜索历史消息（微信式，放在快捷回复旁） -->
        <el-button
          text
          size="small"
          :class="{ 'is-on': searchActive }"
          title="搜索该会话的历史消息"
          :icon="Search"
          @click="emit('toggle-search')"
        >
          搜索
        </el-button>
      </template>

      <!-- 移动端：次要功能收进「更多」下拉，避免按钮挤不下没法点 -->
      <el-dropdown v-else trigger="click" @command="onMore">
        <el-button text size="small" title="更多" :icon="MoreFilled" />
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-if="orderCardEnabled" command="order">🧾 发送订单</el-dropdown-item>
            <el-dropdown-item v-if="productCardEnabled" command="product">🛍 发送商品</el-dropdown-item>
            <el-dropdown-item command="quick">⚡ 快捷回复</el-dropdown-item>
            <el-dropdown-item command="search">🔍 搜索聊天记录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <span v-if="!compact" class="toolbar-tip">
        <span v-if="uploading" class="toolbar-uploading">上传中…</span>
        <span v-else>Ctrl + Enter 发送</span>
      </span>
      <span v-else class="toolbar-tip">
        <span v-if="uploading" class="toolbar-uploading">上传中…</span>
      </span>
    </div>

    <textarea
      ref="areaRef"
      v-model="draft"
      class="input-area"
      :placeholder="disabled ? '请选择会话后再发送消息' : '输入消息，Ctrl + Enter 发送'"
      :disabled="disabled"
      @keydown.ctrl.enter.prevent="submit"
      @keydown.enter.exact.prevent="submit"
    />

    <div class="input-foot">
      <span class="foot-hint">{{ draft.length }} 字</span>
      <el-button
        type="primary"
        size="small"
        :loading="sending"
        :disabled="disabled || sending || !canSend"
        @click="submit"
      >
        发送
      </el-button>
    </div>

    <input ref="fileRef" type="file" hidden accept="image/*,video/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.zip,.rar,.7z,.json,.log" @change="onFile" />

    <!-- 移动端快捷回复弹层：窄屏下宽度自适应、内容可滚动 -->
    <el-dialog
      v-model="quickDialogOpen"
      title="快捷回复"
      width="92%"
      append-to-body
      class="quick-pick-dialog"
    >
      <div v-if="quickLoading" class="quick-empty">加载中…</div>
      <div v-else-if="!quickList.length" class="quick-empty">还没有快捷回复，点下方「管理」新增</div>
      <div v-else class="quick-list quick-list--dialog kf-scroll">
        <button
          v-for="q in quickList"
          :key="q.id"
          type="button"
          class="quick-item"
          @click="useQuick(q)"
        >
          <span class="quick-item__head">
            <span class="quick-item__title kf-ellipsis">{{ q.title }}</span>
            <el-tag v-if="q.shared" size="small" type="info" effect="plain">公共</el-tag>
          </span>
          <span class="quick-item__content">{{ q.content }}</span>
        </button>
      </div>
      <template #footer>
        <el-button size="small" @click="openQuickManage">管理</el-button>
        <el-button size="small" type="primary" @click="quickDialogOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 快捷回复管理（新增 / 编辑 / 删除自己的话术） -->
    <QuickReplyDialog v-model="quickManageOpen" @changed="onQuickChanged" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { MoreFilled, Paperclip, Search } from '@element-plus/icons-vue'
import { uploadAttachment, validateFile } from '@/shared/api/upload'
import { listQuickReplies, type QuickReply } from '@/shared/api/quickReplies'
import type { MessageType } from '@/shared/types'
import QuickReplyDialog from './QuickReplyDialog.vue'

const props = defineProps<{
  disabled?: boolean
  conversationId?: number
  /** 搜索面板是否展开（按钮高亮用） */
  searchActive?: boolean
  /** 本会话是否有消息正在发送（用于禁用发送按钮、防止重复提交） */
  sending?: boolean
  /** 紧凑模式（移动端）：次要按钮收进「更多」下拉 */
  compact?: boolean
  /**
   * 当前会话所属渠道是否支持订单 / 商品卡片（后台渠道配置）。
   * 不支持时入口置灰并提示原因；默认 true，保持改造前行为。
   */
  orderCardEnabled?: boolean
  productCardEnabled?: boolean
}>()

const emit = defineEmits<{
  send: [payload: { type: MessageType; content: string }]
  'pick-order': []
  'pick-product': []
  'toggle-search': []
}>()

/** 卡片能力（未传视为支持，兼容旧调用点） */
const orderCardEnabled = computed(() => props.orderCardEnabled !== false)
const productCardEnabled = computed(() => props.productCardEnabled !== false)
/** 入口不可用时的原因说明（tooltip / 标题） */
const orderCardHint = '该渠道未开启订单卡片'
const productCardHint = '该渠道未开启商品卡片'

/** 移动端「更多」菜单分发 */
function onMore(command: string): void {
  switch (command) {
    case 'order':
      if (!orderCardEnabled.value) {
        ElMessage.warning(orderCardHint)
        return
      }
      emit('pick-order')
      break
    case 'product':
      if (!productCardEnabled.value) {
        ElMessage.warning(productCardHint)
        return
      }
      emit('pick-product')
      break
    case 'quick':
      showQuick()
      break
    case 'search':
      emit('toggle-search')
      break
  }
}

/** 移动端快捷回复弹层 */
const quickDialogOpen = ref(false)
/** PC 端快捷回复 popover 的展开状态（点「管理」时先收起） */
const quickPopoverOpen = ref(false)

function showQuick(): void {
  void ensureQuickReplies()
  quickDialogOpen.value = true
}

// ---------------- 快捷回复数据（接口驱动，首次打开时拉取并缓存） ----------------
/** 待插入的话术列表（已过滤停用项） */
const quickList = ref<QuickReply[]>([])
const quickLoading = ref(false)
/** 是否已拉取过：进入工作台后只拉一次，管理对话框改动后由 onQuickChanged 置回 false 再刷新 */
const quickLoaded = ref(false)

/** 拉取快捷回复（已加载且非强制刷新时直接返回缓存） */
async function ensureQuickReplies(force = false): Promise<void> {
  if (quickLoading.value) return
  if (quickLoaded.value && !force) return
  quickLoading.value = true
  try {
    const rows = await listQuickReplies()
    quickList.value = rows.filter((q) => q.enabled)
    quickLoaded.value = true
  } catch {
    // 错误已由拦截器提示；这里保留空列表，下次打开弹层会再试一次
    quickList.value = []
  } finally {
    quickLoading.value = false
  }
}

/** 快捷回复管理对话框 */
const quickManageOpen = ref(false)

function openQuickManage(): void {
  quickPopoverOpen.value = false
  quickDialogOpen.value = false
  quickManageOpen.value = true
}

/** 管理对话框完成增删改：清缓存并重新拉一次，弹层里立刻能看到新话术 */
function onQuickChanged(): void {
  quickLoaded.value = false
  void ensureQuickReplies(true)
}

const searchActive = computed(() => props.searchActive === true)
/** 发送中（props 变化会触发重渲染，这里转成 ref 便于 submit 内读取最新值） */
const sending = computed(() => props.sending === true)

const EMOJIS = [
  '😊', '😄', '😁', '🙂', '😉', '😍', '🤔', '😅',
  '👍', '🙏', '👏', '💪', '🎉', '❤️', '🌟', '🔥',
  '😢', '😭', '😡', '🤝', '✅', '❌', '📦', '🚚',
]

const draft = ref('')
const areaRef = ref<HTMLTextAreaElement | null>(null)
const fileRef = ref<HTMLInputElement | null>(null)
/** 上传中 */
const uploading = ref(false)

const canSend = computed(() => draft.value.trim().length > 0)

watch(
  () => props.conversationId,
  () => {
    draft.value = ''
  },
)

function focus(): void {
  areaRef.value?.focus()
}

function insert(text: string): void {
  const el = areaRef.value
  if (!el) {
    draft.value += text
    return
  }
  const start = el.selectionStart ?? draft.value.length
  const end = el.selectionEnd ?? draft.value.length
  draft.value = draft.value.slice(0, start) + text + draft.value.slice(end)
  requestAnimationFrame(() => {
    el.focus()
    const pos = start + text.length
    el.setSelectionRange(pos, pos)
  })
}

/** 点选快捷回复：替换草稿内容（保持改造前行为），并收起弹层 */
function useQuick(q: QuickReply): void {
  draft.value = q.content
  quickPopoverOpen.value = false
  quickDialogOpen.value = false
  focus()
}

/** 提交文本：发送中直接忽略，防止连按回车 / 重复点击产生重复请求 */
function submit(): void {
  if (props.disabled || sending.value) return
  const text = draft.value.trim()
  if (!text) return
  // 先清空再 emit：即使父组件尚未回传 sending 状态，
  // 第二次按键也会因为草稿已空而被 canSend 拦下。
  draft.value = ''
  emit('send', { type: 'text', content: text })
  focus()
}

function pickMedia(): void {
  fileRef.value?.click()
}

/** 上传图片 / 视频 / 文件：先调上传接口拿到 URL，再作为消息发送 */
async function onFile(): Promise<void> {
  const file = fileRef.value?.files?.[0]
  if (!fileRef.value || !file) return

  const err = validateFile(file)
  if (err) {
    ElMessage.error(err)
    fileRef.value.value = ''
    return
  }

  uploading.value = true
  try {
    const payload = await uploadAttachment(file)
    emit('send', { type: payload.kind, content: JSON.stringify(payload) })
  } catch {
    ElMessage.error('上传失败，请重试')
  } finally {
    uploading.value = false
    fileRef.value.value = ''
  }
}

defineExpose({ focus, insert })
</script>

<style scoped lang="scss">
.message-input {
  display: flex;
  flex-direction: column;
  height: 172px;
  background: #fff;
  border-top: 1px solid var(--kf-border);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px 10px;
  border-bottom: 1px solid var(--kf-border-light);
}

.toolbar-tip {
  margin-left: auto;
  font-size: 12px;
  color: var(--kf-text-weak);
}

/* 卡片入口的外层包裹：按钮 disabled 后浏览器不再派发鼠标事件，
   tooltip 需要挂在这个仍可悬停的 span 上（同时保持工具栏横向对齐）。 */
.toolbar-btn {
  display: inline-flex;
  align-items: center;
}

.toolbar-uploading {
  color: var(--kf-primary);
}

.input-area {
  flex: 1;
  padding: 8px 12px;
  border: none;
  outline: none;
  resize: none;
  font-family: inherit;
  font-size: 14px;
  line-height: 1.6;
  color: var(--kf-text);
  background: transparent;

  &:disabled {
    background: var(--kf-bg);
    cursor: not-allowed;
  }
}

.input-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px 10px;
}

.foot-hint {
  font-size: 12px;
  color: var(--kf-text-weak);
}

.emoji-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  gap: 2px;
}

.emoji {
  padding: 4px 0;
  font-size: 18px;
  text-align: center;
  cursor: pointer;
  border-radius: var(--kf-radius-sm);

  &:hover {
    background: var(--kf-bg-hover);
  }
}

.quick-panel {
  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
    padding: 0 2px;
  }

  &__title {
    font-size: 13px;
    font-weight: 600;
    color: var(--kf-text);
  }
}

.quick-empty {
  padding: 14px 4px;
  text-align: center;
  font-size: 12px;
  color: var(--kf-text-weak);
}

.quick-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 300px;
  overflow-y: auto;
}

.quick-list--dialog {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 52vh;
}

.quick-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: 8px 12px;
  border: none;
  background: transparent;
  border-radius: var(--kf-radius-sm);
  font-size: 14px;
  text-align: left;
  color: var(--kf-text);
  cursor: pointer;

  &:hover {
    background: var(--kf-bg-hover);
  }

  &:active {
    background: var(--kf-bg-active);
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  &__title {
    font-size: 13px;
    font-weight: 600;
    color: var(--kf-text);
  }

  &__content {
    font-size: 13px;
    line-height: 1.5;
    color: var(--kf-text-sub);
    word-break: break-word;
    white-space: pre-wrap;
    max-height: 3em;
    overflow: hidden;
  }
}

/* 移动端弹层：窄屏宽度自适应，内容可滚动，不出现横向溢出 */
:global(.quick-pick-dialog) {
  max-width: 94%;
}

:global(.quick-pick-dialog .el-dialog__body) {
  padding: 10px 14px;
  overflow-x: hidden;
}
</style>
