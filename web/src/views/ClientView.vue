<template>
  <div class="client-view">
    <div class="client-shell">
      <!-- 顶部标题栏：客服名称 + 企业认证徽标。
           APP WebView 内嵌时隐藏（宿主 APP 自带导航栏）：
           URL ?chrome=0 / ?hide_header=1，或后台 渠道管理 → 隐藏 header。 -->
      <header v-if="!hideHeader" class="topbar">
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

        <div v-if="!hideDebug" class="topbar__right">
          <button
            type="button"
            class="topbar__btn"
            :class="{ 'is-on': debugOpen }"
            title="接入调试"
            @click="toggleDebug"
          >
            <el-icon :size="17"><Setting /></el-icon>
          </button>
        </div>
      </header>

      <!-- header 被隐藏时（APP 内嵌），用浮动按钮保留调试入口 -->
      <div v-if="hideHeader && !hideDebug" class="float-tools">
        <button
          type="button"
          class="float-tools__btn"
          :class="{ 'is-on': debugOpen }"
          title="接入调试"
          @click="toggleDebug"
        >
          <el-icon :size="15"><Setting /></el-icon>
        </button>
      </div>

      <!-- 接入调试面板（默认收起，右上角齿轮打开）。
           header 隐藏时上方有浮动按钮，需留出间距避免被遮挡。 -->
      <section v-if="debugOpen" class="debug" :class="{ 'debug--floated': hideHeader }">
        <div class="debug__head">
          <span class="debug__title">接入调试面板 · URL 参数解析</span>
          <button type="button" class="debug__close" @click="debugOpen = false">收起</button>
        </div>

        <div
          class="debug__url"
          :title="`点击复制：${currentUrl}`"
          @click="copyDebugUrl"
        >
          {{ currentUrl }}
          <span class="debug__url-tip">点击复制</span>
        </div>

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
      <div ref="scroller" class="chat kf-scroll" @scroll="onHistoryScroll">
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

          <!-- 历史消息上滑分页 -->
          <div v-if="loadingHistory" class="chat__pager">加载中…</div>
          <div v-else-if="!hasMoreHistory && stream.length" class="chat__pager">没有更多消息了</div>

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
                    'bubble--media': item.media,
                    'is-pending': item.pending,
                    'is-failed': item.failed,
                  }"
                >
                  <MediaBubble v-if="item.media" :payload="item.media" :failed="item.failed" />
                  <span v-else class="bubble__text">{{ item.text }}</span>
                </div>

                <div v-if="item.failed" class="row__error">发送失败，请重试</div>

                <div class="row__time">{{ item.time }}</div>
              </div>
            </div>
          </template>
        </template>
      </div>

      <!-- 订单/商品卡片气泡（拼多多客服式）：无遮罩，悬于输入栏上方，点卡片发送，点 ✕ 取消 -->
      <transition name="card-pop">
        <div v-if="cardConfirmVisible" class="card-pop">
          <button type="button" class="card-pop__close" aria-label="关闭" @click="dismissCard">
            ✕
          </button>
          <div class="card-pop__body">
            <div
              v-if="ctxOrder"
              class="card-confirm__item"
              role="button"
              :class="{ 'is-sending': sendingCard }"
              @click="sendContextCards"
            >
              <OrderCard :order="ctxOrder" />
            </div>
            <div
              v-if="ctxProduct"
              class="card-confirm__item"
              role="button"
              :class="{ 'is-sending': sendingCard }"
              @click="sendContextCards"
            >
              <ProductCard :product="ctxProduct" />
            </div>
          </div>
        </div>
      </transition>

      <!-- 底部输入栏 -->
      <footer class="composer">
        <div class="composer__bar">
          <!-- 语音输入：当前版本暂时隐藏，后续版本开放 -->

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
            class="composer__icon"
            title="发送图片 / 视频 / 文件"
            :disabled="uploading"
            @click="pickMedia"
          >
            <el-icon :size="20"><Paperclip /></el-icon>
          </button>
          <button
            type="button"
            class="composer__send"
            :disabled="!ready || !canSend || sending"
            @click="send"
          >
            {{ sending || uploading ? '发送中' : '发送' }}
          </button>
        </div>

        <input
          ref="fileRef"
          type="file"
          hidden
          accept="image/*,video/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.zip,.rar,.7z"
          @change="onPickFile"
        />

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
  Paperclip,
  Setting,
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { guestInit, guestMessages, guestSend, type GuestParams } from '@/api/guest'
import { kindOfFile, uploadAttachment, validateFile } from '@/api/upload'
import MediaBubble, { type MediaPayload } from '@/components/MediaBubble.vue'
import type {
  ChannelBasic,
  Message,
  MessageType,
  OrderCardPayload,
  ProductCardPayload,
} from '@/types'
import { channelIcon, channelLabel, formatMessageTime, parseCardContent } from '@/utils/format'
import { useSocket } from '@/composables/useSocket'
import OrderCard from '@/components/OrderCard.vue'
import ProductCard from '@/components/ProductCard.vue'

