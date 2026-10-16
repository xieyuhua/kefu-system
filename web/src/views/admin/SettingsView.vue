<script setup lang="ts">
/**
 * 系统设置：默认图片等租户级配置。
 */
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Upload } from '@element-plus/icons-vue'
import { listSettings, saveSetting, type SettingItem } from '../../api/admin'
import { uploadAttachment, validateFile } from '@/api/upload'
import ThumbImage from '@/components/ThumbImage.vue'

const loading = ref(false)
const saving = ref<Record<string, boolean>>({})
const uploading = ref(false)
const settings = ref<Record<string, string>>({})
const remarks = ref<Record<string, string>>({})

/** 变更过的项（用于显示"保存"按钮） */
const dirty = computed(() => Object.keys(settings.value).length > 0)

async function load(): Promise<void> {
  loading.value = true
  try {
    const list: SettingItem[] = await listSettings()
    const vals: Record<string, string> = {}
    const rms: Record<string, string> = {}
    list.forEach((it) => {
      vals[it.key] = it.value || ''
      rms[it.key] = it.remark || ''
    })
    settings.value = vals
    remarks.value = rms
  } catch {
    ElMessage.error('配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save(key: string): Promise<void> {
  saving.value[key] = true
  try {
    await saveSetting(key, settings.value[key] || '')
    ElMessage.success('已保存')
    await load()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    saving.value[key] = false
  }
}

async function saveAll(): Promise<void> {
  const keys = Object.keys(settings.value)
  for (const k of keys) {
    await save(k)
  }
}

/** 上传默认图片 */
const fileRef = ref<HTMLInputElement | null>(null)

function pickImage(): void {
  fileRef.value?.click()
}

async function onPickFile(): Promise<void> {
  const el = fileRef.value
  const file = el?.files?.[0]
  if (!el || !file) return

  // 只允许图片
  if (!/\.(jpg|jpeg|png|gif|webp|bmp|svg)$/i.test(file.name)) {
    ElMessage.error('请选择图片文件')
    el.value = ''
    return
  }
  const err = validateFile(file)
  if (err) {
    ElMessage.error(err)
    el.value = ''
    return
  }

  uploading.value = true
  try {
    const payload = await uploadAttachment(file)
    settings.value.default_image = payload.url
    await save('default_image')
    ElMessage.success('默认图片已更新')
  } catch {
    ElMessage.error('上传失败')
  } finally {
    uploading.value = false
    el.value = ''
  }
}

/** 恢复内置兜底图 */
async function resetDefault(): Promise<void> {
  settings.value.default_image = '/img/placeholder.svg'
  await save('default_image')
  ElMessage.success('已恢复内置兜底图')
}

const defaultImage = computed(() => settings.value.default_image || '')

onMounted(load)
</script>

<template>
  <div class="admin">
    <div v-loading="loading" class="admin-card">
      <div class="toolbar">
        <span class="toolbar__title">
          系统设置
          <span class="toolbar__sub">租户级配置，保存后立即对全站生效</span>
        </span>
        <div>
          <el-button size="small" :icon="Refresh" @click="load">刷新</el-button>
          <el-button
            v-if="dirty"
            size="small"
            type="primary"
            @click="saveAll"
          >
            保存全部
          </el-button>
        </div>
      </div>

      <!-- 默认图片 -->
      <div class="setting-item">
        <div class="setting-item__label">
          <div class="setting-item__name">默认图片</div>
          <div class="setting-item__remark">
            {{ remarks.default_image || '客户 / 商品 / 订单没有图片时展示该图片' }}
          </div>
        </div>

        <div class="setting-item__control">
          <div class="default-image">
            <ThumbImage :src="defaultImage" :size="88" :radius="6" alt="默认图片" />
            <div class="default-image__info">
              <el-input
                v-model="settings.default_image"
                size="small"
                placeholder="图片地址，如 /uploads/20260101/xxx.png"
                style="width: 320px"
              />
              <div class="default-image__actions">
                <el-button
                  size="small"
                  type="primary"
                  plain
                  :icon="Upload"
                  :loading="uploading"
                  @click="pickImage"
                >
                  上传图片
                </el-button>
                <el-button
                  size="small"
                  text
                  :loading="saving.default_image"
                  @click="save('default_image')"
                >
                  保存
                </el-button>
                <el-button size="small" text @click="resetDefault">恢复内置</el-button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <input
        ref="fileRef"
        type="file"
        hidden
        accept="image/*"
        @change="onPickFile"
      />

      <el-alert type="info" :closable="false" class="tip">
        说明：默认图片用于客户列表、订单管理、商品管理等页面的<b>图片兜底</b>；
        聊天中的图片消息单独使用内置占位图，不受此项影响。
      </el-alert>
    </div>
  </div>
</template>

<style scoped lang="scss">
@use './shared.scss';

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  &__title {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__sub {
    margin-left: 8px;
    font-size: 12px;
    font-weight: 400;
    color: #7f7f7f;
  }
}

.setting-item {
  display: flex;
  gap: 24px;
  padding: 14px 0;
  border-bottom: 1px solid #f0f0f0;

  &__label {
    width: 180px;
    flex-shrink: 0;
  }

  &__name {
    font-size: 14px;
    color: #191919;
    margin-bottom: 4px;
  }

  &__remark {
    font-size: 12px;
    color: #7f7f7f;
    line-height: 1.6;
  }

  &__control {
    flex: 1;
    min-width: 0;
  }
}

.default-image {
  display: flex;
  align-items: center;
  gap: 16px;

  &__info {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  &__actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.tip {
  margin-top: 16px;
}
</style>
