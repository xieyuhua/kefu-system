<script setup lang="ts">
/**
 * 系统设置：默认图片等租户级配置。
 */
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Upload } from '@element-plus/icons-vue'
import {
  AI_API_KEY_MASK,
  AI_SETTING_DEFAULTS,
  AI_SETTING_KEYS,
  STORAGE_OBJECT_DEFAULTS,
  STORAGE_OBJECT_KEY,
  STORAGE_SECRET_MASK,
  listSettings,
  parseAISetting,
  parseStorageObject,
  saveSetting,
  serializeAISetting,
  serializeStorageObject,
  testAISetting,
  testStorageObject,
  type AISettingForm,
  type SettingItem,
  type StorageObjectConfig,
} from '@/shared/api/admin'
import { uploadAttachment, validateFile } from '@/shared/api/upload'
import { playMessageSound } from '@/shared/utils/sound'
import { useIsMobile } from '@/shared/composables/useIsMobile'
import { useAuthStore } from '@/shared/stores/auth'
import ThumbImage from '@/shared/components/ThumbImage.vue'

const { isMobile } = useIsMobile()
const auth = useAuthStore()

const loading = ref(false)
const saving = ref<Record<string, boolean>>({})
const uploading = ref(false)
/** 助手头像单独的上传态：与默认图片互不影响（两个按钮各自 loading） */
const uploadingAvatar = ref(false)
const settings = ref<Record<string, string>>({})
const remarks = ref<Record<string, string>>({})

/** 变更过的项（用于显示"保存"按钮） */
const dirty = computed(() => Object.keys(settings.value).length > 0)

/* ---------------- 对象存储（storage.object） ---------------- *
 * 该配置以整份 JSON 字符串存在 settings 表里，字段较多，
 * 因此单独维护一份表单状态，不塞进 settings（避免污染「保存全部」的逐项逻辑）。
 */
const storage = ref<StorageObjectConfig>({ ...STORAGE_OBJECT_DEFAULTS })
/** 后端已保存过密钥（返回 ******）：未改动时提交 ******，由后端保留原值 */
const secretSaved = ref(false)
/** 表单有未保存改动（用于刷新时不冲掉正在编辑的内容） */
const storageDirty = ref(false)
const testing = ref(false)
const savingStorage = ref(false)
/** 最近一次「测试连接」结果（常驻展示，便于截图/排查） */
const testResult = ref<{ ok: boolean; message: string } | null>(null)

/** 把配置写进表单（同时刷新「密钥已保存」标记） */
function applyStorage(cfg: StorageObjectConfig): void {
  const masked = cfg.secret_key === STORAGE_SECRET_MASK
  // 已脱敏时表单里保留 ****** 原样展示，未改动则原样回写
  storage.value = { ...cfg, provider: cfg.provider || 'ctyun' }
  secretSaved.value = masked
}

/** 密钥仍是「后端已保存」的状态：显示 ****** 或留空（留空提交 ******，由后端保留原值） */
const secretUntouched = computed(
  () =>
    secretSaved.value &&
    (storage.value.secret_key === '' || storage.value.secret_key === STORAGE_SECRET_MASK),
)

/** 编辑任一存储字段：标记为待保存 */
function markStorageDirty(): void {
  storageDirty.value = true
}

/** 提交给后端的配置：密钥未改动 / 留空时提交 ******，由后端保留原值 */
function storageForSubmit(): StorageObjectConfig {
  const cfg: StorageObjectConfig = { ...storage.value, provider: 'ctyun' }
  if (!cfg.secret_key && secretSaved.value) cfg.secret_key = STORAGE_SECRET_MASK
  return cfg
}

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
    // 读取失败 / 字段缺失时 parseStorageObject 会自动兜底成默认值
    if (!storageDirty.value) applyStorage(parseStorageObject(vals[STORAGE_OBJECT_KEY]))
    // AI 应答表单同理：有未保存改动时不冲掉正在编辑的内容
    if (!aiDirty.value) applyAI(parseAISetting(vals))
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
    // 系统名称改完要立刻反映到顶栏 / 页面标题，所以顺手刷新一次登录态
    if (key === 'system.name') {
      await auth.me()
      document.title = settings.value['system.name'] || document.title
    }
  } catch {
    ElMessage.error('保存失败')
  } finally {
    saving.value[key] = false
  }
}

