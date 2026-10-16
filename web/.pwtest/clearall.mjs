/**
 * 验证客服工作台「一键清空」。
 */
import { chromium } from 'playwright'

const BASE = 'http://127.0.0.1:8480'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })

let bulkCalls = 0
page.on('request', (r) => {
  if (r.url().includes('archive-bulk')) bulkCalls++
})

console.log('=== 客服工作台「一键清空」验证 ===\n')

await page.goto(`${BASE}/chat/`, { waitUntil: 'networkidle' })
await page.waitForTimeout(600)
await page.locator('input').nth(0).fill('kf01')
await page.locator('input[type="password"]').fill('123456')
await page.locator('.login-submit').click()
await page.waitForTimeout(2600)

const convBefore = await page.locator('.conv-item').count()
console.log(`1) 登录后会话数: ${convBefore}`)

const btn = page.locator('button:has-text("一键清空")')
console.log(`2) 「一键清空」按钮存在: ${(await btn.count()) > 0 ? '✓' : '✗'}`)

// 点按钮 → 应弹确认框
await btn.click()
await page.waitForTimeout(1000)
const dlgText = await page.locator('.el-message-box').innerText().catch(() => '')
console.log(`3) 二次确认弹窗: ${dlgText ? '✓' : '✗ 未弹出'}`)
console.log('   提示内容:')
dlgText
  .split('\n')
  .filter((l) => l.trim())
  .forEach((l) => console.log(`     ${l.trim()}`))

// 取消 → 不应清空
await page.locator('.el-message-box__btns button').last().click() // 取消
await page.waitForTimeout(1200)
const afterCancel = await page.locator('.conv-item').count()
console.log(`\n4) 点「取消」后会话数: ${afterCancel}  ${afterCancel === convBefore ? '✓ 未清空' : '✗ 被误清'}`)

// 确认清空
await btn.click()
await page.waitForTimeout(900)
await page.locator('.el-message-box__btns button').first().click() // 确认
await page.waitForTimeout(2600)

const afterClear = await page.locator('.conv-item').count()
const msg = await page.locator('.el-message').allInnerTexts().catch(() => [])
console.log(`\n5) 确认清空后:`)
console.log(`   POST 请求次数 : ${bulkCalls}  ${bulkCalls === 1 ? '✓ 只发一次' : '✗ 重复'}`)
console.log(`   会话数       : ${afterClear}  ${afterClear === 0 ? '✓ 已清空' : '✗'}`)
console.log(`   提示         : ${msg.filter(Boolean).join(' | ')}`)

const emptyText = await page.locator('.el-empty__description, .list-empty').allInnerTexts().catch(() => [])
console.log(`   空状态       : ${emptyText.filter(Boolean).join(' ') || '(无)'}`)

// 数据仍在（归档不删）
const stillInDB = await page.evaluate(async () => {
  const token = localStorage.getItem('kefu_token')
  const r = await fetch('/api/conversations?status=all&include_archived=1&page_size=100', {
    headers: { Authorization: 'Bearer ' + token },
  }).then((x) => x.json())
  return r.data.total
})
console.log(`\n6) 数据库中会话总数(含归档): ${stillInDB}  ${stillInDB > 0 ? '✓ 数据未丢失' : '✗ 数据被删了'}`)

await browser.close()
console.log('\nCLEAR ALL DONE')