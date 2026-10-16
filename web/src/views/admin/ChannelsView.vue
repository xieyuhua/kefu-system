<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Link, Picture, Plus, CopyDocument, Refresh } from '@element-plus/icons-vue'
import {
  createChannel,
  createChannelType,
  deleteChannel,
  getChannelQr,
  listChannelAgents,
  listChannelTypes,
  listChannels,
  resetChannelAppId,
  updateChannel,
  updateChannelAgents,
  channelQrPngUrl,
  type ChannelAgent,
  type ChannelQr,
  type ChannelTypeItem,
} from '../../api/admin'
import { getToken } from '../../api/token'
import ThumbImage from '@/components/ThumbImage.vue'
import type { Channel, ChannelPayload } from '../../types'

const loading = ref(false)
const submitting = ref(false)
const list = ref<Channel[]>([])
/** 渠道类型来自 GET /api/channel-types，完全自定义、不限制枚举 */
const typeOptions = ref<ChannelTypeItem[]>([])

const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const formRef = ref<FormInstance>()

interface ChannelForm {
  type: string
  name: string
  app_id: string
  enabled: boolean
  robot_enabled: boolean
  /** 用户端展示控制 */
  hide_header: boolean
  hide_debug: boolean
  hide_more_menu: boolean
}

function emptyForm(): ChannelForm {
  return {
    type: 'wechat',
    name: '',
    app_id: '',
    enabled: true,
    robot_enabled: false,
    hide_header: false,
    hide_debug: false,
    hide_more_menu: false,
  }
}

const form = reactive<ChannelForm>(emptyForm())

const rules: FormRules<ChannelForm> = {
  type: [{ required: true, message: '请选择或直接输入渠道类型', trigger: 'change' }],
  name: [{ required: true, message: '请输入渠道名称', trigger: 'blur' }],
}

/** 本地生成随机渠道标识（与后端格式一致：kefu_ + 8 位十六进制）。后端也会兜底生成。 */
function randomAppId(): string {
  const buf = new Uint8Array(4)
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(buf)
  } else {
    for (let i = 0; i < buf.length; i += 1) buf[i] = Math.floor(Math.random() * 256)
  }
  return `kefu_${Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')}`
}

const dialogTitle = computed(() => (editingId.value === null ? '新增渠道' : '编辑渠道'))

/** 按 code 取类型（icon + name + color），自定义类型也能正确展示 */
function typeMeta(type: string): { icon: string; name: string; color: string } {
  const hit = typeOptions.value.find((item) => item.code === type)
  if (hit) return { icon: hit.icon || '💬', name: hit.name, color: hit.color || '#7F7F7F' }
  return { icon: '💬', name: type || '未知类型', color: '#7F7F7F' }
}

/** 新建渠道时若输入了类型库里没有的类型，先创建类型再建渠道 */
async function ensureType(type: string): Promise<void> {
  const value = type.trim()
  if (!value) return
  if (typeOptions.value.some((item) => item.code === value)) return
  await createChannelType({
    code: value,
    name: value,
    color: '#07C160',
    icon: '💬',
    sort: 0,
  })
  await loadTypes()
}

/** 重置渠道标识：生成新的随机 app_id（旧链接与二维码会失效） */
async function resetAppId(row: Channel): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `重置后「${row.name}」的渠道标识会变成新的随机值，<strong>已分发的链接和二维码立即失效</strong>。确认重置？`,
      '重置渠道标识',
      { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消', dangerouslyUseHTMLString: true },
    )
  } catch {
    return
  }
  resettingId.value = row.id
  try {
    const res = await resetChannelAppId(row.id)
    ElMessage.success(`已重置为 ${res.app_id}`)
    await load()
  } catch {
    ElMessage.error('重置失败')
  } finally {
    resettingId.value = null
  }
}