/**
 * 用户端 URL 上允许出现的参数（与 docs/API.md §13.1 一致）
 * 额外支持展示层参数：
 *   chrome=0 / hide_header=1 —— 隐藏顶部 header（APP WebView 内嵌用）
 *   hide_debug=1             —— 隐藏「接入调试」齿轮入口
 *   hide_more_menu=1         —— 已废弃（「更多」菜单已整体移除，仅保留解析以兼容旧链接）
 */
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
  'chrome',
  'hide_header',
  'hide_debug',
  'hide_more_menu',
] as const

const EMOJIS = ['😊', '😄', '😍', '🤔', '😅', '👍', '🙏', '👏', '❤️', '🎉', '😢', '😡', '📦', '🚚']

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
  /** 图片 / 视频 / 文件载荷 */
  media: MediaPayload | null
  time: string
  pending: boolean
  failed: boolean
}

const route = useRoute()
const router = useRouter()

/** 附件上传中 */
const uploading = ref(false)
/** 附件选择框 */
const fileRef = ref<HTMLInputElement | null>(null)

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
/** 进线带订单/商品卡片时的确认弹窗（电商咨询效果） */
const cardConfirmVisible = ref(false)
/** 正在发送卡片 */
const sendingCard = ref(false)
const messages = ref<Message[]>([])
const customerAvatar = ref('')

const sinceId = ref(0)
const draft = ref('')
const panel = ref<'' | 'emoji'>('')
const debugOpen = ref(false)

const scroller = ref<HTMLElement | null>(null)
let timer: number | undefined
/** 用户端实时连接（WebSocket；失败自动降级轮询） */
let socket: ReturnType<typeof useSocket> | null = null
let unsubscribers: (() => void)[] = []

