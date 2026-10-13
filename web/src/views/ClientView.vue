<template>
  <div class="client-view">
    <div class="client-shell">
      <!-- 顶部标题栏：客服名称 + 企业认证徽标 -->
      <header class="topbar">
        <button type="button" class="topbar__btn" title="返回" @click="goBack">
          <el-icon :size="20"><ArrowLeft /></el-icon>
        </button>

        <div class="topbar__center">
          <span class="topbar__title">{{ serviceName }}</span>
          <span class="topbar__badge">
            <el-icon :size="11"><CircleCheck /></el-icon>
            <span>企业</span>
          </span>
        </div>

        <div class="topbar__right">
          <button
            type="button"
            class="topbar__btn"
            :class="{ 'is-on': debugOpen }"
            title="接入调试"
            @click="toggleDebug"
          >
            <el-icon :size="17"><Setting /></el-icon>
          </button>
          <button type="button" class="topbar__btn" title="更多" @click="moreOpen = !moreOpen">
            <el-icon :size="18"><MoreFilled /></el-icon>
          </button>
        </div>

        <div v-if="moreOpen" class="more-menu">
          <button type="button" @click="copyLink">复制接入链接</button>
          <button type="button" @click="clearLocal">清空本地消息游标</button>
        </div>
      </header>

      <!-- 接入调试面板（默认收起，右上角齿轮打开） -->
      <section v-if="debugOpen" class="debug">
        <div class="debug__head">
          <span class="debug__title">接入调试面板 · URL 参数解析</span>
          <button type="button" class="debug__close" @click="debugOpen = false">收起</button>
        </div>

        <div class="debug__url">{{ currentUrl }}</div>

        <div class="debug__grid">
          <div v-for="row in paramRows" :key="row.key" class="debug__row">
            <span class="debug__key">
              {{ row.key }}<em v-if="row.required">*</em>
            </span>
            <span class="debug__val" :class="{ 'is-empty': !row.value }">
              {{ row.value || '（未传）' }}
            </span>
          </div>
        </div>

        <div class="debug__form">
          <label class="debug__field">
            <span>channel</span>
            <input v-model="edit.channel" type="text" placeholder="如 web / wechat / 1" />
          </label>
          <label class="debug__field">
            <span>uid</span>
            <input v-model="edit.uid" type="text" placeholder="业务侧用户 ID" />
          </label>
          <label class="debug__field">
            <span>nickname</span>
            <input v-model="edit.nickname" type="text" placeholder="用户昵称" />
          </label>
          <button type="button" class="debug__btn" :disabled="loading" @click="reInit">
            重新 init
          </button>
        </div>
        <div class="debug__tip">
          会话 ID：{{ conversationId ?? '--' }} ｜ 状态：{{ convStatus || '--' }} ｜ 轮询游标：{{
            sinceId
          }}
        </div>
      </section>

      <!-- 会话区 -->
      <div ref="scroller" class="chat kf-scroll">
        <div v-if="loading" class="chat__loading">正在接入客服…</div>

        <div v-else-if="errorText" class="chat__error">
          <p class="chat__error-title">接入失败</p>
          <p class="chat__error-text">{{ errorText }}</p>
          <p class="chat__error-hint">
            示例：<code>/c?channel=web&amp;uid=u_10001&amp;nickname=王小明</code>
          </p>
        </div>

        <template v-else>
          <div class="source-tag">
            <span class="source-tag__icon">{{ sourceIcon }}</span>
            <span>来自 {{ sourceName }}</span>
          </div>

          <!-- 机器人欢迎语 -->
          <div v-if="greeting" class="row row--service">
            <div class="row__avatar row__avatar--service">
              <img v-if="serviceAvatar" :src="serviceAvatar" alt="" />
              <span v-else>{{ BOT_AVATAR }}</span>
            </div>
            <div class="row__col">
              <div class="row__name">{{ greetingName }}</div>
              <div class="bubble">
                <span class="bubble__text">{{ greeting }}</span>
              </div>
              <div class="row__time">{{ greetingTime }}</div>
            </div>
          </div>

          <!-- 上下文卡片：order_no / pid 带入 -->
          <div v-if="ctxOrder || ctxProduct" class="ctx-wrap">
            <div v-if="ctxOrder" class="ctx-card">
              <OrderCard :order="ctxOrder" />
              <div class="ctx-card__foot" @click="openDetail(ctxOrder.url)">
                <span>订单 {{ ctxOrder.order_no }}</span>
                <span class="ctx-card__more">查看详情 ›</span>
              </div>
            </div>
            <div v-if="ctxProduct" class="ctx-card">
              <ProductCard :product="ctxProduct" />
              <div class="ctx-card__foot" @click="openDetail(ctxProduct.url)">
                <span>商品 {{ ctxProduct.id }}</span>
                <span class="ctx-card__more">查看详情 ›</span>
              </div>
            </div>
          </div>

          <template v-for="item in stream" :key="item.id">
            <!-- 系统通知：居中灰色 pill -->
            <div v-if="item.kind === 'system'" class="sys-pill">
              <span>{{ item.text }}</span>
            </div>

            <div v-else class="row" :class="item.kind === 'mine' ? 'row--mine' : 'row--service'">
              <div
                class="row__avatar"
                :class="item.kind === 'mine' ? 'row__avatar--mine' : 'row__avatar--service'"
              >
                <img v-if="item.avatar" :src="item.avatar" alt="" />
                <span v-else>{{ item.fallback }}</span>
              </div>

              <div class="row__col">
                <div v-if="item.kind === 'service'" class="row__name">{{ item.name }}</div>

                <div
                  v-if="item.order || item.product"
                  class="ctx-card"
                  :class="{ 'ctx-card--mine': item.kind === 'mine' }"
                >
                  <OrderCard v-if="item.order" :order="item.order" />
                  <ProductCard v-else-if="item.product" :product="item.product" />
                  <div class="ctx-card__foot">
                    <span>{{ item.order ? '订单卡片' : '商品卡片' }}</span>
                    <span class="ctx-card__more">查看详情 ›</span>
                  </div>
                </div>

                <div
                  v-else
                  class="bubble"
                  :class="{
                    'bubble--mine': item.kind === 'mine',
                    'is-pending': item.pending,
                    'is-failed': item.failed,
                  }"
                >
                  <img v-if="item.type === 'image'" class="bubble__image" :src="item.text" alt="图片" />
                  <span v-else class="bubble__text">{{ item.text }}</span>
                </div>

                <div v-if="item.failed" class="row__error">发送失败，请重试</div>

                <div class="row__time">{{ item.time }}</div>
              </div>
            </div>
          </template>
        </template>
      </div>

      <!-- 底部输入栏 -->
      <footer class="composer">
        <div class="composer__bar">
          <button type="button" class="composer__icon" title="语音输入" @click="onVoice">
            <el-icon :size="20"><Mic /></el-icon>
          </button>

          <div class="composer__box">
            <textarea
              v-model="draft"
              class="composer__input"
              rows="1"
              :disabled="!ready"
              placeholder="请输入您的问题…"
              @keydown.enter.exact.prevent="send"
            ></textarea>
          </div>

          <button
            type="button"
            class="composer__icon"
            :class="{ 'is-on': panel === 'emoji' }"
            title="表情"
            @click="togglePanel('emoji')"
          >
            😊
          </button>
          <button
            type="button"
            class="composer__icon composer__icon--plus"
            :class="{ 'is-on': panel === 'more' }"
            title="更多"
            @click="togglePanel('more')"
          >
            <el-icon :size="22"><Plus /></el-icon>
          </button>
          <button
            type="button"
            class="composer__send"
            :disabled="!ready || !canSend || sending"
            @click="send"
          >
            {{ sending ? '发送中' : '发送' }}
          </button>
        </div>

        <div v-if="panel === 'emoji'" class="panel">
          <button
            v-for="e in EMOJIS"
            :key="e"
            type="button"
            class="panel__emoji"
            @click="insert(e)"
          >
            {{ e }}
          </button>
        </div>

        <div v-else-if="panel === 'more'" class="panel">
          <button
            v-for="q in QUICK_QUESTIONS"
            :key="q"
            type="button"
            class="panel__quick"
            @click="useQuick(q)"
          >
            {{ q }}
          </button>
        </div>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft,
  CircleCheck,
  Mic,
  MoreFilled,
  Plus,
  Setting,
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { guestInit, guestMessages, guestSend, type GuestParams } from '@/api/guest'
import type {
  ChannelBasic,
  Message,
  MessageType,
  OrderCardPayload,
  ProductCardPayload,
} from '@/types'
import { channelIcon, channelLabel, formatMessageTime, parseCardContent } from '@/utils/format'
import OrderCard from '@/components/OrderCard.vue'
import ProductCard from '@/components/ProductCard.vue'