async function loadTypes(): Promise<void> {
  try {
    typeOptions.value = await listChannelTypes()
  } catch {
    typeOptions.value = []
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    list.value = await listChannels()
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  // 新建时预填一个随机渠道标识（可点「随机」重新生成，也可手动修改）
  Object.assign(form, { ...emptyForm(), app_id: randomAppId() })
  dialogVisible.value = true
}

function regenerateAppId(): void {
  form.app_id = randomAppId()
}

function openEdit(row: Channel): void {
  editingId.value = row.id
  Object.assign(form, {
    type: row.type,
    name: row.name,
    app_id: row.app_id,
    enabled: row.enabled,
    robot_enabled: row.robot_enabled,
    hide_header: row.hide_header,
    hide_debug: row.hide_debug,
    hide_more_menu: row.hide_more_menu,
  })
  dialogVisible.value = true
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  const payload: ChannelPayload = {
    type: form.type.trim(),
    name: form.name.trim(),
    app_id: form.app_id.trim(),
    enabled: form.enabled,
    robot_enabled: form.robot_enabled,
    hide_header: form.hide_header,
    hide_debug: form.hide_debug,
    hide_more_menu: form.hide_more_menu,
  }
  submitting.value = true
  try {
    if (editingId.value === null) {
      await ensureType(payload.type)
      await createChannel(payload)
      ElMessage.success('渠道已创建')
    } else {
      await updateChannel(editingId.value, payload)
      ElMessage.success('渠道已更新')
    }
    dialogVisible.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(row: Channel, value: boolean): Promise<void> {
  try {
    await updateChannel(row.id, { enabled: value })
    row.enabled = value
    ElMessage.success(value ? '渠道已启用' : '渠道已停用')
  } catch {
    await load()
  }
}

async function toggleRobot(row: Channel, value: boolean): Promise<void> {
  try {
    await updateChannel(row.id, { robot_enabled: value })
    row.robot_enabled = value
    ElMessage.success(value ? '已开启机器人自动回复' : '已关闭机器人自动回复')
  } catch {
    await load()
  }
}

async function remove(row: Channel): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `删除渠道「${row.name}」后，该渠道下的接入配置将不可用，确认删除？`,
      '删除渠道',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  await deleteChannel(row.id)
  ElMessage.success('渠道已删除')
  await load()
}

/* ----------------------------- 二维码 & 接入地址 ----------------------------- */

/** 后端在 GET /api/channels 里新增了 access_path，types/index.ts 的 Channel 还没补 */
type ChannelRow = Channel & { access_path?: string; type: string }

/** 二维码尺寸可选值 */
const QR_SIZE_OPTIONS = [256, 320, 512]

const qrVisible = ref(false)
const qrLoading = ref(false)
const qr = ref<ChannelQr | null>(null)
const qrSize = ref<number>(320)
const qrChannelId = ref<number>(0)
const qrChannelName = ref('')
/** 正在重置 app_id 的渠道 id */
const resettingId = ref<number | null>(null)

/**
 * 完整接入地址：直接读后端返回的 access_url（基于后端 public_base 拼接）。
 * 不再使用 window.location.origin —— 客服可能在内网通过内网 IP/端口访问后台，
 * 但客户用的是另一个公网域名/端口，按 origin 拼出来会让客户扫到错误的地址。
 */
function accessUrl(row: ChannelRow): string {
  return row.access_url || (row.access_path || `/c?channel=${row.type}`)
}

/** 是否是相对路径（说明后端没配 public_base） */
function isRelativeUrl(url: string): boolean {
  return !!url && url.startsWith('/')
}

async function loadQr(): Promise<void> {
  if (!qrChannelId.value) return
  qrLoading.value = true
  try {
    qr.value = await getChannelQr(qrChannelId.value, { size: qrSize.value })
  } catch {
    qr.value = null
  } finally {
    qrLoading.value = false
  }
}

async function openQr(row: ChannelRow): Promise<void> {
  qrChannelId.value = row.id
  qrChannelName.value = row.name
  qrSize.value = 320
  qr.value = null
  qrVisible.value = true
  await loadQr()
}

function changeQrSize(size: number | string | boolean | undefined): void {
  qrSize.value = Number(size) || 320
  void loadQr()
}

/** 复制文本：优先 clipboard API，失败/非安全上下文时降级 execCommand */
async function copyText(text: string, okMessage: string): Promise<void> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      ElMessage.success(okMessage)
      return
    }
  } catch {
    /* 继续走降级方案 */
  }
  const input = document.createElement('textarea')
  input.value = text
  input.setAttribute('readonly', 'readonly')
  input.style.position = 'fixed'
  input.style.top = '-9999px'
  document.body.appendChild(input)
  input.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(input)
  if (ok) {
    ElMessage.success(okMessage)
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
}

function copyAccessUrl(row: ChannelRow): void {
  void copyText(accessUrl(row), '接入地址已复制')
}