/** 从 query 对象读取单个字符串参数（重复参数取第一个） */
function readParam(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

/**
 * 读取接入参数。
 *
 * 用户端使用 hash 路由，但投放链接的参数写在 pathname 之后：
 *   /kefu/?channel=wechat&uid=u1   ← 参数在 window.location.search
 * 而 vue-router 的 route.query 在 hash 模式下只解析 `#` 之后的部分，这里读不到。
 * 因此优先读 location.search，读不到再回退到 route.query（兼容 /kefu/#/?channel=... 形式）。
 */
function queryOf(key: string): string {
  if (typeof window !== 'undefined' && window.location.search) {
    const v = new URLSearchParams(window.location.search).get(key)
    if (v) return v
  }
  return readParam(route.query[key])
}

function parseParams(): GuestParams {
  return {
    channel: queryOf('channel'),
    uid: queryOf('uid'),
    nickname: queryOf('nickname'),
    avatar: queryOf('avatar'),
    phone: queryOf('phone'),
    order_no: queryOf('order_no'),
    pid: queryOf('pid'),
    scene: queryOf('scene'),
    visitor_id: queryOf('visitor_id'),
    tenant: queryOf('tenant'),
    sign: queryOf('sign'),
    chrome: queryOf('chrome'),
    hide_header: queryOf('hide_header'),
    hide_debug: queryOf('hide_debug'),
    hide_more_menu: queryOf('hide_more_menu'),
  }
}

/** URL 参数命中「真值」集合 */
const TRUTHY = ['1', 'true', 'yes', 'on']
/** URL 参数命中「假值」集合（chrome=0 这种写法） */
const FALSY = ['0', 'false', 'no', 'off']

function isTruthy(v?: string): boolean {
  return TRUTHY.includes((v || '').trim().toLowerCase())
}

function isFalsy(v?: string): boolean {
  return FALSY.includes((v || '').trim().toLowerCase())
}

/**
 * 顶部 header 是否隐藏（APP WebView 内嵌场景）。
 * 优先级：URL 参数 > 后台渠道配置。
 *   URL：?chrome=0 / ?hide_header=1
 *   后台：管理后台 → 渠道管理 → 该渠道「隐藏 header」
 */
const hideHeader = computed(() => {
  const fromURL = isFalsy(params.value.chrome) || isTruthy(params.value.hide_header)
  if (fromURL) return true
  // URL 显式要求显示时优先（chrome=1 / hide_header=0）
  if (isTruthy(params.value.chrome) || isFalsy(params.value.hide_header)) return false
  return channel.value?.hide_header === true
})

/**
 * 「接入调试」入口是否隐藏。
 * 优先级：URL 参数 > 后台渠道配置。
 * 注意与 hide_header 相互独立——header 隐藏时调试入口会移到浮动按钮上，
 * 但只要 hide_debug=1（或渠道配置开启），浮动按钮也不出现。
 */
const hideDebug = computed(() => {
  if (isTruthy(params.value.hide_debug)) return true
  if (isFalsy(params.value.hide_debug)) return false
  return channel.value?.hide_debug === true
})

/** 剥掉纯展示层参数，避免传给后端 */
function businessParams(): GuestParams {
  const { chrome: _c, hide_header: _h, hide_debug: _d, hide_more_menu: _m, ...rest } = params.value
  return rest
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

/**
 * 把服务端生成的游客 ID 同步到地址栏。
 * 这样刷新 / 分享链接时仍是同一个游客，不会每次都新建一个会话。
 */
function writeSinceKeyToUrl(uid: string): void {
  if (!uid || typeof window === 'undefined') return
  try {
    const url = new URL(window.location.href)
    if (url.searchParams.get('uid') === uid) return
    url.searchParams.set('uid', uid)
    window.history.replaceState(null, '', url.toString())
  } catch {
    // 地址栏改写失败不影响聊天，静默忽略
  }
}

function isSystem(m: Message): boolean {
  return m.direction === 'system' || m.type === 'system'
}

function isMine(m: Message): boolean {
  return m.direction === 'in'
}

/** 解析图片 / 视频 / 文件载荷，兼容历史写法（content 直接是 URL） */
function parseMedia(m: Message): MediaPayload | null {
  if (m.type !== 'image' && m.type !== 'video' && m.type !== 'file') return null
  const raw = m.content || ''
  if (!raw) return null
  if (raw.startsWith('{')) {
    const p = parseCardContent<MediaPayload>(raw)
    return p && p.url ? p : null
  }
  return {
    url: raw,
    kind: m.type === 'image' ? 'image' : m.type === 'video' ? 'video' : 'file',
  }
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
    media: parseMedia(m),
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

/* ---------------- 历史消息分页（微信式上滑加载） ---------------- */
/** 是否还有更早的历史可拉 */
const hasMoreHistory = ref(true)
/** 翻页加载中 */
const loadingHistory = ref(false)

/** 最早一条消息 id，作为向前翻页游标 */
const earliestId = computed(() => {
  const ids = messages.value.filter((m) => !m.local).map((m) => m.id)
  return ids.length ? Math.min(...ids) : 0
})

/**
 * 上滑加载更早的历史消息（id < earliestId）。
 * 与客服端一致：先记下原 scrollHeight，把新增高度补回 scrollTop，
 * 视觉上停留在原处不会跳动。
 */
async function loadMoreHistory(): Promise<void> {
  const el = scroller.value
  if (!el || loadingHistory.value || !hasMoreHistory.value) return
  const before = earliestId.value
  if (!before) {
    hasMoreHistory.value = false
    return
  }

  loadingHistory.value = true
  const prevHeight = el.scrollHeight
  const prevTop = el.scrollTop
  try {
    const list = await guestMessages(
      { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
      0,
      before,
      20,
    )
    if (!list.length) {
      hasMoreHistory.value = false
      return
    }
    // 不足一页说明到头了
    if (list.length < 20) hasMoreHistory.value = false
    prependMany(list)
    await nextTick()
    el.scrollTop = prevTop + (el.scrollHeight - prevHeight)
  } catch {
    // 加载失败静默，用户可再次上滑重试
  } finally {
    loadingHistory.value = false
  }
}

/** 顶部滚动到一定位置时触发翻页 */
function onHistoryScroll(): void {
  const el = scroller.value
  if (!el) return
  if (el.scrollTop <= 24) void loadMoreHistory()
}

/**
 * 滚动到最底部，展示最后一条消息。
 *
 * 不能只靠一次 nextTick：消息里的图片/视频是异步加载的，
 * 首屏那一刻真实高度还没算出来，滚下去后会「停在中途」。
 * 这里等高度连续稳定后再定位，图片较多时最多等 ~600ms。
 */
async function scrollToBottom(): Promise<void> {
  const el = scroller.value
  if (!el) return
  await nextTick()

  el.scrollTop = el.scrollHeight

  let lastHeight = el.scrollHeight
  let stable = 0
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => window.setTimeout(r, 40))
    if (!scroller.value) return
    const h = scroller.value.scrollHeight
    if (h === lastHeight) {
      stable++
      // 连续 3 次（约 120ms）高度不变，认为内容已渲染完毕
      if (stable >= 3) break
    } else {
      stable = 0
      lastHeight = h
    }
    el.scrollTop = el.scrollHeight
  }
  el.scrollTop = el.scrollHeight
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

/** 往前插入历史消息（分页加载用，按 id 去重） */
function prependMany(list: Message[] | null | undefined): void {
  if (!list || !list.length) return
  const known = new Set(messages.value.map((m) => m.id))
  const fresh = list.filter((m) => !known.has(m.id))
  if (!fresh.length) return
  messages.value = sortMessages(fresh.concat(messages.value))
}

/**
 * 拉取增量消息（WebSocket 不可用时由 useSocket 降级调用）
 */
async function pull(sinceOverride?: number): Promise<void> {
  if (!ready.value) return
  const sid = typeof sinceOverride === 'number' ? sinceOverride : sinceId.value
  try {
    const list = await guestMessages(
      { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
      sid,
    )
    const before = messages.value.length
    appendMany(list)
    if (messages.value.length !== before) void scrollToBottom()
  } catch {
    // 轮询失败静默重试，不打扰用户
  }
}

/**
 * 建立用户端 WebSocket：
 *  - 客服回复通过 message.new 实时到达（不必等轮询）
 *  - 重连时带 since_id，服务端回放离线期间累积的消息
 *  - 连接失败自动降级为轮询兜底（useSocket 内部实现）
 */
function connectSocket(): void {
  const key = `guest_${params.value.channel || 'x'}_${params.value.uid || 'anon'}`
  socket = useSocket(key)
  unsubscribers.forEach((fn) => fn())
  unsubscribers = []

  // 降级轮询的数据源：直接复用 pull（内部已做去重 + 滚动）
  socket.setFallbackFetch(async (sid: number) => {
    await pull(sid)
    return []
  })

  unsubscribers.push(
    socket.on<Message>('message.new', (m) => {
      removeLocal(m.id)
      upsertById(m)
      void scrollToBottom()
    }),
    socket.on('conversation.updated', (payload) => {
      const p = payload as { status?: string; transferred?: boolean }
      if (p?.status) convStatus.value = p.status
      if (p?.transferred) appendNotice('正在为您转接人工客服')
    }),
  )

  socket.setSinceId(sinceId.value)
  socket.connect({
    guest: {
      channel: params.value.channel || '',
      uid: params.value.uid || '',
      tenant: params.value.tenant || '',
    },
  })
}

/** 关闭用户端实时连接（含降级轮询） */
function stopPolling(): void {
  if (socket) {
    unsubscribers.forEach((fn) => fn())
    unsubscribers = []
    socket.close()
    socket = null
  }
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
    const res = await guestInit(businessParams())
    channel.value = res.channel ?? null
    conversationId.value = res.conversation?.id ?? null
    convStatus.value = res.conversation?.status ?? ''
    greeting.value = res.greeting || '您好，我是智能客服助手，请问有什么可以帮您？'
    ctxOrder.value = res.order ?? null
    ctxProduct.value = res.product ?? null
    // 进线带订单/商品卡片时，弹窗让用户确认是否发送给客服（电商咨询效果），可叉掉
    if (ctxOrder.value || ctxProduct.value) {
      cardConfirmVisible.value = true
    }
    customerAvatar.value = res.customer?.avatar || params.value.avatar || ''
    serviceAvatar.value = ''

    // 关键：链接没带 uid 时服务端会生成一个游客 ID，必须回写到 params。
    // 否则后续 WebSocket（/ws/guest 要求 uid）与轮询都会带空 uid，导致连接被拒。
    if (!params.value.uid && res.customer?.external_id) {
      const generatedUid = res.customer.external_id
      params.value = { ...params.value, uid: generatedUid }
      writeSinceKeyToUrl(generatedUid)
    }
    messages.value = sortMessages(res.history ?? [])
    // 历史超过一页才可继续上滑翻页
    hasMoreHistory.value = (res.history ?? []).length >= 20
    sinceId.value = readSince()
    updateSince()
    ready.value = true
    // 先关掉 loading 让消息真正渲染，再滚到底部
    loading.value = false
    await scrollToBottom()
    // 建立 WebSocket 实时连接（自带离线补拉与自动重连；失败会降级轮询）
    connectSocket()
  } catch (err) {
    ready.value = false
    errorText.value = err instanceof Error ? err.message : '接入失败，请检查链接参数'
  } finally {
    loading.value = false
  }
}

/** 选择并上传附件（图片 / 视频 / 文件） */
function pickMedia(): void {
  if (!ready.value || uploading.value) return
  fileRef.value?.click()
}

async function onPickFile(): Promise<void> {
  const el = fileRef.value
  const file = el?.files?.[0]
  if (!el || !file) return

  const err = validateFile(file)
  if (err) {
    ElMessage.error(err)
    el.value = ''
    return
  }

  uploading.value = true
  const tempId = -Date.now()
  // 先插入本地占位（乐观显示文件名），上传成功后回填真实载荷
  messages.value.push({
    id: tempId,
    conversation_id: conversationId.value ?? 0,
    type: kindOfFile(file),
    direction: 'in',
    content: JSON.stringify({ url: '', kind: kindOfFile(file), name: file.name, size: file.size }),
    sender: null,
    created_at: dayjs().format('YYYY-MM-DD HH:mm:ss'),
    pending: true,
    local: true,
  })
  void scrollToBottom()

  try {
    const payload = await uploadAttachment(file, {
      guest: true,
      params: {
        channel: params.value.channel || '',
        uid: params.value.uid || '',
        tenant: params.value.tenant || '',
      },
    })
    removeLocal(tempId)
    const content = JSON.stringify(payload)
    const res = await guestSend(
      { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
      { type: payload.kind, content },
    )
    upsertById({ ...res.message })
    if (res.reply) appendMany([res.reply])
    await scrollToBottom()
  } catch {
    ElMessage.error('上传失败，请重试')
    const idx = messages.value.findIndex((m) => m.id === tempId)
    if (idx >= 0) {
      messages.value[idx] = { ...messages.value[idx], pending: false, failed: true }
      messages.value = messages.value.slice()
    }
  } finally {
    uploading.value = false
    el.value = ''
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
    // 移除乐观气泡，再按 id upsert —— 避免与轮询/WebSocket 推来的同一条消息重复
    removeLocal(tempId)
    upsertById({ ...res.message })
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

/** 移除一条本地临时消息（乐观气泡 / 本地提示） */
function removeLocal(id: number): void {
  const idx = messages.value.findIndex((m) => m.id === id)
  if (idx >= 0) messages.value.splice(idx, 1)
}

/**
 * 发送进线带入的订单/商品卡片给客服（用户点了「发送给客服」）。
 * 卡片作为 type=order / type=product 的消息发送，发送成功后进入消息流。
 */
async function sendContextCards(): Promise<void> {
  if (sendingCard.value) return
  const order = ctxOrder.value
  const product = ctxProduct.value
  if (!order && !product) {
    cardConfirmVisible.value = false
    return
  }
  sendingCard.value = true
  try {
    const gs = (type: 'order' | 'product', payload: object) =>
      guestSend(
        { channel: params.value.channel, uid: params.value.uid, tenant: params.value.tenant },
        { type, content: JSON.stringify(payload) },
      )

    // 依次发送订单与商品卡片（一般只带其一）
    const sent: Message[] = []
    if (order) {
      const r = await gs('order', order)
      if (r?.message) sent.push(r.message)
      if (r?.reply) appendMany([r.reply])
    }
    if (product) {
      const r = await gs('product', product)
      if (r?.message) sent.push(r.message)
      if (r?.reply) appendMany([r.reply])
    }
    sent.forEach((m) => upsertById(m))
    messages.value = sortMessages(messages.value)
    ctxOrder.value = null
    ctxProduct.value = null
    cardConfirmVisible.value = false
    await scrollToBottom()
  } catch {
    ElMessage.error('发送失败，请重试')
  } finally {
    sendingCard.value = false
  }
}

/** 关闭卡片气泡（点 ✕）：不发卡片，清空上下文 */
function dismissCard(): void {
  ctxOrder.value = null
  ctxProduct.value = null
  cardConfirmVisible.value = false
}

/** 按 id 插入或替换消息：已存在则原地替换，杜绝重复气泡 */
function upsertById(m: Message): void {
  const idx = messages.value.findIndex((x) => x.id === m.id)
  if (idx >= 0) {
    messages.value[idx] = m
    return
  }
  messages.value.push(m)
  messages.value = sortMessages(messages.value)
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

function togglePanel(name: 'emoji'): void {
  panel.value = panel.value === name ? '' : name
}

function toggleDebug(): void {
  edit.channel = params.value.channel
  edit.uid = params.value.uid ?? ''
  edit.nickname = params.value.nickname ?? ''
  debugOpen.value = !debugOpen.value
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
  // 用 location.search 承载参数（与投放链接格式一致），不走 hash query
  const q = new URLSearchParams()
  const source = next as unknown as Record<string, string | undefined>
  for (const key of Object.keys(source)) {
    const value = source[key]
    if (value) q.set(key, value)
  }
  window.history.replaceState(null, '', `${window.location.pathname}?${q.toString()}`)

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

/** 点击调试面板里的地址即复制（带降级：非 https / 旧浏览器用 execCommand） */
async function copyDebugUrl(): Promise<void> {
  const text = currentUrl.value
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      legacyCopy(text)
    }
    ElMessage.success('接入地址已复制')
  } catch {
    legacyCopy(text)
    ElMessage.success('接入地址已复制')
  }
}

/** 老浏览器 / 非安全上下文下的复制兜底 */
function legacyCopy(text: string): void {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  try {
    document.execCommand('copy')
  } catch {
    ElMessage.warning('复制失败，请手动选择地址复制')
  }
  document.body.removeChild(ta)
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
/* header 隐藏时（APP 内嵌）的浮动工具按钮 */
.float-tools {
  position: absolute;
  top: 6px;
  right: 8px;
  z-index: 20;
  display: flex;
  gap: 4px;

  &__btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border: none;
    border-radius: 13px;
    background: rgba(0, 0, 0, 0.32);
    color: #fff;
    cursor: pointer;
    backdrop-filter: blur(2px);

    &.is-on {
      background: rgba(0, 0, 0, 0.55);
    }
  }

  &__menu {
    position: absolute;
    top: 32px;
    right: 0;
    min-width: 148px;
    padding: 4px 0;
    background: #fff;
    border-radius: 6px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.14);

    button {
      display: block;
      width: 100%;
      padding: 9px 14px;
      border: none;
      background: none;
      font-size: 13px;
      color: #191919;
      text-align: left;
      cursor: pointer;

      &:hover {
        background: #f5f5f5;
      }
    }
  }
}

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

  /* header 隐藏时右上角有浮动按钮，给「收起」留出空间 */
  &--floated {
    padding-top: 34px;
  }

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
    position: relative;
    padding: 6px 8px;
    background: #fff;
    border: 1px solid #e5e5e5;
    border-radius: 4px;
    font-size: 11px;
    color: #7f7f7f;
    word-break: break-all;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s;

    &:hover {
      border-color: #07c160;
      color: #07c160;
    }

    /* 悬停时右上角提示「点击复制」 */
    &-tip {
      position: absolute;
      top: 4px;
      right: 6px;
      padding: 1px 5px;
      border-radius: 3px;
      background: #07c160;
      color: #fff;
      font-size: 10px;
      line-height: 1.5;
      opacity: 0;
      transition: opacity 0.15s;
      pointer-events: none;
    }

    &:hover &-tip {
      opacity: 1;
    }
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

  /* 上滑分页提示（顶部） */
  &__pager {
    padding: 6px 0 10px;
    text-align: center;
    font-size: 12px;
    color: #9c9c9c;
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

  /* 媒体消息：去掉气泡底色与阴影，让图片/视频/文件自然显示 */
  &--media {
    padding: 4px;
    background: transparent;
    box-shadow: none;

    &::before {
      display: none;
    }
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

/* 订单/商品卡片气泡（拼多多客服式）：无遮罩，悬在输入栏上方，不遮挡输入按钮 */
.card-pop {
  position: absolute;
  right: 12px;
  /* 悬在输入栏上方：底部预留约 64px（输入栏高度），不压住表情/附件/发送按钮 */
  bottom: 72px;
  z-index: 30;
  width: 260px;
  max-width: calc(100% - 24px);
}

.card-pop__close {
  position: absolute;
  top: -10px;
  right: -10px;
  z-index: 1;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.45);
  color: #fff;
  font-size: 13px;
  line-height: 26px;
  cursor: pointer;

  &:hover {
    background: rgba(0, 0, 0, 0.65);
  }
}

.card-pop__body {
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.16);
  overflow: hidden;
}

/* 气泡弹入动画（右下角轻微上浮 + 淡入） */
.card-pop-enter-active,
.card-pop-leave-active {
  transition: opacity 0.22s ease, transform 0.22s ease;
}

.card-pop-enter-from,
.card-pop-leave-to {
  opacity: 0;
  transform: translateY(16px) scale(0.96);
}

.card-confirm {
  &__item {
    cursor: pointer;
    transition: opacity 0.15s, transform 0.1s;

    & + & {
      margin-top: 10px;
    }

    &:active {
      transform: scale(0.98);
    }

    &.is-sending {
      opacity: 0.6;
      pointer-events: none;
    }

    :deep(.order-card),
    :deep(.product-card) {
      width: 100%;
      border: 1px solid #e5e5e5;
      border-radius: 8px;
      overflow: hidden;
    }
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
