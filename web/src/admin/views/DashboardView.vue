<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getStatsOverview, type StatsOverviewResult, type TrendPoint } from '@/shared/api/admin'
import ThumbImage from '@/shared/components/ThumbImage.vue'

const DAY_OPTIONS = [7, 14, 30]

const loading = ref(false)
const days = ref(7)
const data = ref<StatsOverviewResult | null>(null)

async function load(): Promise<void> {
  loading.value = true
  try {
    data.value = await getStatsOverview(days.value)
  } catch {
    data.value = null
    ElMessage.error('统计数据加载失败')
  } finally {
    loading.value = false
  }
}

function changeDays(value: number | string): void {
  days.value = Number(value) || 7
  void load()
}

const summary = computed(() => data.value?.summary ?? null)

const cards = computed(() => {
  const s = summary.value
  return [
    { key: 'total', label: '总会话', value: s?.total_conversations ?? 0, extra: `最近 ${days.value} 天`, accent: '#07C160' },
    { key: 'rate', label: '接待率', value: `${(s?.handled_rate ?? 0).toFixed(1)}%`, extra: '已接待 / 总会话', accent: '#2C7BE5' },
    { key: 'avg', label: '平均消息数', value: (s?.avg_messages ?? 0).toFixed(1), extra: '条 / 会话', accent: '#8B5CF6' },
    { key: 'customers', label: '客户数', value: s?.customers ?? 0, extra: '累计接待客户', accent: '#FF8F1F' },
    { key: 'new', label: '新增客户', value: s?.new_customers ?? 0, extra: `最近 ${days.value} 天`, accent: '#E6162D' },
    { key: 'online', label: '在线客服', value: s?.online_agents ?? 0, extra: '当前在线', accent: '#0BB2D4' },
  ]
})

const trend = computed<TrendPoint[]>(() => data.value?.trend ?? [])
const agents = computed(() => data.value?.agents ?? [])
const channels = computed(() => data.value?.channels ?? [])

/* ---------------- 手绘 SVG 折线图（不引第三方图表库） ---------------- */

const CHART_H = 240
const PAD = { top: 16, right: 16, bottom: 26, left: 40 }

/** 图表宽度随容器自适应，保证 SVG 像素 1:1、文字不被拉伸 */
const chartW = ref(720)
const plotH = CHART_H - PAD.top - PAD.bottom
const plotW = computed(() => Math.max(120, chartW.value - PAD.left - PAD.right))

const chartBox = ref<HTMLElement | null>(null)
let observer: ResizeObserver | null = null

function syncWidth(): void {
  const el = chartBox.value
  if (el) chartW.value = Math.max(320, Math.round(el.clientWidth))
}

const maxValue = computed(() => {
  const max = trend.value.reduce((acc, p) => Math.max(acc, p.total, p.handled), 0)
  return max > 0 ? max : 1
})

interface ChartPoint {
  index: number
  date: string
  x: number
  yTotal: number
  yHandled: number
  total: number
  handled: number
}

const points = computed<ChartPoint[]>(() => {
  const list = trend.value
  if (!list.length) return []
  const width = plotW.value
  const step = list.length > 1 ? width / (list.length - 1) : 0
  return list.map((p, index) => ({
    index,
    date: p.date,
    x: PAD.left + (list.length > 1 ? index * step : width / 2),
    yTotal: PAD.top + plotH - (p.total / maxValue.value) * plotH,
    yHandled: PAD.top + plotH - (p.handled / maxValue.value) * plotH,
    total: p.total,
    handled: p.handled,
  }))
})