/** 直接 a 标签直连 qr.png（后端返回带 Content-Disposition 的 PNG），不落 blob */
function downloadQr(): void {
  if (!qrChannelId.value) return
  const item = qr.value
  const file_name = item?.file_name || `channel-${qrChannelId.value}-qr-${qrSize.value}.png`
  const link = document.createElement('a')
  link.href = channelQrPngUrl(qrChannelId.value, qrSize.value, getToken())
  link.download = file_name
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

/* ------------------------------ 快速设置接待账号 ------------------------------ */

const agentVisible = ref(false)
const agentLoading = ref(false)
const agentSaving = ref(false)
const agentList = ref<ChannelAgent[]>([])
const agentIds = ref<number[]>([])
const agentChannelId = ref<number>(0)
const agentChannelName = ref('')

async function openAgents(row: ChannelRow): Promise<void> {
  agentChannelId.value = row.id
  agentChannelName.value = row.name
  agentList.value = []
  agentIds.value = []
  agentVisible.value = true

  agentLoading.value = true
  try {
    const items = await listChannelAgents(row.id)
    agentList.value = items
    agentIds.value = items.filter((item) => item.selected).map((item) => item.id)
  } catch {
    agentList.value = []
  } finally {
    agentLoading.value = false
  }
}

function selectAllAgents(): void {
  agentIds.value = agentList.value.map((item) => item.id)
}

function clearAgents(): void {
  agentIds.value = []
}

async function saveAgents(): Promise<void> {
  if (!agentChannelId.value) return
  agentSaving.value = true
  try {
    await updateChannelAgents(agentChannelId.value, [...agentIds.value])
    ElMessage.success('接待客服已更新')
    agentVisible.value = false
    await load()
  } catch {
    /* http 拦截器已统一提示错误 */
  } finally {
    agentSaving.value = false
  }
}

onMounted(() => {
  void load()
  void loadTypes()
})
</script>

<template>
  <div class="admin">
    <div class="admin-card">
      <div class="toolbar">
        <div class="toolbar__title">
          渠道管理
          <span class="toolbar__count">共 {{ list.length }} 个渠道</span>
        </div>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增渠道</el-button>
      </div>

      <div class="tip">
        渠道类型可自定义：下拉支持搜索，也可直接输入任意新类型（保存时会自动写入渠道类型库）。
        用户端链接 <code>/c?channel=&lt;类型 code 或渠道 ID&gt;</code> 即指向该渠道。
      </div>

      <el-table
        v-loading="loading"
        class="admin-table"
        :data="list"
        stripe
        border
        empty-text="暂无渠道"
      >
        <el-table-column label="渠道类型" min-width="180">
          <template #default="{ row }">
            <span class="channel-tag">
              <span class="channel-tag__icon" :style="{ color: typeMeta(row.type).color }">
                {{ typeMeta(row.type).icon }}
              </span>
              <span class="channel-tag__label">{{ typeMeta(row.type).name }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型标识" min-width="110" show-overflow-tooltip />
        <el-table-column prop="name" label="渠道名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="渠道标识 app_id" min-width="190">
          <template #default="{ row }">
            <div class="appid-cell">
              <code class="appid-cell__code">{{ row.app_id || '--' }}</code>
              <el-button
                link
                size="small"
                :icon="CopyDocument"
                title="复制 app_id"
                @click="copyText(row.app_id, '渠道标识已复制')"
              />
              <el-button
                link
                size="small"
                :icon="Refresh"
                :loading="resettingId === row.id"
                title="重置为新的随机标识（旧链接会失效）"
                @click="resetAppId(row)"
              />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="客服数" width="100" align="center">
          <template #default="{ row }">{{ row.agent_count ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="待接待" width="100" align="center">
          <template #default="{ row }">
            <span :class="{ 'pending-strong': (row.pending_count ?? 0) > 0 }">
              {{ row.pending_count ?? 0 }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              @update:model-value="(val: boolean) => toggleEnabled(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="机器人自动回复" width="130" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.robot_enabled"
              @update:model-value="(val: boolean) => toggleRobot(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="用户端展示" width="150" align="center">
          <template #default="{ row }">
            <div class="display-tags">
              <el-tag
                v-if="row.hide_header"
                size="small"
                type="warning"
                effect="plain"
                @click="openEdit(row)"
              >
                无header
              </el-tag>
              <el-tag
                v-if="row.hide_debug"
                size="small"
                type="info"
                effect="plain"
                @click="openEdit(row)"
              >
                无调试
              </el-tag>
              <span v-if="!row.hide_header && !row.hide_debug" class="display-none">
                默认
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="160" />
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Picture" @click="openQr(row)">二维码</el-button>
            <el-button link type="primary" :icon="Link" @click="copyAccessUrl(row)">地址</el-button>
            <el-button link type="primary" @click="openAgents(row)">
              接待账号({{ row.agent_count ?? 0 }})
            </el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="480px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item label="渠道类型" prop="type">
          <el-select
            v-model="form.type"
            filterable
            allow-create
            default-first-option
            placeholder="可搜索或直接输入任意类型"
            style="width: 100%"
          >
            <el-option
              v-for="opt in typeOptions"
              :key="opt.code"
              :label="opt.name"
              :value="opt.code"
            >
              <span class="channel-tag">
                <span class="channel-tag__icon" :style="{ color: opt.color || '#7F7F7F' }">
                  {{ opt.icon || '💬' }}
                </span>
                <span class="channel-tag__label">{{ opt.name }}</span>
                <span class="type-code">{{ opt.code }}</span>
              </span>
            </el-option>
          </el-select>
          <div class="form-tip">
            支持输入任意自定义类型（如 xiaohongshu / kuaishou），保存时自动写入渠道类型库。
          </div>
        </el-form-item>
        <el-form-item label="渠道名称" prop="name">
          <el-input v-model="form.name" placeholder="如：微信公众号客服" maxlength="40" show-word-limit />
        </el-form-item>
        <el-form-item label="渠道标识 app_id" prop="app_id">
          <div class="appid-field">
            <el-input v-model="form.app_id" placeholder="kefu_a1b2c3d4" class="appid-field__input" />
            <el-button :icon="Refresh" @click="regenerateAppId">随机</el-button>
          </div>
          <div class="form-tip">
            渠道的唯一标识，用于接入链接区分渠道来源与接待：<br />
            <code>/kefu/?channel={{ form.app_id || 'kefu_xxx' }}</code><br />
            同一渠道类型可配置多个，每个渠道的 app_id 不同，客户扫码/点击进来即可准确归因到该渠道。
          </div>
        </el-form-item>
        <el-form-item label="是否启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="机器人自动回复">
          <el-switch v-model="form.robot_enabled" />
        </el-form-item>

        <el-divider content-position="left">用户端展示（APP 内嵌场景）</el-divider>

        <el-form-item label="隐藏 header">
          <el-switch v-model="form.hide_header" />
          <div class="form-tip">
            隐藏用户端顶部标题栏（含返回按钮与客服名称）。<br />
            适用于把客服页面内嵌到 APP / 小程序 WebView，由宿主提供自己的导航栏。<br />
            也可用链接参数临时控制：<code>?chrome=0</code> 或 <code>?hide_header=1</code>
            （链接参数优先于此处配置）。
          </div>
        </el-form-item>
        <el-form-item label="隐藏接入调试">
          <el-switch v-model="form.hide_debug" />
          <div class="form-tip">
            隐藏 header 里的齿轮「接入调试」入口（排查参数用）。正式投放建议关闭。<br />
            链接参数：<code>?hide_debug=1</code>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 渠道二维码 / 接入地址 -->
    <el-dialog v-model="qrVisible" width="360px" :close-on-click-modal="false">
      <template #header>
        <span class="dialog-title">渠道「{{ qrChannelName }}」接入二维码</span>
      </template>

      <div v-loading="qrLoading" class="qr">
        <div class="qr__image">
          <img v-if="qr" :src="qr.base64" :width="220" :height="220" alt="渠道二维码" />
          <span v-else-if="!qrLoading" class="qr__empty">二维码加载失败</span>
        </div>

        <el-alert
          v-if="qr && isRelativeUrl(qr.url)"
          type="warning"
          :closable="false"
          show-icon
          class="qr__alert"
        >
          二维码里的地址是相对路径，请先在后端
          <code>config.yaml</code> 配 <code>server.public_base</code>（重启后再生效），
          客户扫码后才会进入正确的域名。
        </el-alert>

        <div class="qr__url">
          <span class="qr__url-text">{{ qr ? qr.url : '--' }}</span>
          <el-button link type="primary" :disabled="!qr" @click="qr && copyText(qr.url, '接入地址已复制')">
            复制地址
          </el-button>
        </div>

        <div class="qr__size">
          <span class="qr__size-label">尺寸</span>
          <el-radio-group :model-value="qrSize" @change="changeQrSize">
            <el-radio-button v-for="size in QR_SIZE_OPTIONS" :key="size" :value="size">
              {{ size }}
            </el-radio-button>
          </el-radio-group>
        </div>
      </div>

      <template #footer>
        <el-button @click="qrVisible = false">关闭</el-button>
        <el-button type="primary" :disabled="!qr" @click="downloadQr">下载二维码</el-button>
      </template>
    </el-dialog>

    <!-- 渠道接待账号 -->
    <el-dialog v-model="agentVisible" width="480px" :close-on-click-modal="false">
      <template #header>
        <span class="dialog-title">设置「{{ agentChannelName }}」的接待客服</span>
      </template>

      <div v-loading="agentLoading" class="agents">
        <el-checkbox-group v-if="agentList.length" v-model="agentIds" class="agents__group">
          <div v-for="item in agentList" :key="item.id" class="agents__item">
            <el-checkbox :value="item.id">
              <span class="agents__cell" :class="{ 'agents__cell--off': !item.enabled }">
                <ThumbImage
                  :src="item.avatar"
                  :size="28"
                  round
                  kind="avatar"
                  :fallback="(item.name || item.account || '?').slice(0, 1)"
                  :alt="item.name"
                />
                <span class="agents__info">
                  <span class="agents__name">
                    {{ item.name || item.account }}
                    <span v-if="item.account" class="agents__account">{{ item.account }}</span>
                  </span>
                </span>
                <el-tag size="small" :type="item.online ? 'success' : 'info'" effect="light">
                  {{ item.online ? '在线' : '离线' }}
                </el-tag>
                <el-tag v-if="!item.enabled" size="small" type="info" effect="plain">已停用</el-tag>
              </span>
            </el-checkbox>
          </div>
        </el-checkbox-group>
        <div v-else-if="!agentLoading" class="agents__empty">暂无可分配的客服账号</div>
      </div>

      <template #footer>
        <div class="agents__footer">
          <el-button link type="primary" :disabled="!agentList.length" @click="selectAllAgents">
            全选
          </el-button>
          <el-button link type="primary" :disabled="!agentList.length" @click="clearAgents">
            清空
          </el-button>
          <span class="agents__spacer" />
          <el-button @click="agentVisible = false">取消</el-button>
          <el-button type="primary" :loading="agentSaving" @click="saveAgents">保存</el-button>
        </div>
      </template>
    </el-dialog>
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

  code {
    padding: 1px 4px;
    background: #eceef1;
    border-radius: 3px;
  }
}

.form-tip {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: #7f7f7f;

  code {
    padding: 2px 5px;
    background: #f2f3f5;
    border-radius: 3px;
    font-size: 12px;
    color: #4a6fa5;
    word-break: break-all;
  }
}

/* 渠道标识 app_id：输入 + 随机按钮 */
.appid-field {
  display: flex;
  gap: 8px;
  width: 100%;

  &__input {
    flex: 1;
  }
}

/* 表格里的 app_id 单元格 */
.appid-cell {
  display: flex;
  align-items: center;
  gap: 4px;

  &__code {
    padding: 2px 6px;
    background: #f2f3f5;
    border-radius: 3px;
    font-family: Consolas, Monaco, monospace;
    font-size: 12px;
    color: #4a6fa5;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.type-code {
  margin-left: 6px;
  font-size: 12px;
  color: #b2b2b2;
}

.pending-strong {
  color: #e6162d;
  font-weight: 600;
}

/* 用户端展示配置标签 */
.display-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  align-items: center;
  justify-content: center;
}

.display-none {
  font-size: 12px;
  color: #bfbfbf;
}

.dialog-title {
  font-size: 15px;
  font-weight: 600;
  color: #191919;
}

/* 二维码弹窗 */
.qr {
  &__image {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 8px;
    background: #fff;
    border: 1px solid #e5e5e5;
    border-radius: 6px;

    img {
      display: block;
      width: 220px;
      height: 220px;
      background: #fff;
      border-radius: 6px;
    }
  }

  &__empty {
    display: block;
    padding: 40px 0;
    font-size: 13px;
    color: #b2b2b2;
  }

  &__url {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    margin-top: 12px;
    padding: 8px 10px;
    background: #f5f5f5;
    border-radius: 4px;
  }

  &__url-text {
    flex: 1;
    font-size: 12px;
    line-height: 1.5;
    color: #191919;
    word-break: break-all;
  }

  &__size {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 12px;
  }

  &__size-label {
    font-size: 13px;
    color: #7f7f7f;
  }
}

/* 接待账号弹窗 */
.agents {
  max-height: 380px;
  overflow-y: auto;

  &__group {
    display: block;
    width: 100%;
  }

  &__item {
    padding: 6px 0;
    border-bottom: 1px solid #f2f2f2;

    &:last-child {
      border-bottom: none;
    }
  }

  &__cell {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }

  &__cell--off {
    opacity: 0.5;
  }

  &__info {
    display: inline-flex;
    flex-direction: column;
  }

  &__name {
    font-size: 13px;
    color: #191919;
  }

  &__account {
    margin-left: 6px;
    font-size: 12px;
    color: #b2b2b2;
  }

  &__empty {
    padding: 32px 0;
    text-align: center;
    font-size: 13px;
    color: #b2b2b2;
  }

  &__footer {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__spacer {
    flex: 1;
  }
}
</style>
