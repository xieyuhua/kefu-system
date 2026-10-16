<script setup lang="ts">
/**
 * 转接客服选择弹窗（搜索下拉）。
 *
 * 只能选择「配置了接待该渠道」的客服——渠道外的客服看不到这个会话，
 * 转过去等于把会话丢掉。因此这里直接按渠道过滤，不给输入任意 ID 的机会。
 */
import { computed, nextTick, ref, watch } from 'vue'
import type { Agent } from '@/api/agents'

const visible = defineModel<boolean>({ default: false })

const props = defineProps<{
  /** 当前会话所属渠道 ID */
  channelId: number
  /** 渠道名称（用于空状态提示） */
  channelName: string
  /** 候选客服（全员） */
  agents: Agent[]
  /** 当前登录用户 ID（不能转给自己） */
  currentUserId: number
  /** 正在转接（禁用重复提交） */
  loading?: boolean
}>()

const emit = defineEmits<{
  confirm: [agentId: number]
}>()

const keyword = ref('')
const selected = ref<number | null>(null)
const selectRef = ref<{ focus: () => void } | null>(null)

/**
 * 可转接客服：排除自己 + 排除未配置该渠道的客服。
 * 管理员可接待任意渠道，不受渠道绑定限制。
 */
const options = computed(() => {
  const list = props.agents.filter((a) => {
    if (a.id === props.currentUserId) return false
    if (a.role === 'admin') return true // 管理员不受渠道绑定限制
    return (a.channels || []).some((c) => c.id === props.channelId)
  })
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter(
    (a) =>
      a.name.toLowerCase().includes(kw) || a.account.toLowerCase().includes(kw),
  )
})

/** 该渠道下共有多少可转接客服（不含搜索过滤，用于空状态文案） */
const totalOptions = computed(() => {
  return props.agents.filter((a) => {
    if (a.id === props.currentUserId) return false
    if (a.role === 'admin') return true
    return (a.channels || []).some((c) => c.id === props.channelId)
  }).length
})

// 打开时重置状态并聚焦搜索框
watch(
  visible,
  async (open) => {
    if (!open) return
    keyword.value = ''
    selected.value = null
    await nextTick()
    selectRef.value?.focus()
  },
)

function onSelect(id: number): void {
  selected.value = id
}

function onClose(): void {
  visible.value = false
}

function submit(): void {
  if (!selected.value) return
  emit('confirm', selected.value)
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    title="转接会话"
    width="420px"
    append-to-body
    :close-on-click-modal="false"
    @update:model-value="(v: boolean) => (visible = v)"
  >
    <div class="tip">
      只能转接给配置了接待「<b>{{ channelName || '该渠道' }}</b>」的客服。
    </div>

    <el-select
      ref="selectRef"
      v-model="selected"
      filterable
      remote
      :remote-method="(v: string) => (keyword = v)"
      :loading="false"
      placeholder="搜索客服昵称或账号"
      size="large"
      class="picker"
      @change="onSelect"
    >
      <el-option
        v-for="a in options"
        :key="a.id"
        :value="a.id"
        :label="a.name + '（' + a.account + '）'"
      >
        <div class="opt">
          <img v-if="a.avatar" class="opt__avatar" :src="a.avatar" alt="" />
          <span v-else class="opt__avatar opt__avatar--text">
            {{ (a.name || '?').slice(0, 1) }}
          </span>
          <span class="opt__name">{{ a.name }}</span>
          <span class="opt__account">{{ a.account }}</span>
          <span class="opt__load" :class="{ 'opt__load--on': a.active_count > 0 }">
            接待中 {{ a.active_count ?? 0 }}
          </span>
        </div>
      </el-option>

      <template #empty>
        <div class="empty">
          <template v-if="keyword.trim()">
            没有匹配的客服
          </template>
          <template v-else-if="totalOptions === 0">
            「{{ channelName || '该渠道' }}」暂无可转接的客服，<br />请先在「管理后台 → 渠道管理 → 接待账号」中添加
          </template>
        </div>
      </template>
    </el-select>

    <template #footer>
      <el-button @click="onClose">取消</el-button>
      <el-button type="primary" :disabled="!selected" :loading="loading" @click="submit">
        确认转接
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.tip {
  margin-bottom: 12px;
  padding: 8px 10px;
  border-radius: 6px;
  background: #f4f6f8;
  font-size: 12px;
  line-height: 1.6;
  color: #5f6b7a;

  b {
    color: #07c160;
  }
}

.picker {
  width: 100%;
}

.opt {
  display: flex;
  align-items: center;
  gap: 8px;

  &__avatar {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;

    &--text {
      display: flex;
      align-items: center;
      justify-content: center;
      background: #07c160;
      color: #fff;
      font-size: 11px;
    }
  }

  &__name {
    font-size: 14px;
    color: #191919;
  }

  &__account {
    font-size: 12px;
    color: #9c9c9c;
  }

  &__load {
    margin-left: auto;
    font-size: 12px;
    color: #bfbfbf;

    &--on {
      color: #fa8c16;
    }
  }
}

.empty {
  padding: 14px 8px;
  font-size: 12px;
  line-height: 1.7;
  color: #999;
  text-align: center;
}
</style>