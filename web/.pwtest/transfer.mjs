/**
 * 验证转接选择器：只显示该渠道内的客服 + 搜索下拉（不可输入任意值）。
 */
import { chromium } from 'playwright'

const BASE = 'http://127.0.0.1:8480'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })

console.log('=== 转接选择器验证 ===\n')

await page.goto(`${BASE}/chat/`, { waitUntil: 'networkidle' })
await page.waitForTimeout(600)
await page.locator('input').nth(0).fill('kf01')
await page.locator('input[type="password"]').fill('123456')
await page.locator('.login-submit').click()
await page.waitForTimeout(2600)

// 造一个「网页在线客服」渠道的会话（kf01 和 kf02 都接待网页？先查绑定）
const info = await page.evaluate(async () => {
  const token = localStorage.getItem('kefu_token')
  const r = await fetch('/api/agents', { headers: { Authorization: 'Bearer ' + token } }).then((x) => x.json())
  return (r.data || []).map((a) => ({
    id: a.id,
    name: a.name,
    role: a.role,
    channels: (a.channels || []).map((c) => `${c.id}:${c.name}`),
  }))
})
console.log('1) 客服与渠道绑定:')
info.forEach((a) => console.log(`   ${a.name}(${a.role}): ${a.channels.join(', ') || '(无)'}`))

// 选一个会话并点转接
const convs = await page.locator('.conv-item').count()
console.log(`\n2) 会话数: ${convs}`)
if (convs > 0) {
  await page.locator('.conv-item').first().click()
  await page.waitForTimeout(1500)

  const convChannel = await page.evaluate(() => {
    const el = document.querySelector('.chat-title, .chat-header')
    return el ? el.textContent.trim().slice(0, 30) : '(未知)'
  })
  console.log(`   当前会话渠道: ${convChannel}`)

  await page.locator('button:has-text("转接")').first().click()
  await page.waitForTimeout(1200)

  const dlg = await page.locator('.el-dialog:visible').count()
  const tipText = await page.locator('.el-dialog:visible .tip').innerText().catch(() => '')
  console.log(`\n3) 转接弹窗: ${dlg > 0 ? '✓ 已打开' : '✗ 未打开'}`)
  console.log(`   提示: ${tipText.replace(/\s+/g, ' ')}`)

  // 检查是否有输入框（不应该有自由输入）
  const inputs = await page.locator('.el-dialog:visible input').count()
  const isSelect = await page.locator('.el-dialog:visible .el-select').count()
  console.log(`\n4) 控件类型:`)
  console.log(`   el-select 下拉: ${isSelect > 0 ? '✓' : '✗'}`)
  console.log(`   输入框数量    : ${inputs}（el-select 内部的搜索框属正常）`)

  // 展开下拉看选项
  await page.locator('.el-dialog:visible .el-select').click()
  await page.waitForTimeout(1000)
  const opts = await page.locator('.el-select-dropdown__item:visible').allInnerTexts()
  console.log(`\n5) 下拉选项 (${opts.length} 个):`)
  opts.forEach((o) => console.log(`   - ${o.replace(/\s+/g, ' ').trim()}`))

  // 测试搜索
  await page.locator('.el-select-dropdown:visible input').first().fill('小')
  await page.waitForTimeout(900)
  const filtered = await page.locator('.el-select-dropdown__item:visible').allInnerTexts()
  console.log(`\n6) 搜索「小」后 (${filtered.length} 个):`)
  filtered.forEach((o) => console.log(`   - ${o.replace(/\s+/g, ' ').trim()}`))

  // 搜索不存在的
  await page.locator('.el-select-dropdown:visible input').first().fill('zzzz不存在')
  await page.waitForTimeout(900)
  const none = await page.locator('.el-select-dropdown__item:visible').count()
  const emptyTxt = await page.locator('.select-dropdown__empty, .empty').allInnerTexts().catch(() => [])
  console.log(`\n7) 搜索不存在:`)
  console.log(`   选项数: ${none}  ${none === 0 ? '✓' : '✗'}`)
  if (emptyTxt.filter(Boolean).length) {
    console.log(`   空状态: ${emptyTxt.filter(Boolean).join(' ').replace(/\s+/g, ' ')}`)
  }
}

await browser.close()
console.log('\nTRANSFER DONE')