/** 用户端 URL 上允许出现的参数（与 docs/API.md §13.1 一致） */
const PARAM_KEYS = [
  'channel',
  'uid',
  'nickname',
  'avatar',
  'phone',
  'order_no',
  'pid',
  'scene',
  'visitor_id',
  'tenant',
  'sign',
] as const

const EMOJIS = ['😊', '😄', '😍', '🤔', '😅', '👍', '🙏', '👏', '❤️', '🎉', '😢', '😡', '📦', '🚚']
const QUICK_QUESTIONS = ['你好', '什么时候发货？', '怎么申请退款？', '我要转人工']

/** 消息流渲染项：把 Message 摊平成模板可直接用的结构 */
interface StreamItem {
  id: number
  kind: 'mine' | 'service' | 'system'
  type: MessageType
  text: string
  name: string
  avatar: string
  /** 客服侧头像缺失时用 emoji 兜底 */
  fallback: string
  order: OrderCardPayload | null
  product: ProductCardPayload | null
  time: string
  pending: boolean
  failed: boolean
}

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const ready = ref(false)
const errorText = ref('')
const sending = ref(false)

const channel = ref<ChannelBasic | null>(null)
const conversationId = ref<number | null>(null)
const convStatus = ref('')
const greeting = ref('')
const ctxOrder = ref<OrderCardPayload & { url?: string } | null>(null)
const ctxProduct = ref<ProductCardPayload | null>(null)
const messages = ref<Message[]>([])
const customerAvatar = ref('')

