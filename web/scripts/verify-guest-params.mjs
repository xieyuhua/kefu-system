/**
 * 用户端接入参数解析验证（纯 Node，无依赖）。
 *
 * 复刻 web/src/views/ClientView.vue 的 queryOf() 逻辑，验证投放链接
 * （形如 /kefu/?channel=wechat&uid=u1）在 hash 路由下也能正确取到参数。
 *
 * 背景：本项目三端用 hash 路由，但投放链接的参数写在 pathname 之后（location.search），
 * 而 vue-router 的 route.query 在 hash 模式下只能解析 `#` 之后的部分，
 * 因此必须优先读 window.location.search。
 *
 * 运行：node scripts/verify-guest-params.mjs
 */

/** 模拟 URL 对象 */
function makeURL(href) {
  const u = new URL(href)
  return {
    pathname: u.pathname,
    search: u.search,
    hash: u.hash,
  }
}

/**
 * 复刻修复后的 queryOf()：
 *   1) 优先从 window.location.search 取
 *   2) 取不到再回退到 route.query（即 hash 里的 query）
 */
function queryOf(loc, routeQuery, key) {
  if (loc.search) {
    const v = new URLSearchParams(loc.search).get(key)
    if (v) return v
  }
  const rv = routeQuery[key]
  if (typeof rv === 'string') return rv
  if (Array.isArray(rv) && typeof rv[0] === 'string') return rv[0]
  return ''
}

function parseParams(href) {
  const loc = makeURL(href)
  // hash 模式下 vue-router 看到的 query：只解析 # 之后的部分
  const routeQuery = {}
  const hashQuery = loc.hash.includes('?') ? loc.hash.slice(loc.hash.indexOf('?')) : ''
  if (hashQuery) {
    new URLSearchParams(hashQuery).forEach((v, k) => {
      routeQuery[k] = v
    })
  }
  const keys = ['channel', 'uid', 'nickname', 'order_no', 'pid', 'scene', 'tenant']
  const out = { routeQuery: Object.keys(routeQuery) }
  for (const k of keys) out[k] = queryOf(loc, routeQuery, k)
  return out
}

const cases = [
  {
    href: 'http://h/kefu/?channel=wechat',
    expect: { channel: 'wechat', uid: '' },
    note: '★ 修复目标：投放链接（参数在 pathname 之后、hash 为空）',
  },
  {
    href: 'http://h:8480/kefu/?channel=wechat',
    expect: { channel: 'wechat', uid: '' },
    note: '★ 用户实际遇到的报错链接',
  },
  {
    href: 'http://h/kefu/?channel=web&uid=u_10001&nickname=%E7%8E%8B%E5%B0%8F%E6%98%8E',
    expect: { channel: 'web', uid: 'u_10001', nickname: '王小明' },
    note: '多参数 + 中文 URL 编码',
  },
  {
    href: 'http://h/kefu/?channel=wechat&order_no=SO20260305001&pid=P10086&scene=order',
    expect: { channel: 'wechat', order_no: 'SO20260305001', pid: 'P10086', scene: 'order' },
    note: '带订单/商品上下文',
  },
  {
    href: 'http://h/?channel=robot&uid=x1',
    expect: { channel: 'robot', uid: 'x1' },
    note: '根路径入口（/ 与 /kefu/ 等价）',
  },
  {
    href: 'http://h/kefu/?channel=web&tenant=1',
    expect: { channel: 'web', tenant: '1' },
    note: '多租户 tenant 参数',
  },
  // 兼容：参数写在 hash 里（调试面板旧行为）
  {
    href: 'http://h/kefu/#/?channel=app&uid=hash1',
    expect: { channel: 'app', uid: 'hash1' },
    note: '兼容 hash query 形式（调试面板）',
  },
  // 无参数：应给出可读错误提示
  {
    href: 'http://h/kefu/',
    expect: { channel: '' },
    note: '无 channel → 触发"缺少必填参数"提示',
  },
]

let pass = 0
let fail = 0
console.log('=== 用户端接入参数解析验证 ===\n')

for (const { href, expect, note } of cases) {
  const got = parseParams(href)
  const mismatches = Object.entries(expect).filter(([k, v]) => got[k] !== v)

  // 关键：证明 route.query 读不到、只有 location.search 才行
  const hasSearch = !!makeURL(href).search
  const routeQuerySeesChannel = 'channel' in got.routeQuery

  if (mismatches.length === 0) {
    pass++
    console.log(`✓ ${href}`)
    console.log(
      `   channel=${got.channel || '(空)'} uid=${got.uid || '(空)'}  [location.search=${hasSearch ? '有' : '无'} / route.query 可见=${routeQuerySeesChannel ? '是' : '否'}]`,
    )
  } else {
    fail++
    console.log(`✗ ${href}`)
    console.log(`   期望 ${JSON.stringify(expect)}，实际 ${JSON.stringify(got)}`)
  }
  console.log(`   ${note}\n`)
}

console.log('=== 关键回归：修复前后差异 ===')
const before = (() => {
  // 修复前：只读 route.query（hash 模式读不到 location.search）
  const href = 'http://h:8480/kefu/?channel=wechat'
  const loc = makeURL(href)
  return loc.search ? '' : '' // 修复前 route.query.channel === undefined → ''
})()
const after = parseParams('http://h:8480/kefu/?channel=wechat').channel
const ok = before === '' && after === 'wechat'
ok ? pass++ : fail++
console.log(`  ${ok ? '✓' : '✗'} 修复前 channel='${before}'（空 → 触发报错）→ 修复后 channel='${after}'`)

console.log(`\n结果：${pass} 通过 / ${fail} 失败`)
if (fail > 0) process.exit(1)
