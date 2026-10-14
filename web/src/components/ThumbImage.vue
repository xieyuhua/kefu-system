<script setup lang="ts">
/**
 * 通用缩略图组件（管理后台用）
 * - 统一尺寸/圆角/懒加载
 * - 加载失败自动替换为**兜底图**（/img/placeholder.svg），不显示浏览器默认破图
 * - 悬停显示放大镜，点击查看大图
 */
import { ref, computed, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    src?: string
    size?: number
    round?: boolean
    fallback?: string
    alt?: string
    preview?: boolean
    radius?: number
    /** 头像类型使用头像兜底图 */
    kind?: 'goods' | 'avatar'
  }>(),
  {
    src: '',
    size: 40,
    round: false,
    fallback: '',
    alt: '',
    preview: true,
    radius: 4,
    kind: 'goods',
  },
)

const emit = defineEmits<{ preview: [src: string] }>()

/** 兜底图地址（public/img 下，构建时原样拷贝到 server/web/img） */
const PLACEHOLDER = {
  goods: '/img/placeholder.svg',
  avatar: '/img/avatar-placeholder.svg',
}

const failed = ref(false)
const loaded = ref(false)
const viewerVisible = ref(false)

watch(
  () => props.src,
  () => {
    failed.value = false
    loaded.value = false
    viewerVisible.value = false
  },
)

const hasSrc = computed(() => !!props.src)
const useFallbackImg = computed(() => failed.value)
const displaySrc = computed(() => {
  if (useFallbackImg.value) return PLACEHOLDER[props.kind]
  return props.src
})
const boxStyle = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
  borderRadius: props.round ? '50%' : `${props.radius}px`,
}))

function onError(): void {
  // 兜底图本身再失败则不再处理，避免死循环
  if (failed.value) return
  failed.value = true
  loaded.value = true
}

function onLoad(): void {
  loaded.value = true
}

function openViewer(): void {
  // 只有真实图片加载成功才允许预览，兜底图不预览
  if (props.preview && hasSrc.value && !failed.value && loaded.value) viewerVisible.value = true
}
</script>

<template>
  <span class="thumb" :style="boxStyle" :title="alt">
    <!-- 无地址：直接显示兜底图 -->
    <img
      v-if="!hasSrc"
      class="thumb__img"
      :src="PLACEHOLDER[kind]"
      :alt="alt"
      loading="lazy"
    />
    <template v-else>
      <img
        class="thumb__img"
        :src="displaySrc"
        :alt="alt"
        loading="lazy"
        @load="onLoad"
        @error="onError"
      />
      <span v-if="preview && !failed && loaded" class="thumb__mask" @click="openViewer">
        <svg width="45%" height="45%" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
          <circle cx="11" cy="11" r="7" />
          <path d="m21 21-4.3-4.3M11 8v6M8 11h6" />
        </svg>
      </span>
    </template>
  </span>

  <el-image-viewer
    v-if="viewerVisible"
    :url-list="[src]"
    hide-on-click-modal
    teleported
    @close="viewerVisible = false"
  />
</template>

<style scoped lang="scss">
.thumb {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: #f5f5f5;
  flex-shrink: 0;
  vertical-align: middle;
}

.thumb__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.thumb__mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
  opacity: 0;
  transition: opacity 0.15s;
  cursor: zoom-in;

  &:hover {
    opacity: 1;
  }
}
</style>