const sinceId = ref(0)
const draft = ref('')
const panel = ref<'' | 'emoji' | 'more'>('')
const debugOpen = ref(false)
const moreOpen = ref(false)

const scroller = ref<HTMLElement | null>(null)
let timer: number | undefined

/** 从 route.query 读取单个字符串参数（重复参数取第一个） */
function readParam(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

function parseParams(): GuestParams {
  const q = route.query
  return {
    channel: readParam(q.channel),
    uid: readParam(q.uid),
    nickname: readParam(q.nickname),
    avatar: readParam(q.avatar),
    phone: readParam(q.phone),
    order_no: readParam(q.order_no),
    pid: readParam(q.pid),
    scene: readParam(q.scene),
    visitor_id: readParam(q.visitor_id),
    tenant: readParam(q.tenant),
    sign: readParam(q.sign),
  }
}

const params = ref<GuestParams>(parseParams())
const edit = reactive({ channel: '', uid: '', nickname: '' })

const paramRows = computed(() =>
  PARAM_KEYS.map((key) => ({
    key,
    required: key === 'channel',
    value: (params.value[key] as string | undefined) || '',
  })),
)

const currentUrl = computed(() => window.location.href)

/** 机器人 / 智能助手固定署名与兜底头像（无 logo 资源，用 emoji 占位） */
const BOT_NAME = '智能助手'
const BOT_AVATAR = '🤖'

const serviceName = computed(() => channel.value?.name || '在线客服')
const sourceName = computed(() => {
  if (channel.value?.name) return channel.value.name
  return channelLabel(channel.value?.type)
})
const sourceIcon = computed(() => channelIcon(channel.value?.type))
const greetingName = computed(() => BOT_NAME)
const greetingTime = computed(() => dayjs().format('HH:mm'))
/** 客服头像：后端暂不下发客服头像，留空走 emoji 兜底 */
const serviceAvatar = ref('')

/** 轮询游标持久化：离线消息补拉 */
function sinceKey(): string {
  return `kefu_guest_since_${params.value.channel || 'x'}_${params.value.uid || 'guest'}`
}

function readSince(): number {
  const raw = Number(localStorage.getItem(sinceKey()))
  return Number.isFinite(raw) && raw > 0 ? raw : 0
}

function writeSince(id: number): void {
  localStorage.setItem(sinceKey(), String(id))
}

function isSystem(m: Message): boolean {
  return m.direction === 'system' || m.type === 'system'
}

function isMine(m: Message): boolean {
  return m.direction === 'in'
}

function toItem(m: Message): StreamItem {
  const kind = isSystem(m) ? 'system' : isMine(m) ? 'mine' : 'service'
  return {
    id: m.id,
    kind,
    type: m.type,
    text: m.content || '',
    name: m.sender?.name || '客服',
    avatar:
      kind === 'mine'
        ? customerAvatar.value || params.value.avatar || ''
        : m.sender?.avatar || serviceAvatar.value,
    fallback: kind === 'mine' ? '我' : '客',
    order: m.type === 'order' ? parseCardContent<OrderCardPayload>(m.content) : null,
    product: m.type === 'product' ? parseCardContent<ProductCardPayload>(m.content) : null,
    time: formatMessageTime(m.created_at),
    pending: !!m.pending,
    failed: !!m.failed,
  }
}

/**
 * 本地临时消息（乐观气泡 / 转人工提示）用负 id：
 * 用 id 升序排序会让它们跑到最前面，渲染时统一按「服务端 id 升序 + 本地消息按负 id 排在其后」处理。
 */
const sortKey = (m: Message): number => (m.local ? Number.MAX_SAFE_INTEGER + m.id : m.id)

function sortMessages(list: Message[]): Message[] {
  return list.slice().sort((a, b) => sortKey(a) - sortKey(b))
}

const stream = computed<StreamItem[]>(() => sortMessages(messages.value).map(toItem))

const canSend = computed(() => draft.value.trim().length > 0)

async function scrollToBottom(): Promise<void> {
  await nextTick()
  const el = scroller.value
  if (el) el.scrollTop = el.scrollHeight
}

function updateSince(): void {
  // 本地临时消息 id 为负数，不参与游标计算
  const max = messages.value.reduce((acc, m) => (m.local ? acc : m.id > acc ? m.id : acc), 0)
  if (max > sinceId.value) {
    sinceId.value = max
    writeSince(max)
  }
}

function appendMany(list: Message[] | null | undefined): void {
  if (!list || !list.length) return
  const known = new Set(messages.value.map((m) => m.id))
  const fresh = list.filter((m) => !known.has(m.id))
  if (!fresh.length) return
  messages.value = sortMessages(messages.value.concat(fresh))
  updateSince()
}

/** 降级轮询：无 WebSocket 时 3s 拉取增量消息 */
async function pull(): Promise<void> {
  if (!ready.value) return
  try {
    const list = await guestMessages(
      { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
      sinceId.value,
    )
    const before = messages.value.length
    appendMany(list)
    if (messages.value.length !== before) void scrollToBottom()
  } catch {
    // 轮询失败静默重试，不打扰用户
  }
}

function startPolling(): void {
  stopPolling()
  timer = window.setInterval(() => void pull(), 3000)
}

function stopPolling(): void {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
}

async function init(): Promise<void> {
  stopPolling()
  if (!params.value.channel) {
    loading.value = false
    ready.value = false
    errorText.value = '缺少必填参数 channel，请在链接后追加 ?channel=web'
    ElMessage.error(errorText.value)
    return
  }

  loading.value = true
  errorText.value = ''
  try {
    const res = await guestInit(params.value)
    channel.value = res.channel ?? null
    conversationId.value = res.conversation?.id ?? null
    convStatus.value = res.conversation?.status ?? ''
    greeting.value = res.greeting || '您好，我是智能客服助手，请问有什么可以帮您？'
    ctxOrder.value = res.order ?? null
    ctxProduct.value = res.product ?? null
    customerAvatar.value = res.customer?.avatar || params.value.avatar || ''
    serviceAvatar.value = ''
    messages.value = sortMessages(res.history ?? [])
    sinceId.value = readSince()
    updateSince()
    ready.value = true
    await scrollToBottom()
    void pull()
    startPolling()
  } catch (err) {
    ready.value = false
    errorText.value = err instanceof Error ? err.message : '接入失败，请检查链接参数'
  } finally {
    loading.value = false
  }
}

async function send(): Promise<void> {
  const content = draft.value.trim()
  if (!content || sending.value || !ready.value) return

  const tempId = -Date.now()
  messages.value.push({
    id: tempId,
    conversation_id: conversationId.value ?? 0,
    type: 'text',
    direction: 'in',
    content,
    sender: null,
    created_at: dayjs().format('YYYY-MM-DD HH:mm:ss'),
    pending: true,
    local: true,
  })
  draft.value = ''
  panel.value = ''
  void scrollToBottom()

  sending.value = true
  try {
    const res = await guestSend(
      { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
      { type: 'text', content },
    )
    // 乐观气泡替换成服务端返回的真实消息
    const idx = messages.value.findIndex((m) => m.id === tempId)
    if (idx >= 0) messages.value[idx] = { ...res.message }
    if (res.reply) appendMany([res.reply])
    if (res.transferred) {
      appendNotice('正在为您转接人工客服')
    }
    messages.value = sortMessages(messages.value)
    await scrollToBottom()
  } catch {
    const idx = messages.value.findIndex((m) => m.id === tempId)
    if (idx >= 0) {
      messages.value[idx] = { ...messages.value[idx], pending: false, failed: true }
      messages.value = messages.value.slice()
    }
  } finally {
    sending.value = false
  }
}

/** 插入一条本地系统提示（居中灰色 pill），不参与轮询游标 */
function appendNotice(text: string): void {
  messages.value.push({
    id: -Date.now(),
    conversation_id: conversationId.value ?? 0,
    type: 'system',
    direction: 'system',
    content: text,
    sender: null,
    created_at: dayjs().format('YYYY-MM-DD HH:mm:ss'),
    local: true,
  })
}

function insert(text: string): void {
  draft.value += text
}

function useQuick(text: string): void {
  draft.value = text
  panel.value = ''
}

function togglePanel(name: 'emoji' | 'more'): void {
  panel.value = panel.value === name ? '' : name
}

function onVoice(): void {
  ElMessage.info('演示页面暂未接入语音输入')
}

function openDetail(url?: string): void {
  if (url) window.open(url, '_blank')
}

function toggleDebug(): void {
  edit.channel = params.value.channel
  edit.uid = params.value.uid ?? ''
  edit.nickname = params.value.nickname ?? ''
  debugOpen.value = !debugOpen.value
  moreOpen.value = false
}

async function reInit(): Promise<void> {
  const next: GuestParams = { ...params.value }
  next.channel = edit.channel.trim()
  const uid = edit.uid.trim()
  const nickname = edit.nickname.trim()
  if (uid) next.uid = uid
  else delete next.uid
  if (nickname) next.nickname = nickname
  else delete next.nickname

  params.value = next
  const query: Record<string, string> = {}
  const source = next as unknown as Record<string, string | undefined>
  for (const key of Object.keys(source)) {
    const value = source[key]
    if (value) query[key] = value
  }
  await router.replace({ path: '/', query })

  messages.value = []
  ctxOrder.value = null
  ctxProduct.value = null
  greeting.value = ''
  await init()
}

function goBack(): void {
  if (window.history.length > 1) router.back()
  else void router.push('/')
}

async function copyLink(): Promise<void> {
  moreOpen.value = false
  try {
    await navigator.clipboard.writeText(window.location.href)
    ElMessage.success('接入链接已复制')
  } catch {
    ElMessage.warning('当前环境不支持剪贴板，请手动复制地址栏链接')
  }
}

function clearLocal(): void {
  moreOpen.value = false
  localStorage.removeItem(sinceKey())
  sinceId.value = 0
  ElMessage.success('已清空本地消息游标')
}

onMounted(() => {
  void init()
})

onBeforeUnmount(stopPolling)
</script>

<style scoped lang="scss">
.client-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: #ededed;
}

/* 移动端优先：最大宽度 480px；桌面端显示为手机壳 */
.client-shell {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 480px;
  height: 100vh;
  background: #ededed;
  overflow: hidden;
}

@media (min-width: 600px) {
  .client-view {
    background: #d8dadd;
    padding: 24px 0;
  }

  .client-shell {
    height: min(880px, calc(100vh - 48px));
    border-radius: 20px;
    box-shadow: 0 12px 48px rgba(0, 0, 0, 0.18);
  }
}

/* ---------------- 顶部标题栏 ---------------- */
.topbar {
  position: relative;
  flex: 0 0 52px;
  display: flex;
  align-items: center;
  padding: 0 6px;
  background: #fff;
  border-bottom: 1px solid #e5e5e5;

  &__btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    border: none;
    background: transparent;
    color: #191919;
    cursor: pointer;

    &.is-on {
      color: #07c160;
    }
  }

  &__center {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }

  &__title {
    font-size: 16px;
    font-weight: 600;
    color: #191919;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  &__badge {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 1px 5px;
    border-radius: 3px;
    background: rgba(7, 193, 96, 0.12);
    color: #07c160;
    font-size: 11px;
    line-height: 1.5;
  }

  &__right {
    display: flex;
    align-items: center;
  }
}

.more-menu {
  position: absolute;
  top: 50px;
  right: 8px;
  z-index: 20;
  min-width: 150px;
  padding: 4px;
  background: #fff;
  border: 1px solid #e5e5e5;
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);

  button {
    display: block;
    width: 100%;
    padding: 8px 10px;
    border: none;
    background: transparent;
    text-align: left;
    font-size: 13px;
    color: #191919;
    border-radius: 4px;
    cursor: pointer;

    &:hover {
      background: #f5f5f5;
    }
  }
}

