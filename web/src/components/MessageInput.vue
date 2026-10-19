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

      <!-- PC 端：订单 / 商品 / 快捷回复 / 搜索 平铺展示 -->
      <template v-if="!compact">
        <el-button text size="small" title="订单" @click="emit('pick-order')">🧾 订单</el-button>
        <el-button text size="small" title="商品" @click="emit('pick-product')">🛍 商品</el-button>

        <el-popover placement="top-start" trigger="click" width="260">
          <template #reference>
            <el-button text size="small" title="快捷回复">⚡ 快捷回复</el-button>
          </template>
          <div class="quick-list">
            <div v-for="q in QUICK_REPLIES" :key="q" class="quick-item" @click="useQuick(q)">
              {{ q }}
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
            <el-dropdown-item command="order">🧾 发送订单</el-dropdown-item>
            <el-dropdown-item command="product">🛍 发送商品</el-dropdown-item>
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

    <!-- 移动端快捷回复弹层 -->
    <el-dialog v-model="quickOpen" title="快捷回复" width="86%" append-to-body>
      <div class="quick-list quick-list--dialog">
        <button
          v-for="q in QUICK_REPLIES"
          :key="q"
          type="button"
          class="quick-item"
          @click="useQuickFromDialog(q)"
        >
          {{ q }}
        </button>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { MoreFilled, Paperclip, Search } from '@element-plus/icons-vue'
import { uploadAttachment, validateFile } from '@/api/upload'
import type { MessageType } from '@/types'

const props = defineProps<{
  disabled?: boolean
  conversationId?: number
  /** 搜索面板是否展开（按钮高亮用） */
  searchActive?: boolean
  /** 本会话是否有消息正在发送（用于禁用发送按钮、防止重复提交） */
  sending?: boolean
  /** 紧凑模式（移动端）：次要按钮收进「更多」下拉 */
  compact?: boolean
}>()

const emit = defineEmits<{
  send: [payload: { type: MessageType; content: string }]
  'pick-order': []
  'pick-product': []
  'toggle-search': []
}>()

/** 移动端「更多」菜单分发 */
function onMore(command: string): void {
  switch (command) {
    case 'order':
      emit('pick-order')
      break
    case 'product':
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

/** 移动端快捷回复（不用 popover，用轻量弹层） */
const quickOpen = ref(false)

function showQuick(): void {
  quickOpen.value = true
}

const searchActive = computed(() => props.searchActive === true)
/** 发送中（props 变化会触发重渲染，这里转成 ref 便于 submit 内读取最新值） */
const sending = computed(() => props.sending === true)

const EMOJIS = [
  '😊', '😄', '😁', '🙂', '😉', '😍', '🤔', '😅',
  '👍', '🙏', '👏', '💪', '🎉', '❤️', '🌟', '🔥',
  '😢', '😭', '😡', '🤝', '✅', '❌', '📦', '🚚',
]

const QUICK_REPLIES = [
  '您好，很高兴为您服务～',
  '请稍等，我帮您查询一下。',
  '已为您登记，稍后会有专人跟进。',
  '麻烦提供一下订单号，方便我核实。',
  '感谢您的耐心等待！',
  '不好意思给您带来不便，我们会尽快处理。',
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

function useQuick(text: string): void {
  draft.value = text
  focus()
}

/** 移动端弹层里选用快捷回复：填充草稿、关弹层并聚焦输入框 */
function useQuickFromDialog(text: string): void {
  draft.value = text
  quickOpen.value = false
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

.quick-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.quick-list--dialog {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.quick-item {
  padding: 10px 12px;
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
}
</style>
