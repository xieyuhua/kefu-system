/**
 * 客服工作台「PC / 平板 / 移动端自适应」布局逻辑验证（纯 Node，无依赖）。
 * 复刻 WorkbenchView 的分档与单栏切换逻辑，覆盖各屏幕宽度下应该展示哪些面板。
 *
 * 运行：node scripts/verify-responsive.mjs
 */

const MOBILE_BREAKPOINT = 768
const TABLET_BREAKPOINT = 1200

/** useScreenTier 的分档（与 CSS 媒体查询共用同一组断点） */
function resolveTier(width) {
  if (width <= MOBILE_BREAKPOINT) return 'mobile'
  if (width <= TABLET_BREAKPOINT) return 'tablet'
  return 'desktop'
}

/**
 * 复刻模板里的 v-show / v-if 条件：
 *   列表栏   : tier !== 'mobile' || mobileView === 'list'
 *   聊天区   : tier !== 'mobile' || mobileView === 'chat'
 *   资料固定栏: tier === 'desktop' && panelOpen
 *   资料抽屉 : tier !== 'desktop' && mobileProfileOpen
 *   图标导航 : tier !== 'mobile'
 */
function layout(width, { mobileView = 'list', panelOpen = true, profileOpen = false } = {}) {
  const tier = resolveTier(width)
  const isMobile = tier === 'mobile'
  return {
    tier,
    isMobile,
    list: !isMobile || mobileView === 'list',
    chat: !isMobile || mobileView === 'chat',
    sidePanel: tier === 'desktop' && panelOpen,
    drawer: tier !== 'desktop' && profileOpen,
    sidebar: !isMobile,
  }
}

function describe(v) {
  const parts = []
  if (v.sidebar) parts.push('图标导航')
  if (v.list) parts.push('列表')
  if (v.chat) parts.push('聊天')
  if (v.sidePanel) parts.push('资料栏')
  if (v.drawer) parts.push('资料抽屉')
  return parts.join(' + ')
}

const cases = [
  [1920, {}, '图标导航 + 列表 + 聊天 + 资料栏', 'PC 大屏：四栏并排'],
  [1440, {}, '图标导航 + 列表 + 聊天 + 资料栏', 'PC 常规'],
  [1280, {}, '图标导航 + 列表 + 聊天 + 资料栏', 'PC 窄屏（>1200 保留资料栏）'],
  [1201, {}, '图标导航 + 列表 + 聊天 + 资料栏', '桌面档临界'],
  [1200, {}, '图标导航 + 列表 + 聊天', '平板 1200：资料栏改抽屉'],
  [1024, {}, '图标导航 + 列表 + 聊天', 'iPad 横屏'],
  [900, {}, '图标导航 + 列表 + 聊天', '小平板'],
  [769, {}, '图标导航 + 列表 + 聊天', '刚高于移动断点'],
  [768, {}, '列表', '移动端：默认停在会话列表（隐藏图标导航）'],
  [768, { mobileView: 'chat' }, '聊天', '移动端：点会话后切到聊天'],
  [768, { mobileView: 'chat', profileOpen: true }, '聊天 + 资料抽屉', '移动端：客户资料用抽屉'],
  [1200, { profileOpen: true }, '图标导航 + 列表 + 聊天 + 资料抽屉', '平板：抽屉打开'],
  [414, {}, '列表', 'iPhone：列表占满全屏'],
  [414, { mobileView: 'chat' }, '聊天', 'iPhone：聊天占满全屏'],
  [390, { mobileView: 'chat', profileOpen: true }, '聊天 + 资料抽屉', '小屏 + 抽屉'],
  [375, { mobileView: 'chat' }, '聊天', 'iPhone SE'],
  [320, { mobileView: 'chat' }, '聊天', '极窄屏仍可用'],
  [1440, { panelOpen: false }, '图标导航 + 列表 + 聊天', 'PC 收起资料栏'],
]

let pass = 0
let fail = 0
console.log('=== 客服工作台 PC / 平板 / 移动端自适应验证 ===\n')

for (const [w, state, expect, note] of cases) {
  const v = layout(w, state)
  const got = describe(v)
  const ok = got === expect
  ok ? pass++ : fail++
  console.log(`${ok ? '✓' : '✗'} ${String(w).padStart(4)}px [${v.tier.padEnd(7)}] ${got}`)
  if (!ok) console.log(`   期望: ${expect}`)
  console.log(`         ${note}`)
}

console.log('\n=== 关键不变量 ===')
const invariants = [
  ['移动端始终隐藏图标导航', [320, 375, 414, 768].every((w) => layout(w).sidebar === false)],
  ['移动端列表与聊天互斥', layout(414).list === true && layout(414).chat === false],
  ['移动端切到聊天后列表隐藏', layout(414, { mobileView: 'chat' }).chat === true && layout(414, { mobileView: 'chat' }).list === false],
  ['移动端不出现固定资料栏', layout(414, { profileOpen: true }).sidePanel === false],
  ['平板不出现固定资料栏', layout(1024, { profileOpen: true }).sidePanel === false],
  ['桌面档保留固定资料栏', layout(1440).sidePanel === true],
  ['桌面档同时显示列表与聊天', layout(1440).list && layout(1440).chat],
  ['任意宽度至少有内容可见', [320, 768, 1024, 1440, 1920].every((w) => layout(w, { mobileView: 'chat' }).list || layout(w, { mobileView: 'chat' }).chat)],
]
for (const [name, ok] of invariants) {
  ok ? pass++ : fail++
  console.log(`${ok ? '✓' : '✗'} ${name}`)
}

console.log(`\n结果：${pass} 通过 / ${fail} 失败`)
if (fail > 0) process.exit(1)