/* ---------------- 接入调试面板 ---------------- */
.debug {
  flex: 0 0 auto;
  max-height: 42%;
  overflow-y: auto;
  padding: 10px 12px;
  background: #f7f8fa;
  border-bottom: 1px solid #e5e5e5;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
  }

  &__title {
    font-size: 13px;
    font-weight: 600;
    color: #191919;
  }

  &__close {
    border: none;
    background: transparent;
    font-size: 12px;
    color: #7f7f7f;
    cursor: pointer;
  }

  &__url {
    padding: 6px 8px;
    background: #fff;
    border: 1px solid #e5e5e5;
    border-radius: 4px;
    font-size: 11px;
    color: #7f7f7f;
    word-break: break-all;
  }

  &__grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 4px 10px;
    margin: 8px 0;
  }

  &__row {
    display: flex;
    align-items: baseline;
    gap: 6px;
    font-size: 12px;
  }

  &__key {
    color: #7f7f7f;

    em {
      color: #fa5151;
      font-style: normal;
    }
  }

  &__val {
    color: #191919;
    word-break: break-all;

    &.is-empty {
      color: #b2b2b2;
    }
  }

  &__form {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }

  &__field {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: #7f7f7f;

    input {
      width: 108px;
      padding: 4px 6px;
      border: 1px solid #e5e5e5;
      border-radius: 4px;
      font-size: 12px;
      color: #191919;
      outline: none;

      &:focus {
        border-color: #07c160;
      }
    }
  }

  &__btn {
    padding: 5px 10px;
    border: none;
    border-radius: 4px;
    background: #07c160;
    color: #fff;
    font-size: 12px;
    cursor: pointer;

    &:disabled {
      background: #b2b2b2;
      cursor: not-allowed;
    }
  }

  &__tip {
    margin-top: 6px;
    font-size: 11px;
    color: #b2b2b2;
  }
}

