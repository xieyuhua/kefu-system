<template>
  <div class="workbench">
    <header class="workbench-topbar">
      <div class="topbar-left">
        <span class="topbar-title">客服工作台</span>
        <el-tag size="small" effect="plain" :type="socketStatus === 'open' ? 'success' : 'info'">
          {{ socketStatus === 'open' ? '实时连接正常' : '连接中…' }}
        </el-tag>
      </div>
      <div class="topbar-right">
        <span class="topbar-stat">待接待 {{ chat.stats.pending }}</span>
        <span class="topbar-stat">进行中 {{ chat.stats.open }}</span>
        <span class="topbar-stat">未读 {{ chat.stats.unread }}</span>
        <DemoPanel />
        <el-button size="small" :icon="Clock" @click="goHistory">历史档案</el-button>
        <el-button size="small" :icon="Refresh" @click="refresh">刷新</el-button>
      </div>
    </header>

    <div class="workbench-body">
      <ChannelSidebar />

      <ConversationList
        :conversations="chat.conversations"
        :current-id="chat.currentId"
        :loading="chat.listLoading"
        :total="chat.total"
        :stats="chat.stats"
        :status="chat.filters.status"
        @select="onSelect"
        @update:status="(s) => chat.setFilter({ status: s })"
        @update:keyword="onKeyword"
        @refresh="refresh"
      />

      <ChatWindow
        :conversation="chat.current"
        :messages="chat.messages"
        :loading="chat.messageLoading"
        :has-more="false"
        :panel-open="panelOpen"
        @send="onSend"
        @claim="onClaim"
        @transfer="onTransfer"
        @close="onClose"
        @toggle-panel="panelOpen = !panelOpen"
        @pick-order="orderPickerOpen = true"
        @pick-product="productPickerOpen = true"
        @load-more="() => {}"
      />

      <CustomerPanel
        v-if="panelOpen"
        :customer="customer"
        :orders="customerOrders"
        :loading="customerLoading"
        :can-send="canSend"
        @close="panelOpen = false"
        @send-order="sendOrderCard"
      />
    </div>

    <OrderPicker v-model="orderPickerOpen" :user-id="chat.current?.user?.id" @pick="onPickOrder" />
    <ProductPicker v-model="productPickerOpen" @pick="onPickProduct" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Clock, Refresh } from '@element-plus/icons-vue'
import ChannelSidebar from '@/components/ChannelSidebar.vue'
import ConversationList from '@/components/ConversationList.vue'
import ChatWindow from '@/components/ChatWindow.vue'
import CustomerPanel from '@/components/CustomerPanel.vue'
import DemoPanel from '@/components/DemoPanel.vue'
import OrderPicker from '@/components/OrderPicker.vue'
import ProductPicker from '@/components/ProductPicker.vue'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'
import { useSocket } from '@/composables/useSocket'
import { getCustomer } from '@/api/users'
import { listAgents, type Agent } from '@/api/agents'
import type {
  Conversation,
  Customer,
  Message,
  MessageType,
  NotificationPayload,
  Order,
  OrderCardPayload,
  Product,
  ProductCardPayload,
} from '@/types'

const auth = useAuthStore()
const chat = useChatStore()
const router = useRouter()
const { status: socketStatus, connect, close, on } = useSocket()

const panelOpen = ref(true)
const orderPickerOpen = ref(false)
const productPickerOpen = ref(false)

const customer = ref<Customer | null>(null)
const customerOrders = ref<Order[]>([])
const customerLoading = ref(false)
const agents = ref<Agent[]>([])

const canSend = computed(() => !!chat.current && chat.current.status !== 'closed')

const offs: Array<() => void> = []

async function loadCustomer(userId: number | undefined): Promise<void> {
  customer.value = null
  customerOrders.value = []
  if (!userId) return
  customerLoading.value = true
  try {
    const res = await getCustomer(userId)
    customer.value = res?.user || null
    customerOrders.value = res?.orders || []
  } catch {
    customer.value = null
    customerOrders.value = []
  } finally {
    customerLoading.value = false
  }
}

async function onSelect(conversation: Conversation): Promise<void> {
  await chat.selectConversation(conversation)
}

function refresh(): void {
  void chat.loadConversations()
  void chat.loadStats()
  void chat.loadChannels()
}

let keywordTimer = 0
function onKeyword(keyword: string): void {
  window.clearTimeout(keywordTimer)
  keywordTimer = window.setTimeout(() => {
    chat.setFilter({ keyword })
    void chat.loadConversations()
  }, 300)
}

