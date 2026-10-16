<template>
  <div class="demo-panel">
    <el-button size="small" :icon="MagicStick" @click="open = true">模拟新咨询</el-button>

    <el-dialog v-model="open" title="模拟客户发起咨询" width="420px" append-to-body>
      <el-form :model="form" label-width="80px">
        <el-form-item label="渠道">
          <el-select v-model="form.channel_id" placeholder="选择渠道" style="width: 100%">
            <el-option
              v-for="c in channels"
              :key="c.id"
              :label="c.name"
              :value="c.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="客户ID">
          <el-input v-model="form.uid" placeholder="选填，如 user001" clearable />
          <div class="form-tip">
            填写后，<b>相同 ID 视为同一个客户</b>，可反复给他发消息（继续同一个会话）；
            留空则每次都会新建一个客户。
          </div>
        </el-form-item>
        <el-form-item label="客户昵称">
          <el-input v-model="form.nickname" placeholder="王小明" />
        </el-form-item>
        <el-form-item label="消息内容">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="3"
            placeholder="你好，我想问一下发货时间"
          />
        </el-form-item>
        <el-form-item label="消息类型">
          <el-radio-group v-model="form.type">
            <el-radio-button value="text">文本</el-radio-button>
            <el-radio-button value="image">图片</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button size="small" @click="open = false">取消</el-button>
        <el-button size="small" type="primary" :loading="submitting" @click="submit">
          发起咨询
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { MagicStick } from '@element-plus/icons-vue'
import { useChatStore } from '@/stores/chat'
import { incoming } from '@/api/demo'
import type { MessageType } from '@/types'

const chat = useChatStore()

const open = ref(false)
const submitting = ref(false)
const channels = computed(() => chat.channels)

const form = reactive<{
  channel_id: number
  uid: string
  nickname: string
  content: string
  type: MessageType
}>({
  channel_id: 0,
  uid: '',
  nickname: '',
  content: '你好，我想问一下发货时间',
  type: 'text',
})

const emit = defineEmits<{ created: [] }>()

async function submit(): Promise<void> {
  if (!form.channel_id) {
    ElMessage.warning('请先选择渠道')
    return
  }
  if (!form.nickname.trim()) {
    ElMessage.warning('请填写客户昵称')
    return
  }
  submitting.value = true
  const uid = form.uid.trim()
  try {
    await incoming({
      channel_id: form.channel_id,
      uid: uid || undefined,
      nickname: form.nickname.trim(),
      content: form.content.trim() || '你好',
      type: form.type,
    })
    ElMessage.success(
      uid
        ? `已向客户「${uid}」发起咨询（复用同一会话）`
        : '已发起模拟咨询，等待 WebSocket 推送',
    )
    emit('created')
    await chat.loadConversations()
    await chat.loadStats()
    open.value = false
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped lang="scss">
.demo-panel {
  display: inline-flex;
}
</style>
