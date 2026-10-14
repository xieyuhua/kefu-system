<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Promotion } from '@element-plus/icons-vue'
import { deleteDevice, listDevices, testDevicePush, type Device, type DeviceKind } from '../../api/admin'
import dayjs from 'dayjs'

const loading = ref(false)
const testing = ref(false)
const devices = ref<Device[]>([])

const DEVICE_META: Record<string, { label: string; icon: string }> = {
  app: { label: 'APP', icon: '📱' },
  desktop: { label: 'PC 桌面端', icon: '💻' },
  web: { label: 'Web', icon: '🌐' },
}

const appCount = computed(() => devices.value.filter((d) => d.device === 'app').length)
const desktopCount = computed(() => devices.value.filter((d) => d.device === 'desktop').length)

function deviceLabel(device: DeviceKind | string): string {
  return DEVICE_META[device]?.label || device
}

function deviceIcon(device: DeviceKind | string): string {
  return DEVICE_META[device]?.icon || '🖥️'
}

function formatTime(value: string | undefined): string {
  if (!value) return '--'
  const d = dayjs(value.replace(/-/g, '/'))
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm') : value
}

/** 平台中文名 */
function platformLabel(platform: string): string {
  const map: Record<string, string> = {
    ios: 'iOS',
    android: 'Android',
    windows: 'Windows',
    mac: 'macOS',
  }
  return map[platform] || platform || '--'
}

/** Token 脱敏显示，避免在后台泄露完整凭据 */
function maskToken(token: string): string {
  if (!token) return '--'
  if (token.length <= 12) return '***'
  return `${token.slice(0, 6)}****${token.slice(-4)}`
}

/** 发送一条测试推送，用于验证厂商通道是否接通 */
async function sendTest(): Promise<void> {
  testing.value = true
  try {
    await testDevicePush({ title: '推送自检', body: '这是一条测试推送，收到说明厂商通道已接通' })
    const withToken = devices.value.filter((d) => d.push_token).length
    if (withToken === 0) {
      ElMessage.warning('当前账号没有已注册 Token 的设备，请先让 APP / 桌面端调用 POST /api/devices 注册')
    } else {
      ElMessage.success(`已触发推送，详情见服务端日志（目标设备 ${withToken} 台）`)
    }
  } catch {
    ElMessage.error('触发推送失败')
  } finally {
    testing.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    devices.value = await listDevices()
  } catch {
    devices.value = []
  } finally {
    loading.value = false
  }
}

async function remove(row: Device): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `注销「${row.name}」后该设备将不再收到新消息推送，确认注销？`,
      '注销设备',
      { type: 'warning', confirmButtonText: '注销', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  await deleteDevice(row.id)
  ElMessage.success('设备已注销')
  await load()
}

onMounted(load)
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          设备 / 推送管理
          <span class="toolbar__count">
            共 {{ devices.length }} 台设备（APP {{ appCount }} · PC {{ desktopCount }}）
          </span>
        </div>
        <el-button size="small" :icon="Promotion" :loading="testing" @click="sendTest">
          发送测试推送
        </el-button>
      </div>

      <el-alert type="info" :closable="false" class="push-alert">
        <div class="push-alert__title">推送接入说明</div>
        <ul class="push-alert__list">
          <li>APP / PC 桌面端启动后需调用 <code>POST /api/devices</code> 注册推送 Token，离线才能收到通知。</li>
          <li>在线时消息走 WebSocket 实时下发；离线时由服务端厂商推送（APNs / 极光 / FCM），<strong>两条通道互补，任一不通都不丢消息</strong>。</li>
          <li>离线提醒会落库，端侧重连后通过 <code>GET /api/offline/notifications?since_id=</code> 增量拉取。</li>
          <li>厂商通道的接入步骤见后端配置项 <code>config.yaml → push</code> 与文档 <code>docs/推送对接.md</code>。</li>
        </ul>
      </el-alert>

      <el-table
        v-loading="loading"
        class="admin-table"
        :data="devices"
        stripe
        border
        empty-text="暂无已注册设备"
      >
        <el-table-column label="设备类型" min-width="150">
          <template #default="{ row }">
            <span class="channel-tag">
              <span class="channel-tag__icon">{{ deviceIcon(row.device) }}</span>
              <span class="channel-tag__label">{{ deviceLabel(row.device) }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="设备名称" min-width="180" show-overflow-tooltip />
        <el-table-column label="平台" min-width="110">
          <template #default="{ row }">
            <span class="platform">{{ platformLabel(row.platform) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="推送 Token" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.push_token" class="token">{{ maskToken(row.push_token) }}</span>
            <span v-else class="token token--none">未注册（仅 WebSocket）</span>
          </template>
        </el-table-column>
        <el-table-column label="注册时间" min-width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="danger" @click="remove(row)">注销</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
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

.push-alert {
  margin-bottom: 12px;

  &__title {
    font-weight: 600;
    margin-bottom: 4px;
  }

  &__list {
    margin: 0;
    padding-left: 18px;
    line-height: 1.8;

    code {
      padding: 1px 4px;
      background: rgba(0, 0, 0, 0.06);
      border-radius: 3px;
      font-size: 12px;
    }
  }
}

.platform {
  color: #191919;
}

.token {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  color: #7f7f7f;

  &--none {
    font-family: var(--kf-font);
    font-style: italic;
    color: #b2b2b2;
  }
}
</style>