async function onSend(payload: { type: MessageType; content: string }): Promise<void> {
  try {
    await chat.send(payload)
  } catch {
    ElMessage.error('发送失败，请重试')
  }
}

async function onPickOrder(order: Order): Promise<void> {
  const card: OrderCardPayload = {
    order_no: order.order_no,
    title: order.title,
    status: order.status,
    amount: order.amount,
    quantity: order.quantity,
    created_at: order.created_at,
    image: order.image,
    sku: order.sku,
  }
  await onSend({ type: 'order', content: JSON.stringify(card) })
}

async function onPickProduct(product: Product): Promise<void> {
  const card: ProductCardPayload = {
    id: product.id,
    title: product.title,
    price: product.price,
    image: product.image,
    stock: product.stock,
    sku: product.sku,
    url: product.url,
  }
  await onSend({ type: 'product', content: JSON.stringify(card) })
}

async function sendOrderCard(order: Order): Promise<void> {
  await onPickOrder(order)
}

async function onClaim(conversation: Conversation): Promise<void> {
  try {
    await chat.claim(conversation.id)
    ElMessage.success('已接入会话')
    await chat.loadConversations()
    await chat.loadStats()
  } catch {
    // 错误已提示
  }
}

async function onTransfer(conversation: Conversation): Promise<void> {
  if (!agents.value.length) {
    try {
      agents.value = await listAgents()
    } catch {
      agents.value = []
    }
  }
  const options = agents.value
    .filter((a) => a.id !== auth.user?.id)
    .map((a) => ({ value: a.id, label: `${a.name}（${a.account}）` }))
  if (!options.length) {
    ElMessage.warning('暂无可转接的客服')
    return
  }
  const optionsText = options.map((o, i) => `${i + 1}. ${o.label}`).join('\n')
  try {
    const { value } = await ElMessageBox.prompt(
      `可转接客服：\n${optionsText}\n\n请输入序号：`,
      '转接会话',
      {
        confirmButtonText: '转接',
        cancelButtonText: '取消',
        inputPattern: /^\d+$/,
        inputErrorMessage: '请输入正确的序号',
      },
    )
    const index = Number(value) - 1
    const target = options[index]
    if (!target) {
      ElMessage.warning('序号不存在')
      return
    }
    await chat.transfer(conversation.id, target.value)
    ElMessage.success('已转接')
    await chat.loadConversations()
  } catch {
    // 取消或失败
  }
}

async function onClose(conversation: Conversation): Promise<void> {
  try {
    await ElMessageBox.confirm('确定结束该会话吗？结束后将不可继续回复。', '结束会话', {
      type: 'warning',
      confirmButtonText: '结束',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await chat.close(conversation.id)
    ElMessage.success('会话已结束')
    await chat.loadConversations()
    await chat.loadStats()
  } catch {
    // 错误已提示
  }
}

function goHistory(): void {
  void router.push('/chat/history')
}

function notify(payload: NotificationPayload): void {
  ElMessage({
    message: `${payload.title}：${payload.body}`,
    type: 'info',
    duration: 3000,
  })
  if (window.Notification && window.Notification.permission === 'granted') {
    new window.Notification(payload.title, { body: payload.body })
  }
}

watch(
  () => chat.current?.user?.id,
  (id) => {
    void loadCustomer(id)
  },
)

watch(
  () => auth.token,
  (token) => {
    if (token) connect(token)
  },
  { immediate: true },
)

onMounted(() => {
  void chat.loadChannels()
  void chat.loadStats()
  void chat.loadConversations()
  if (auth.token) connect(auth.token)

  offs.push(on<Message>('message.new', (msg) => chat.onMessage(msg)))
  offs.push(on<Conversation>('conversation.new', (conv) => chat.onConversation(conv)))
  offs.push(on<Conversation>('conversation.updated', (conv) => chat.onConversation(conv)))
  offs.push(on<NotificationPayload>('notification.new', notify))

  if (window.Notification && window.Notification.permission === 'default') {
    void window.Notification.requestPermission()
  }
})

onBeforeUnmount(() => {
  offs.forEach((off) => off())
  close()
})
</script>

<style scoped lang="scss">
.workbench {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  background: var(--kf-bg);
}

.workbench-body {
  display: flex;
  flex: 1;
  min-height: 0;
}

.workbench-topbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  height: 40px;
  padding: 0 14px;
  background: #fff;
  border-bottom: 1px solid var(--kf-border);
}

.topbar-left,
.topbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.topbar-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--kf-text);
}

.topbar-stat {
  font-size: 12px;
  color: var(--kf-text-sub);
}
</style>
