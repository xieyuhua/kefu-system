<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteDevice, listDevices, type Device, type DeviceKind } from '../../api/admin'
import dayjs from 'dayjs'

const loading = ref(false)
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
      </div>

      <div class="tip">
        展示当前账号已注册的 APP 与 PC 桌面端设备（docs/API.md §10.2）。消息会同时投递到在线的
        Web、PC 桌面端与 APP；注销后该设备不再接收推送。
      </div>

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
        <el-table-column prop="name" label="设备名称" min-width="200" show-overflow-tooltip />
        <el-table-column prop="platform" label="平台" min-width="120" show-overflow-tooltip />
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

.tip {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: #f5f5f5;
  border-radius: 4px;
  font-size: 12px;
  color: #7f7f7f;
  line-height: 1.6;
}
</style>
