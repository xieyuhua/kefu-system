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

      <el-button text size="small" title="图片" :icon="Picture" @click="pickImage" />
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

      <span class="toolbar-tip">Ctrl + Enter 发送</span>
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
      <el-button type="primary" size="small" :disabled="disabled || !canSend" @click="submit">
        发送
      </el-button>
    </div>

    <input ref="fileRef" type="file" accept="image/*" hidden @change="onFile" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Picture } from '@element-plus/icons-vue'
import type { MessageType } from '@/types'

const props = defineProps<{ disabled?: boolean; conversationId?: number }>()

const emit = defineEmits<{
  send: [payload: { type: MessageType; content: string }]
  'pick-order': []
  'pick-product': []
}>()

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

function submit(): void {
  if (props.disabled || !canSend.value) return
  emit('send', { type: 'text', content: draft.value.trim() })
  draft.value = ''
  focus()
}

function pickImage(): void {
  fileRef.value?.click()
}

function onFile(): void {
  const file = fileRef.value?.files?.[0]
  if (!fileRef.value || !file) return
  const reader = new FileReader()
  reader.onload = () => {
    const result = typeof reader.result === 'string' ? reader.result : ''
    if (result) {
      emit('send', { type: 'image', content: result })
    }
  }
  reader.readAsDataURL(file)
  fileRef.value.value = ''
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

.quick-item {
  padding: 6px 8px;
  border-radius: var(--kf-radius-sm);
  font-size: 13px;
  cursor: pointer;

  &:hover {
    background: var(--kf-bg-hover);
  }
}
</style>