/* ---------------- 会话区 ---------------- */
.chat {
  flex: 1;
  min-height: 0;
  padding: 12px 12px 16px;
  background: #ededed;

  &__loading {
    padding: 24px 0;
    text-align: center;
    font-size: 13px;
    color: #7f7f7f;
  }

  &__error {
    margin: 24px 0;
    padding: 16px;
    background: #fff;
    border-radius: 8px;
    text-align: center;
  }

  &__error-title {
    margin: 0 0 6px;
    font-size: 15px;
    font-weight: 600;
    color: #fa5151;
  }

  &__error-text {
    margin: 0 0 8px;
    font-size: 13px;
    color: #191919;
  }

  &__error-hint {
    margin: 0;
    font-size: 12px;
    color: #7f7f7f;

    code {
      padding: 2px 4px;
      background: #f5f5f5;
      border-radius: 3px;
    }
  }
}

/* 来源渠道标签 */
.source-tag {
  display: flex;
  justify-content: center;
  margin-bottom: 12px;

  span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 10px;
    border-radius: 10px;
    background: rgba(0, 0, 0, 0.06);
    color: #7f7f7f;
    font-size: 12px;
  }

  &__icon {
    font-size: 12px;
  }
}

/* 气泡行 */
.row {
  display: flex;
  gap: 8px;
  padding: 5px 0;

  &--mine {
    flex-direction: row-reverse;
  }

  &__avatar {
    flex: 0 0 36px;
    width: 36px;
    height: 36px;
    border-radius: 4px;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;

    img {
      display: block;
      width: 100%;
      height: 100%;
      object-fit: cover;
    }

    &--service {
      background: #fff;
      color: #07c160;
    }

    &--mine {
      background: #dcdcdc;
      color: #191919;
    }
  }

  &__col {
    display: flex;
    flex-direction: column;
    max-width: 74%;
  }

  &--mine .row__col {
    align-items: flex-end;
  }

  &__name {
    margin-bottom: 3px;
    font-size: 12px;
    color: #7f7f7f;
  }

  &__time {
    margin-top: 3px;
    font-size: 11px;
    color: #b2b2b2;
  }

  &__error {
    margin-top: 3px;
    font-size: 11px;
    color: #fa5151;
  }
}