function pathOf(key: 'yTotal' | 'yHandled'): string {
  return points.value
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p[key].toFixed(1)}`)
    .join(' ')
}

const areaTotal = computed(() => {
  const pts = points.value
  if (!pts.length) return ''
  const line = pathOf('yTotal')
  const base = (PAD.top + plotH).toFixed(1)
  return `${line} L${pts[pts.length - 1].x.toFixed(1)},${base} L${pts[0].x.toFixed(1)},${base} Z`
})

const gridTicks = computed(() => {
  const ticks = [0, 0.25, 0.5, 0.75, 1]
  return ticks.map((t) => ({ y: PAD.top + plotH - t * plotH, value: Math.round(maxValue.value * t) }))
})

/** x 轴标签：数据点多时隔位显示，避免挤在一起 */
const xLabels = computed(() => {
  const pts = points.value
  if (!pts.length) return []
  const step = Math.max(1, Math.ceil(pts.length / 8))
  return pts.filter((p) => p.index % step === 0 || p.index === pts.length - 1)
})

const hoverIndex = ref<number | null>(null)

const hover = computed(() => (hoverIndex.value === null ? null : points.value[hoverIndex.value] ?? null))

/** tooltip 位置：靠右时向左翻转，避免溢出图表 */
const tipStyle = computed(() => {
  const p = hover.value
  if (!p) return {}
  const flip = p.x > chartW.value - 150
  return {
    left: `${(flip ? p.x - 140 : p.x + 10) / chartW.value * 100}%`,
    top: `${Math.min(p.yTotal, p.yHandled) / CHART_H * 100}%`,
  }
})

function onMove(event: MouseEvent): void {
  const target = event.currentTarget as SVGSVGElement
  const rect = target.getBoundingClientRect()
  const ratio = (event.clientX - rect.left) / rect.width
  const x = ratio * chartW.value
  let best: number | null = null
  let bestDist = Number.POSITIVE_INFINITY
  for (const p of points.value) {
    const dist = Math.abs(p.x - x)
    if (dist < bestDist) {
      bestDist = dist
      best = p.index
    }
  }
  hoverIndex.value = best
}

function onLeave(): void {
  hoverIndex.value = null
}

/* ---------------- 渠道分布：横向条形占比 ---------------- */

const channelRows = computed(() => {
  const max = channels.value.reduce((acc, c) => Math.max(acc, c.total), 0)
  return channels.value.map((c) => ({
    id: c.channel_id,
    name: c.name,
    type: c.type,
    total: c.total,
    pending: c.pending,
    open: c.open,
    closed: c.closed,
    percent: max > 0 ? Math.round((c.total / max) * 100) : 0,
  }))
})

const channelTotalSum = computed(() => channels.value.reduce((acc, c) => acc + c.total, 0))

function shortDate(date: string): string {
  return date.length >= 10 ? date.slice(5) : date
}

function formatRate(value: number): string {
  return `${Number(value || 0).toFixed(1)}%`
}

onMounted(() => {
  syncWidth()
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(syncWidth)
    if (chartBox.value) observer.observe(chartBox.value)
  }
  void load()
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<template>
  <div class="admin">
    <div v-loading="loading" class="dashboard">
      <div class="head">
        <span class="head__title">数据概览</span>
        <div class="head__right">
          <el-radio-group :model-value="days" size="small" @change="changeDays">
            <el-radio-button v-for="d in DAY_OPTIONS" :key="d" :value="d">
              {{ d }} 天
            </el-radio-button>
          </el-radio-group>
          <el-button :icon="Refresh" link @click="load">刷新</el-button>
        </div>
      </div>

      <div class="stat-grid">
        <div v-for="card in cards" :key="card.key" class="stat-card">
          <span class="stat-card__bar" :style="{ background: card.accent }"></span>
          <div class="stat-card__body">
            <div class="stat-card__value">{{ card.value }}</div>
            <div class="stat-card__label">{{ card.label }}</div>
            <div class="stat-card__extra">{{ card.extra }}</div>
          </div>
        </div>
      </div>

      <!-- 最近 N 天接待趋势（纯 SVG 手绘） -->
      <div class="admin-card">
        <div class="section-head">
          <span class="section-head__title">最近 {{ days }} 天接待趋势</span>
          <span class="section-head__sub">
            <i class="dot dot--total"></i>会话数
            <i class="dot dot--handled"></i>已接待数
          </span>
        </div>

        <el-empty v-if="!points.length" description="暂无趋势数据" :image-size="70" />

        <div v-else ref="chartBox" class="chart">
          <svg
            class="chart__svg"
            :viewBox="`0 0 ${chartW} ${CHART_H}`"
            :width="chartW"
            :height="CHART_H"
            @mousemove="onMove"
            @mouseleave="onLeave"
          >
            <!-- 网格线 + y 轴刻度 -->
            <g class="grid">
              <template v-for="tick in gridTicks" :key="tick.y">
                <line :x1="PAD.left" :y1="tick.y" :x2="chartW - PAD.right" :y2="tick.y" />
                <text :x="PAD.left - 8" :y="tick.y + 4" text-anchor="end">{{ tick.value }}</text>
              </template>
            </g>

            <!-- 会话数面积 -->
            <path :d="areaTotal" class="area" />
            <path :d="pathOf('yHandled')" class="line line--handled" />
            <path :d="pathOf('yTotal')" class="line line--total" />

            <!-- 数据点 -->
            <g>
              <circle
                v-for="p in points"
                :key="`t-${p.index}`"
                :cx="p.x"
                :cy="p.yTotal"
                r="3"
                class="node node--total"
              />
              <circle
                v-for="p in points"
                :key="`h-${p.index}`"
                :cx="p.x"
                :cy="p.yHandled"
                r="3"
                class="node node--handled"
              />
            </g>

            <!-- hover 指示线 -->
            <line
              v-if="hover"
              class="hover-line"
              :x1="hover.x"
              :y1="PAD.top"
              :x2="hover.x"
              :y2="PAD.top + plotH"
            />

            <!-- x 轴标签 -->
            <g class="axis">
              <text v-for="p in xLabels" :key="`x-${p.index}`" :x="p.x" :y="CHART_H - 8" text-anchor="middle">
                {{ shortDate(p.date) }}
              </text>
            </g>
          </svg>

          <div v-if="hover" class="chart__tip" :style="tipStyle">
            <div class="chart__tip-date">{{ hover.date }}</div>
            <div class="chart__tip-row">
              <i class="dot dot--total"></i>会话数 <b>{{ hover.total }}</b>
            </div>
            <div class="chart__tip-row">
              <i class="dot dot--handled"></i>已接待 <b>{{ hover.handled }}</b>
            </div>
          </div>
        </div>
      </div>

      <!-- 渠道分布 -->
      <div class="admin-card">
        <div class="section-head">
          <span class="section-head__title">渠道分布</span>
          <span class="section-head__sub">
            共 {{ channels.length }} 个渠道 · 最近 {{ days }} 天 {{ channelTotalSum }} 个会话
          </span>
        </div>

        <el-empty v-if="!channelRows.length" description="暂无渠道数据" :image-size="70" />
        <div v-else class="dist-list">
          <div v-for="item in channelRows" :key="item.id" class="dist-item">
            <div class="dist-item__head">
              <span class="dist-item__name">{{ item.name }}</span>
              <span class="dist-item__total">共 {{ item.total }} 个会话</span>
            </div>

            <div class="dist-item__bar">
              <span class="bar">
                <span class="bar__total" :style="{ width: item.percent + '%' }"></span>
              </span>
              <span class="dist-item__percent">{{ item.percent }}%</span>
            </div>

            <div class="dist-item__meta">
              <span>待接待 <b class="c-pending">{{ item.pending }}</b></span>
              <span>进行中 <b class="c-open">{{ item.open }}</b></span>
              <span>已结束 <b class="c-closed">{{ item.closed }}</b></span>
              <span class="muted">类型 {{ item.type }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 客服工作量 -->
      <div class="admin-card">
        <div class="section-head">
          <span class="section-head__title">客服工作量</span>
          <span class="section-head__sub">
            接待数 / 结束数 / 进行中为累计值，消息数为最近 {{ days }} 天
          </span>
        </div>

        <el-table class="admin-table" :data="agents" stripe border empty-text="暂无客服数据">
          <el-table-column label="客服" min-width="180">
            <template #default="{ row }">
              <span class="agent">
                <ThumbImage
                  :src="row.avatar"
                  :size="26"
                  round
                  kind="avatar"
                  :fallback="row.name.slice(0, 1)"
                  :alt="row.name"
                />
                <span>{{ row.name }}</span>
                <el-tag
                  v-if="row.role === 'admin'"
                  size="small"
                  type="warning"
                  effect="plain"
                  class="agent__role"
                >管理员</el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="handled" label="接待数" width="110" align="center" sortable />
          <el-table-column prop="closed" label="结束数" width="110" align="center" sortable />
          <el-table-column prop="messages" label="消息数" width="110" align="center" sortable>
            <template #default="{ row }">
              <span :title="`最近 ${days} 天发送`">{{ row.messages ?? 0 }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="active_now" label="进行中" width="110" align="center" sortable>
            <template #default="{ row }">
              <span :class="{ 'in-progress': (row.active_now ?? 0) > 0 }">{{ row.active_now ?? 0 }}</span>
            </template>
          </el-table-column>
          <el-table-column label="结束率" min-width="150">
            <template #default="{ row }">
              <span class="muted">
                {{ row.handled > 0 ? formatRate((row.closed / row.handled) * 100) : '--' }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
@use './shared.scss';

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  &__title {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 12px;
  overflow: hidden;
  background: #fff;
  border-radius: 6px;
  padding: 16px 16px 16px 0;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);

  &__bar {
    width: 3px;
    flex: 0 0 3px;
    border-radius: 3px 0 0 3px;
  }

  &__body {
    padding-left: 14px;
  }

  &__value {
    font-size: 24px;
    font-weight: 600;
    color: #191919;
    line-height: 1.2;
  }

  &__label {
    margin-top: 2px;
    font-size: 13px;
    color: #191919;
  }

  &__extra {
    margin-top: 2px;
    font-size: 12px;
    color: #7f7f7f;
  }
}

.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;

  &__title {
    font-size: 15px;
    font-weight: 600;
    color: #191919;
  }

  &__sub {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: #7f7f7f;
  }
}

.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 3px;

  &--total {
    background: #07c160;
  }

  &--handled {
    background: #2c7be5;
    margin-left: 8px;
  }
}

/* ---------------- 折线图 ---------------- */
.chart {
  position: relative;

  &__svg {
    display: block;
    width: 100%;
    max-width: 100%;
    height: 240px;
  }

  &__tip {
    position: absolute;
    transform: translateY(-50%);
    min-width: 120px;
    padding: 8px 10px;
    background: rgba(25, 25, 25, 0.85);
    border-radius: 4px;
    color: #fff;
    font-size: 12px;
    line-height: 1.6;
    pointer-events: none;
    white-space: nowrap;
  }

  &__tip-date {
    margin-bottom: 2px;
    color: #d8d8d8;
  }

  &__tip-row {
    b {
      margin-left: 4px;
      font-weight: 600;
    }
  }
}

.grid line {
  stroke: #f0f0f0;
  stroke-width: 1;
}

.grid text {
  fill: #b2b2b2;
  font-size: 10px;
}

.axis text {
  fill: #7f7f7f;
  font-size: 10px;
}

.area {
  fill: rgba(7, 193, 96, 0.12);
}

.line {
  fill: none;
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;

  &--total {
    stroke: #07c160;
  }

  &--handled {
    stroke: #2c7be5;
  }
}

.node {
  &--total {
    fill: #07c160;
  }

  &--handled {
    fill: #2c7be5;
  }
}

.hover-line {
  stroke: #b2b2b2;
  stroke-width: 1;
  stroke-dasharray: 3 3;
}

/* ---------------- 渠道分布 ---------------- */
.dist-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dist-item {
  padding: 10px 12px;
  border: 1px solid #e5e5e5;
  border-radius: 6px;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  &__name {
    font-size: 14px;
    color: #191919;
  }

  &__total {
    font-size: 12px;
    color: #7f7f7f;
  }

  &__bar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 8px 0 6px;
  }

  &__percent {
    flex: 0 0 40px;
    text-align: right;
    font-size: 12px;
    color: #7f7f7f;
  }

  &__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    font-size: 12px;
    color: #191919;
  }
}

.bar {
  flex: 1;
  height: 8px;
  background: #f2f3f5;
  border-radius: 4px;
  overflow: hidden;

  &__total {
    display: block;
    height: 100%;
    background: linear-gradient(90deg, #07c160, #4ad07f);
    border-radius: 4px;
    transition: width 0.3s ease;
  }
}

.c-pending {
  color: #e6162d;
}

.c-open {
  color: #07c160;
}

.c-closed {
  color: #7f7f7f;
}

.agent {
  display: inline-flex;
  align-items: center;
  gap: 8px;

  &__role {
    flex-shrink: 0;
  }
}

.in-progress {
  color: #07c160;
  font-weight: 600;
}

.muted {
  color: #7f7f7f;
}
</style>