async function saveAll(): Promise<void> {
  // 对象存储由下方 saveStorage 统一序列化提交，这里跳过它的原始 JSON，避免重复写入；
  // AI 应答卡片同理（有自己的「保存」按钮，且 api_key 需要按密钥口径回写）。
  const aiKeys = new Set<string>(Object.values(AI_SETTING_KEYS))
  const keys = Object.keys(settings.value).filter(
    (k) => k !== STORAGE_OBJECT_KEY && !aiKeys.has(k),
  )
  for (const k of keys) {
    await save(k)
  }
  // 对象存储改动一并落库（未改动则不打扰后端）
  if (storageDirty.value) {
    await saveStorage()
  }
  if (aiDirty.value) {
    await saveAI()
  }
}

/** 保存对象存储配置：整份配置序列化成 JSON 写入 storage.object */
async function saveStorage(): Promise<void> {
  savingStorage.value = true
  try {
    await saveSetting(STORAGE_OBJECT_KEY, serializeStorageObject(storageForSubmit()))
    storageDirty.value = false
    ElMessage.success('对象存储配置已保存')
    // 重新拉取：后端回显的 secret_key 是脱敏值，落回表单后即为 ******
    await load()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    savingStorage.value = false
  }
}

/** 测试连接：body 为配置对象本体，结果用后端返回的 message 提示 */
async function testStorage(): Promise<void> {
  if (!storage.value.endpoint.trim() || !storage.value.bucket.trim()) {
    ElMessage.warning('请先填写 Endpoint 和 Bucket')
    return
  }
  testing.value = true
  try {
    const res = await testStorageObject(storageForSubmit())
    const message = (res && res.message) || (res && res.ok ? '连接成功' : '连接失败')
    const ok = !!(res && res.ok)
    testResult.value = { ok, message }
    if (ok) {
      ElMessage.success(message)
    } else {
      ElMessage.error(message)
    }
  } catch (err) {
    // 后端未就绪 / 网络异常：http 拦截器已弹过 message，这里只补结果行与兜底提示，不让页面报错
    const msg = err instanceof Error && err.message ? err.message.trim() : ''
    const message = msg || '接口暂不可用，请确认后端服务已就绪'
    testResult.value = { ok: false, message }
    if (!msg) {
      ElMessage.error(`测试连接失败：${message}`)
    }
  } finally {
    testing.value = false
  }
}

/* ---------------- AI 应答（ai.* / rag.*） ---------------- *
 * 未命中关键词规则时的兜底链路：知识库检索 → 大模型 → 兜底话术。
 * 默认关闭：不填不启用时，机器人行为与改造前完全一致。
 * api_key 由后端脱敏成 ******，未改动时提交 ****** 即表示沿用已保存的 Key。
 */
const aiForm = ref<AISettingForm>({ ...AI_SETTING_DEFAULTS })
/** 后端已保存过 API Key（返回 ******） */
const aiKeySaved = ref(false)
/** AI 表单有未保存改动（用于刷新时不冲掉正在编辑的内容） */
const aiDirty = ref(false)
const aiTesting = ref(false)
const aiSaving = ref(false)
const aiTestResult = ref<{ ok: boolean; message: string } | null>(null)

/** 把解析后的配置写进 AI 表单（同时刷新「密钥已保存」标记） */
function applyAI(cfg: AISettingForm): void {
  aiForm.value = { ...cfg }
  aiKeySaved.value = cfg.apiKey === AI_API_KEY_MASK
}

/** 密钥仍是「后端已保存」状态：显示 ****** 或留空（留空提交 ******，由后端保留原值） */
const aiKeyUntouched = computed(
  () =>
    aiKeySaved.value &&
    (aiForm.value.apiKey === '' || aiForm.value.apiKey === AI_API_KEY_MASK),
)