/* 气泡：圆角 8px + 小尖角，字号 15px / 行高 1.6 */
.bubble {
  position: relative;
  padding: 8px 12px;
  border-radius: 8px;
  background: #fff;
  color: #191919;
  font-size: 15px;
  line-height: 1.6;
  word-break: break-word;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);

  &::before {
    content: '';
    position: absolute;
    top: 12px;
    left: -5px;
    width: 0;
    height: 0;
    border: 5px solid transparent;
    border-right-color: #fff;
    border-left-width: 0;
  }

  &--mine {
    background: #95ec69;
    color: #191919;

    &::before {
      left: auto;
      right: -5px;
      border-right-color: transparent;
      border-left-color: #95ec69;
      border-left-width: 5px;
      border-right-width: 0;
    }
  }

  &.is-pending {
    opacity: 0.6;
  }

  &.is-failed {
    border: 1px solid #fa5151;
  }

  &__text {
    white-space: pre-wrap;
  }

  &__image {
    display: block;
    max-width: 160px;
    max-height: 180px;
    border-radius: 4px;
  }
}

/* 系统通知 pill */
.sys-pill {
  display: flex;
  justify-content: center;
  padding: 8px 0;

  span {
    display: inline-block;
    max-width: 78%;
    padding: 4px 12px;
    border-radius: 10px;
    background: rgba(0, 0, 0, 0.06);
    color: #7f7f7f;
    font-size: 12px;
    line-height: 1.4;
    text-align: center;
  }
}

