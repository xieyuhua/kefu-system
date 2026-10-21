<template>
  <!--
    安全富文本渲染：
    - 不含任何 Markdown 标记 → 走原来的纯文本路径（<span> + pre-wrap，字体/行高完全继承气泡）
    - 含标记 → v-html 渲染 renderMarkdownSafe() 的结果（输入已先整体 HTML 转义，不含用户标签）
  -->
  <div class="richtext" :class="{ 'richtext--rich': result.rich }">
    <span v-if="!result.rich" class="richtext__plain">{{ content }}</span>
    <div v-else class="richtext__rich" v-html="result.html"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { renderMarkdownSafe } from '@/shared/utils/markdown'

const props = defineProps<{
  /** 消息原文（未转义） */
  content: string
}>()

/** 渲染结果：rich=false 时 html 为空串，模板走纯文本分支 */
const result = computed(() => renderMarkdownSafe(props.content || ''))
</script>

<style scoped lang="scss">
.richtext {
  min-width: 0;
  word-break: break-word;
  overflow-wrap: anywhere;
}

/* ---------------- 纯文本分支（与改造前的 .bubble-text 完全一致） ---------------- */
.richtext__plain {
  white-space: pre-wrap;
}

/* ---------------- 富文本分支 ---------------- */
.richtext__rich {
  line-height: 1.6;
  word-break: break-word;
  overflow-wrap: anywhere;
  /* 长串/长链接不撑出横向滚动条 */
  white-space: normal;
}

:deep(.md-p) {
  margin: 0 0 6px;

  &:last-child {
    margin-bottom: 0;
  }
}

/* 标题统一渲染为加粗段落（不生成 h1~h6，避免撑大字号） */
:deep(.md-h) {
  font-size: inherit;
  font-weight: 700;
  line-height: 1.6;
}

:deep(.md-ul),
:deep(.md-ol) {
  margin: 0 0 6px;
  padding-left: 16px;

  &:last-child {
    margin-bottom: 0;
  }
}

:deep(.md-ul) {
  list-style: disc;
}

:deep(.md-ol) {
  list-style: decimal;
}

:deep(li) {
  margin: 0 0 2px;
  word-break: break-word;

  &:last-child {
    margin-bottom: 0;
  }
}

:deep(strong) {
  font-weight: 700;
}

:deep(em) {
  font-style: italic;
}

:deep(.md-code) {
  padding: 1px 4px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.06);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Courier New', monospace;
  font-size: 0.92em;
  word-break: break-all;
}

:deep(.md-a) {
  color: var(--kf-primary, #07c160);
  text-decoration: underline;
  word-break: break-all;
}

:deep(.md-quote) {
  margin: 0 0 6px;
  padding: 0 0 0 8px;
  border-left: 3px solid rgba(0, 0, 0, 0.12);
  color: inherit;
  opacity: 0.9;

  &:last-child {
    margin-bottom: 0;
  }
}
</style>