/** 编辑任一 AI 字段：标记为待保存 */
function markAIDirty(): void {
  aiDirty.value = true
}

/** 提交给后端的 API Key：未改动 / 留空时提交 ******，由后端保留原值 */
function aiKeyForSubmit(): string {
  const key = (aiForm.value.apiKey || '').trim()
  if (!key && aiKeySaved.value) return AI_API_KEY_MASK
  return key
}

/** 保存 AI 配置：逐项 PUT /api/settings（一个 key 一个值） */
async function saveAI(): Promise<void> {
  if (aiForm.value.enabled && !(aiForm.value.baseUrl || '').trim()) {
    ElMessage.warning('启用 AI 应答前请先填写 Base URL')
    return
  }
  aiSaving.value = true
  try {
    const items = serializeAISetting({ ...aiForm.value, apiKey: aiKeyForSubmit() })
    for (const item of items) {
      await saveSetting(item.key, item.value)
    }
    aiDirty.value = false
    ElMessage.success('AI 应答配置已保存')
    // 重新拉取：后端回显的 api_key 是脱敏值，落回表单后即为 ******
    await load()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    aiSaving.value = false
  }
}

/** 测试连接：只验证「地址 + 鉴权 + 模型名」，不会保存任何配置 */
async function testAI(): Promise<void> {
  if (!(aiForm.value.baseUrl || '').trim() || !(aiForm.value.model || '').trim()) {
    ElMessage.warning('请先填写 Base URL 与模型名')
    return
  }
  aiTesting.value = true
  try {
    const res = await testAISetting({
      base_url: (aiForm.value.baseUrl || '').trim(),
      api_key: aiKeyForSubmit(),
      model: (aiForm.value.model || '').trim(),
      timeout_seconds: Number(aiForm.value.timeoutSeconds) || 20,
    })
    const message = (res && res.message) || (res && res.ok ? '连接成功' : '连接失败')
    const ok = !!(res && res.ok)
    aiTestResult.value = { ok, message }
    if (ok) {
      ElMessage.success(message)
    } else {
      ElMessage.error(message)
    }
  } catch (err) {
    const msg = err instanceof Error && err.message ? err.message.trim() : ''
    const message = msg || '接口暂不可用，请确认后端服务已就绪'
    aiTestResult.value = { ok: false, message }
    if (!msg) {
      ElMessage.error(`测试连接失败：${message}`)
    }
  } finally {
    aiTesting.value = false
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

/* ---------------- 助手头像（assistant.avatar） ---------------- *
 * 智能助手（机器人）回复时展示的头像：用户端 /kefu/ 与客服工作台 /chat/ 共用同一个值。
 * 留空（或点「恢复内置」）时两端各自回退到内置头像。
 */
const avatarFileRef = ref<HTMLInputElement | null>(null)
const assistantAvatar = computed(() => settings.value['assistant.avatar'] || '')

function pickAssistantAvatar(): void {
  avatarFileRef.value?.click()
}

async function onPickAssistantAvatar(): Promise<void> {
  const el = avatarFileRef.value
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

  uploadingAvatar.value = true
  try {
    const payload = await uploadAttachment(file)
    // 上传成功先把返回地址回写输入框，再落库（用户可再手改 URL 后单独点保存）
    settings.value['assistant.avatar'] = payload.url
    await save('assistant.avatar')
    ElMessage.success('助手头像已更新')
  } catch {
    ElMessage.error('上传失败')
  } finally {
    uploadingAvatar.value = false
    el.value = ''
  }
}

/** 清空设置：两端的助手头像回到内置样式 */
async function resetAssistantAvatar(): Promise<void> {
  settings.value['assistant.avatar'] = ''
  await save('assistant.avatar')
  ElMessage.success('已恢复内置头像')
}

/** 提示音模式：on=内置 / off=静音 / custom=自定义 URL */
const soundMode = computed({
  get: () => {
    const v = (settings.value.message_sound || '').trim().toLowerCase()
    if (v === 'off') return 'off'
    if (v === 'on' || !v) return 'on'
    return 'custom'
  },
  set: (m: string) => {
    if (m === 'off') {
      settings.value.message_sound = 'off'
      void save('message_sound')
    } else if (m === 'on') {
      settings.value.message_sound = 'on'
      void save('message_sound')
    } else {
      // 切到自定义：清空等待输入，由「保存」按钮提交
      settings.value.message_sound = ''
    }
  },
})

/** 试听当前配置的提示音 */
function previewSound(): void {
  playMessageSound(settings.value.message_sound)
}

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

      <!-- 系统名称 -->
      <div class="setting-item">
        <div class="setting-item__label">
          <div class="setting-item__name">系统名称</div>
          <div class="setting-item__remark">
            {{ remarks['system.name'] || '展示在后台顶栏与浏览器标题；留空则使用租户名称' }}
          </div>
        </div>
        <div class="setting-item__control">
          <el-input
            v-model="settings['system.name']"
            size="small"
            class="setting-input"
            placeholder="例如：云跳跳客服 / 某某科技客服"
            maxlength="32"
            show-word-limit
          />
          <div class="setting-item__actions">
            <el-button size="small" type="primary" plain :loading="saving['system.name']" @click="save('system.name')">
              保存
            </el-button>
          </div>
        </div>
      </div>

      <!-- 机器人兜底话术 -->
      <div class="setting-item">
        <div class="setting-item__label">
          <div class="setting-item__name">机器人兜底话术</div>
          <div class="setting-item__remark">
            {{ remarks['robot.fallback_reply'] || '渠道开启机器人但未命中任何规则时的回复；留空则机器人不回复' }}
          </div>
        </div>
        <div class="setting-item__control">
          <el-input
            v-model="settings['robot.fallback_reply']"
            type="textarea"
            :rows="3"
            size="small"
            class="setting-input"
            maxlength="300"
            show-word-limit
            placeholder="例如：您的问题我已记录，稍后将由人工客服为您详细解答～"
          />
          <div class="setting-item__actions">
            <el-button
              size="small"
              type="primary"
              plain
              :loading="saving['robot.fallback_reply']"
              @click="save('robot.fallback_reply')"
            >
              保存
            </el-button>
          </div>
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
                class="setting-input"
                placeholder="图片地址，如 /uploads/20260101/xxx.png"
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

      <!-- 助手头像：智能助手（机器人）回复时展示，用户端与客服工作台共用 -->
      <div class="setting-item" data-test="assistant-avatar">
        <div class="setting-item__label">
          <div class="setting-item__name">助手头像</div>
          <div class="setting-item__remark">
            {{ remarks['assistant.avatar'] || '智能助手（机器人）回复时展示的头像；留空使用内置头像' }}
          </div>
        </div>

        <div class="setting-item__control">
          <div class="default-image">
            <ThumbImage
              :src="assistantAvatar"
              :size="88"
              :radius="6"
              kind="avatar"
              alt="助手头像"
            />
            <div class="default-image__info">
              <el-input
                v-model="settings['assistant.avatar']"
                size="small"
                class="setting-input"
                placeholder="图片地址，如 /uploads/20260101/xxx.png"
                data-test="assistant-avatar-url"
              />
              <div class="default-image__actions">
                <el-button
                  size="small"
                  type="primary"
                  plain
                  :icon="Upload"
                  :loading="uploadingAvatar"
                  data-test="assistant-avatar-upload"
                  @click="pickAssistantAvatar"
                >
                  上传图片
                </el-button>
                <el-button
                  size="small"
                  text
                  :loading="saving['assistant.avatar']"
                  data-test="assistant-avatar-save"
                  @click="save('assistant.avatar')"
                >
                  保存
                </el-button>
                <el-button
                  size="small"
                  text
                  data-test="assistant-avatar-reset"
                  @click="resetAssistantAvatar"
                >
                  恢复内置
                </el-button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <input
        ref="avatarFileRef"
        type="file"
        hidden
        accept="image/*"
        data-test="assistant-avatar-file"
        @change="onPickAssistantAvatar"
      />

      <!-- 消息提示音 -->
      <div class="setting-item">
        <div class="setting-item__label">
          <div class="setting-item__name">消息提示音</div>
          <div class="setting-item__remark">
            客服工作台收到新消息时播放提示音（客户发来的消息，自己发出的不播）
          </div>
        </div>

        <div class="setting-item__control">
          <div class="sound-row">
            <el-radio-group v-model="soundMode" size="small">
              <el-radio value="on">内置提示音</el-radio>
              <el-radio value="off">静音</el-radio>
              <el-radio value="custom">自定义</el-radio>
            </el-radio-group>
            <el-button size="small" text type="primary" @click="previewSound">试听</el-button>
          </div>

          <div v-if="soundMode === 'custom'" class="sound-custom">
            <el-input
              v-model="settings.message_sound"
              size="small"
              class="setting-input setting-input--wide"
              placeholder="音频 URL，如 /uploads/20260101/xxx.mp3 或 https://..."
            />
            <el-button
              size="small"
              type="primary"
              plain
              :loading="saving.message_sound"
              @click="save('message_sound')"
            >
              保存
            </el-button>
          </div>
        </div>
      </div>

      <el-alert type="info" :closable="false" class="tip">
        说明：默认图片用于客户列表、订单管理、商品管理等页面的<b>图片兜底</b>；
        聊天中的图片消息单独使用内置占位图，不受此项影响。提示音「内置」无需上传音频文件，由浏览器直接合成。
      </el-alert>
    </div>

    <!-- ---------------- 对象存储（S3 兼容） ---------------- -->
    <div v-loading="loading" class="admin-card storage-card" data-test="storage-card">
      <div class="toolbar">
        <span class="toolbar__title">
          对象存储
          <span class="toolbar__sub">S3 兼容对象存储，图片 / 文件直传</span>
        </span>
        <div>
          <el-button
            size="small"
            :loading="testing"
            data-test="storage-test"
            @click="testStorage"
          >
            测试连接
          </el-button>
          <el-button
            size="small"
            type="primary"
            :loading="savingStorage"
            data-test="storage-save"
            @click="saveStorage"
          >
            保存
          </el-button>
        </div>
      </div>

      <div :class="['storage-form', isMobile ? 'storage-form--mobile' : '']">
        <!-- 启用开关 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">启用对象存储</div>
            <div class="setting-item__remark">
              关闭时继续使用服务器本地存储，下方参数仅可查看
            </div>
          </div>
          <div class="setting-item__control">
            <div class="storage-switch">
              <el-switch
                v-model="storage.enabled"
                :size="isMobile ? 'default' : 'small'"
                data-test="storage-enabled"
                @change="markStorageDirty"
              />
              <span class="storage-switch__state">
                {{ storage.enabled ? '已启用' : '未启用（参数置灰）' }}
              </span>
            </div>
          </div>
        </div>

        <!-- 服务商（只读） -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">服务商</div>
            <div class="setting-item__remark">S3 协议兼容，可对接 MinIO、各云厂商对象存储等</div>
          </div>
          <div class="setting-item__control">
            <div class="storage-readonly" data-test="storage-provider">
              S3 兼容协议
            </div>
          </div>
        </div>

        <!-- Endpoint -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">Endpoint</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.endpoint"
              size="small"
              class="setting-input setting-input--wide"
              placeholder="https://s3.example.com"
              :disabled="!storage.enabled"
              data-test="storage-endpoint"
              @input="markStorageDirty"
            />
            <div class="storage-hint">S3 端点，不要带 bucket</div>
          </div>
        </div>

        <!-- 区域 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">区域 Region</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.region"
              size="small"
              class="setting-input"
              placeholder="留空使用默认"
              :disabled="!storage.enabled"
              data-test="storage-region"
              @input="markStorageDirty"
            />
            <div class="storage-hint">可选，留空使用默认</div>
          </div>
        </div>

        <!-- Bucket -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">Bucket</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.bucket"
              size="small"
              class="setting-input"
              placeholder="kefu"
              :disabled="!storage.enabled"
              data-test="storage-bucket"
              @input="markStorageDirty"
            />
            <div class="storage-hint">存储桶名称，如 kefu</div>
          </div>
        </div>

        <!-- Access Key -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">Access Key</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.access_key"
              size="small"
              class="setting-input setting-input--wide"
              placeholder="Access Key"
              :disabled="!storage.enabled"
              data-test="storage-ak"
              @input="markStorageDirty"
            />
          </div>
        </div>

        <!-- Secret Key -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">Secret Key</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.secret_key"
              type="password"
              show-password
              size="small"
              class="setting-input setting-input--wide"
              placeholder="Secret Key"
              :disabled="!storage.enabled"
              data-test="storage-sk"
              @input="markStorageDirty"
            />
            <div v-if="secretUntouched" class="storage-hint" data-test="storage-sk-hint">
              已保存，留空表示不修改
            </div>
            <div v-else-if="secretSaved" class="storage-hint">
              将保存新的密钥，留空则保留原密钥
            </div>
            <div v-else class="storage-hint">与 Access Key 配对使用</div>
          </div>
        </div>

        <!-- 访问域名 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">访问域名</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.public_base"
              size="small"
              class="setting-input setting-input--wide"
              placeholder="https://cdn.example.com"
              :disabled="!storage.enabled"
              data-test="storage-public-base"
              @input="markStorageDirty"
            />
            <div class="storage-hint">文件对外访问的域名（CDN 或 bucket 域名）</div>
          </div>
        </div>

        <!-- 路径前缀 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">路径前缀</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="storage.path_prefix"
              size="small"
              class="setting-input"
              placeholder="uploads/"
              :disabled="!storage.enabled"
              data-test="storage-prefix"
              @input="markStorageDirty"
            />
            <div class="storage-hint">对象 key 前缀，默认 uploads/，保存时自动补尾部 /</div>
          </div>
        </div>
      </div>

      <div
        v-if="testResult"
        class="storage-test-result"
        :class="testResult.ok ? 'is-ok' : 'is-fail'"
        data-test="storage-test-result"
      >
        上次测试：{{ testResult.message }}
      </div>

      <el-alert type="info" :closable="false" class="tip">
        说明：开启后新上传的图片 / 文件写入对象存储，历史文件仍按原地址访问；
        测试连接只校验 Endpoint / Bucket / 密钥是否可用，不会写入任何文件。
        密钥已保存时页面只显示 <b>******</b>，留空提交表示保留原密钥。
      </el-alert>
    </div>

    <!-- ---------------- AI 应答（可选，默认关闭） ---------------- -->
    <div v-loading="loading" class="admin-card ai-card" data-test="ai-card">
      <div class="toolbar">
        <span class="toolbar__title">
          AI 应答（可选）
          <span class="toolbar__sub">知识库检索 + 大模型生成，默认关闭</span>
        </span>
        <div>
          <el-button
            size="small"
            :loading="aiTesting"
            data-test="ai-test"
            @click="testAI"
          >
            测试连接
          </el-button>
          <el-button
            size="small"
            type="primary"
            :loading="aiSaving"
            data-test="ai-save"
            @click="saveAI"
          >
            保存
          </el-button>
        </div>
      </div>

      <div :class="['ai-form', isMobile ? 'ai-form--mobile' : '']">
        <!-- 启用开关 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">启用 AI 应答</div>
            <div class="setting-item__remark">
              未命中自动回复规则时，先查知识库再让大模型生成回复；失败自动降级为兜底话术
            </div>
          </div>
          <div class="setting-item__control">
            <div class="storage-switch">
              <el-switch
                v-model="aiForm.enabled"
                :size="isMobile ? 'default' : 'small'"
                data-test="ai-enabled"
                @change="markAIDirty"
              />
              <span class="storage-switch__state">
                {{ aiForm.enabled ? '已启用' : '未启用（只走规则 + 兜底话术）' }}
              </span>
            </div>
          </div>
        </div>

        <!-- Base URL -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">Base URL</div>
            <div class="setting-item__remark">OpenAI 兼容接口根地址</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="aiForm.baseUrl"
              size="small"
              class="setting-input setting-input--wide"
              placeholder="https://api.deepseek.com/v1"
              data-test="ai-base-url"
              @input="markAIDirty"
            />
            <div class="storage-hint">不要带 /chat/completions，后端会自行拼接</div>
          </div>
        </div>

        <!-- API Key -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">API Key</div>
            <div class="setting-item__remark">大模型接口密钥</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="aiForm.apiKey"
              type="password"
              show-password
              size="small"
              class="setting-input setting-input--wide"
              placeholder="sk-..."
              data-test="ai-api-key"
              @input="markAIDirty"
            />
            <div v-if="aiKeyUntouched" class="storage-hint" data-test="ai-api-key-hint">
              已保存，留空表示不修改
            </div>
            <div v-else-if="aiKeySaved" class="storage-hint">将保存新的 Key，留空则保留原 Key</div>
            <div v-else class="storage-hint">只保存在服务端，页面只显示 ******</div>
          </div>
        </div>

        <!-- 模型名 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">模型名</div>
            <div class="setting-item__remark">以服务商文档为准</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="aiForm.model"
              size="small"
              class="setting-input"
              placeholder="deepseek-chat"
              data-test="ai-model"
              @input="markAIDirty"
            />
            <div class="storage-hint">例如 deepseek-chat / gpt-4o-mini / qwen-plus</div>
          </div>
        </div>

        <!-- 系统提示词 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">系统提示词</div>
            <div class="setting-item__remark">约束模型只依据知识库回答，不确定就转人工</div>
          </div>
          <div class="setting-item__control">
            <el-input
              v-model="aiForm.systemPrompt"
              type="textarea"
              :rows="4"
              size="small"
              class="setting-input setting-input--wide"
              maxlength="1000"
              show-word-limit
              data-test="ai-system-prompt"
              @input="markAIDirty"
            />
          </div>
        </div>

        <!-- 超时 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">超时（秒）</div>
            <div class="setting-item__remark">超时即降级为兜底话术，客户不会等不到回复</div>
          </div>
          <div class="setting-item__control">
            <el-input-number
              v-model="aiForm.timeoutSeconds"
              :min="1"
              :max="120"
              :step="1"
              size="small"
              data-test="ai-timeout"
              @change="markAIDirty"
            />
          </div>
        </div>

        <!-- 检索条数 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">检索条数 rag.top_k</div>
            <div class="setting-item__remark">拼进提示词的知识片段数量</div>
          </div>
          <div class="setting-item__control">
            <el-input-number
              v-model="aiForm.topK"
              :min="1"
              :max="10"
              :step="1"
              size="small"
              data-test="ai-top-k"
              @change="markAIDirty"
            />
          </div>
        </div>

        <!-- 最低相关度 -->
        <div class="setting-item">
          <div class="setting-item__label">
            <div class="setting-item__name">最低相关度 rag.min_score</div>
            <div class="setting-item__remark">0~1；低于该值视为知识库无命中，直接回兜底话术</div>
          </div>
          <div class="setting-item__control">
            <el-input-number
              v-model="aiForm.minScore"
              :min="0"
              :max="1"
              :step="0.05"
              :precision="2"
              size="small"
              data-test="ai-min-score"
              @change="markAIDirty"
            />
          </div>
        </div>
      </div>

      <div
        v-if="aiTestResult"
        class="storage-test-result"
        :class="aiTestResult.ok ? 'is-ok' : 'is-fail'"
        data-test="ai-test-result"
      >
        上次测试：{{ aiTestResult.message }}
      </div>

      <el-alert type="info" :closable="false" class="tip">
        说明：AI 应答只在「未命中自动回复规则 + 渠道已开启机器人」时才触发，
        链路为 <b>知识库检索 → 大模型 → 兜底话术</b>；检索无命中不会去猜，直接回兜底话术。
        知识条目在「知识库」页维护。Key 已保存时页面只显示 <b>******</b>，留空提交表示保留原 Key。
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

.sound-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sound-custom {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}

/* 输入框宽度：与原内联样式一致（PC 端几何不变），窄屏再改为 100% */
.setting-input {
  width: 320px;
}

.setting-input--wide {
  width: 360px;
}

/* ---------------- 对象存储卡片 ---------------- */
.storage-card {
  /* 右上角操作按钮：窄屏自动换行，避免撑出横向滚动 */
  .toolbar > div {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}

.storage-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 24px;

  &__state {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.storage-readonly {
  font-size: 14px;
  color: #191919;
  line-height: 24px;
}

/* 关闭启用时输入框置灰，但值仍要看得清（Element 默认的 disabled 文字过淡） */
.storage-form :deep(.el-input.is-disabled .el-input__inner) {
  color: #7d828c;
  -webkit-text-fill-color: #7d828c;
  opacity: 1;
}

.storage-hint {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.6;
  color: #909399;
  word-break: break-word;
}

.storage-test-result {
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.6;
  word-break: break-word;

  &.is-ok {
    color: #067a46;
    background: #e8f8ef;
  }

  &.is-fail {
    color: #c45656;
    background: #fef0f0;
  }
}

/* JS 判定为移动端时同样上下堆叠（与下方媒体查询保持一致，双保险） */
.storage-form--mobile {
  .setting-item {
    flex-direction: column;
    gap: 8px;
    padding: 12px 0;

    &__label {
      width: auto;
    }

    &__control {
      width: 100%;
    }
  }

  .setting-input,
  .setting-input--wide {
    width: 100%;
  }
}

/* ---------------- AI 应答卡片 ---------------- */
.ai-card {
  /* 右上角操作按钮：窄屏自动换行，避免撑出横向滚动 */
  .toolbar > div {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}

/* JS 判定为移动端时上下堆叠（与 storage-form--mobile 同一口径） */
.ai-form--mobile {
  .setting-item {
    flex-direction: column;
    gap: 8px;
    padding: 12px 0;

    &__label {
      width: auto;
    }

    &__control {
      width: 100%;
    }
  }

  .setting-input,
  .setting-input--wide {
    width: 100%;
  }
}

/* ---------------- 移动端（≤768px） ---------------- */
@media (max-width: 768px) {
  .toolbar {
    align-items: flex-start;
    margin-bottom: 10px;

    &__title {
      display: block;
      white-space: normal;
    }

    &__sub {
      display: block;
      margin-left: 0;
      margin-top: 2px;
    }
  }

  /* 标签与控件上下堆叠，控件占满整行 */
  .setting-item {
    flex-direction: column;
    gap: 8px;
    padding: 12px 0;

    &__label {
      width: auto;
    }

    &__control {
      width: 100%;
    }
  }

  .default-image {
    align-items: flex-start;
    gap: 12px;

    &__info {
      flex: 1;
      min-width: 0;
      width: 100%;
    }

    &__actions {
      flex-wrap: wrap;
      row-gap: 8px;
    }
  }

  .setting-input,
  .setting-input--wide {
    width: 100%;
  }

  .sound-row {
    flex-wrap: wrap;
    row-gap: 8px;
  }

  .sound-custom {
    flex-wrap: wrap;
    align-items: stretch;
    row-gap: 8px;
  }

  /* 对象存储：标题与操作按钮上下排列，避免按钮把标题挤成竖排 */
  .storage-card .toolbar {
    flex-direction: column;
    row-gap: 8px;

    > div {
      justify-content: flex-start;
    }
  }

  /* AI 应答卡片同理 */
  .ai-card .toolbar {
    flex-direction: column;
    row-gap: 8px;

    > div {
      justify-content: flex-start;
    }
  }

  .ai-form .setting-item {
    flex-direction: column;
    gap: 8px;
    padding: 12px 0;

    .setting-item__label {
      width: auto;
    }

    .setting-item__control {
      width: 100%;
    }
  }

  .storage-readonly {
    line-height: 1.6;
  }
}
</style>