/* 订单 / 商品卡片 */
.ctx-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 2px 0 8px 44px;
}

.ctx-card {
  width: 260px;
  overflow: hidden;
  background: #fff;
  border: 1px solid #e5e5e5;
  border-radius: 8px;

  &--mine {
    margin-left: auto;
  }

  :deep(.order-card),
  :deep(.product-card) {
    width: 100%;
    border: none;
    border-radius: 0;
  }

  &__foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 7px 10px;
    border-top: 1px solid #f0f0f0;
    font-size: 12px;
    color: #7f7f7f;
    cursor: pointer;
  }

  &__more {
    color: #07c160;
  }
}

/* ---------------- 底部输入栏 ---------------- */
.composer {
  flex: 0 0 auto;
  background: #fff;
  border-top: 1px solid #e5e5e5;

  &__bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
  }

  &__icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: transparent;
    font-size: 20px;
    color: #191919;
    cursor: pointer;

    &.is-on {
      color: #07c160;
    }

    &--plus {
      font-size: 18px;
    }
  }

  &__box {
    flex: 1;
    min-width: 0;
  }

  &__input {
    display: block;
    width: 100%;
    height: 34px;
    padding: 0 12px;
    border: 1px solid #e5e5e5;
    border-radius: 17px;
    outline: none;
    resize: none;
    font-family: inherit;
    font-size: 14px;
    line-height: 32px;
    color: #191919;
    background: #fff;

    &:focus {
      border-color: #07c160;
    }

    &:disabled {
      background: #f5f5f5;
    }
  }

  &__send {
    flex: 0 0 auto;
    height: 32px;
    padding: 0 14px;
    border: none;
    border-radius: 16px;
    background: #07c160;
    color: #fff;
    font-size: 14px;
    cursor: pointer;

    &:disabled {
      background: #c8e6d4;
      cursor: not-allowed;
    }
  }
}

.panel {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 10px 10px;
  border-top: 1px solid #f0f0f0;

  &__emoji {
    width: 34px;
    height: 34px;
    border: none;
    background: transparent;
    font-size: 20px;
    border-radius: 4px;
    cursor: pointer;

    &:hover {
      background: #f5f5f5;
    }
  }

  &__quick {
    padding: 6px 10px;
    border: 1px solid #e5e5e5;
    border-radius: 14px;
    background: #fff;
    font-size: 13px;
    color: #191919;
    cursor: pointer;

    &:hover {
      border-color: #07c160;
      color: #07c160;
    }
  }
}
</style>
