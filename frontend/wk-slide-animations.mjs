/**
 * wk-slide-animations.mjs — v0.7.112 SLIDE 动画面板细粒度编辑 + E2E 验证.
 *
 * Flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide id> (fresh doc f12a724e-...)
 *   3. add a rectangle shape on slide 1
 *   4. click the rect to select it
 *   5. add 2 animations with default (fade / onClick / 1000ms / 0ms)
 *   6. patch effect #0 -> flyIn
 *   7. patch duration #0 -> 2500ms
 *   8. patch delay #1 -> 800ms
 *   9. reorder: move item 0 down (1)
 *  10. wait saveLabel -> 已保存
 *  11. download pptx, parse ppt/slides/slide1.xml, find <p:timing>; verify
 *      two presetIDs (10=fade, 2=flyIn), durations 2500/1000 in dur="N".
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync, writeFileSync } from 'node:fs'
import { execSync } from 'node:child_process'

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()
const Buffer = (await import('node:buffer')).Buffer

async function extractArchive(buf) {
  writeFileSync('/tmp/wk-anim-dump.zip', Buffer.from(buf))
  const out = {}
  for (const path of execSync('unzip -Z1 /tmp/wk-anim-dump.zip', { encoding: 'utf8' }).split('\n').filter(Boolean)) {
    try {
      const data = execSync(`unzip -p /tmp/wk-anim-dump.zip "${path}"`, { encoding: 'buffer' })
      out[path] = data.toString('utf8')
    } catch { /* skip */ }
  }
  return out
}

async function downloadArchive() {
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, { headers: { Authorization: `Bearer ${TOKEN}` } })
  if (!res.ok) throw new Error('download failed ' + res.status)
  return new Uint8Array(await res.arrayBuffer())
}

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(3500)
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 900 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(6000)

  // 1. add a rectangle shape on slide 1
  await page.waitForSelector('[data-testid="slide-add-rect"]', { timeout: 10000 })
  await page.locator('[data-testid="slide-add-rect"]').click()
  await page.waitForTimeout(800)

  // 2. click the rectangle to select it
  const cr = await page.evaluate(() => document.querySelector('canvas')?.getBoundingClientRect() ?? null)
  if (!cr) throw new Error('canvas not found')
  await page.mouse.click(cr.x + cr.width / 2, cr.y + cr.height / 2)
  await page.waitForTimeout(500)

  // 3. clear leftover animations and add 2 fresh.
  await page.waitForSelector('[data-testid="slide-anim-clear-btn"]', { timeout: 5000 })
  await page.locator('[data-testid="slide-anim-clear-btn"]').click({ force: true })
  await page.waitForTimeout(400)
  await page.waitForSelector('[data-testid="slide-anim-add-btn"]', { timeout: 5000 })
  await page.locator('[data-testid="slide-anim-add-btn"]').click()
  await page.waitForTimeout(300)
  await page.locator('[data-testid="slide-anim-add-btn"]').click()
  await page.waitForTimeout(300)
  const countA = await page.locator('[data-testid^="slide-anim-item-"]').count()
  console.log('animations after add:', countA)
  if (countA !== 2) throw new Error('expected 2 animations, got ' + countA)

  // 4. patch effect #0 -> flyIn
  await page.locator('[data-testid="slide-anim-effect-0"]').selectOption('flyIn')
  await page.waitForTimeout(200)

  // 5. patch duration #0 -> 2500
  const dur0 = page.locator('[data-testid="slide-anim-duration-0"]')
  await dur0.fill('2500')
  await dur0.dispatchEvent('change')
  await page.waitForTimeout(200)

  // 6. patch delay #1 -> 800
  const delay1 = page.locator('[data-testid="slide-anim-delay-1"]')
  await delay1.fill('800')
  await delay1.dispatchEvent('change')
  await page.waitForTimeout(200)

  // 7. reorder: move item 0 down
  await page.locator('[data-testid="slide-anim-down-0"]').click()
  await page.waitForTimeout(300)

  const order = await page.evaluate(() => Array.from(document.querySelectorAll('[data-testid^="slide-anim-effect-"]')).map((el) => el.value))
  console.log('effect order after reorder:', order)
  if (order[0] !== 'fade' || order[1] !== 'flyIn') throw new Error('reorder failed: ' + order.join(','))

  const d0 = await page.locator('[data-testid="slide-anim-duration-0"]').inputValue()
  const d1 = await page.locator('[data-testid="slide-anim-duration-1"]').inputValue()
  console.log('duration row0 (was originally flyIn):', d0, 'row1 (was originally fade):', d1)
  if (d0 !== '1000' || d1 !== '2500') throw new Error('durations swapped incorrectly: row0=' + d0 + ' row1=' + d1)

  // 8. wait for saveLabel "已保存"
  const saw = await page.waitForFunction(() => document.body.innerText.includes('已保存'), { timeout: 30000 }).then(() => true).catch(() => false)
  console.log('saveLabel 已保存:', saw)
  if (!saw) throw new Error('saveLabel never reached 已保存')
  await page.waitForTimeout(2000)

  // 9. download pptx and verify <p:timing>
  const bytes = await downloadArchive()
  const entries = await extractArchive(bytes)
  const slidePath = Object.keys(entries).find((p) => /^ppt\/slides\/slide1\.xml$/.test(p))
  if (!slidePath) throw new Error('slide1.xml not found in pptx')
  const slideXml = entries[slidePath]
  console.log('slide1.xml length:', slideXml.length)
  const timingMatch = slideXml.match(/<p:timing>[\s\S]*?<\/p:timing>/)
  if (!timingMatch) throw new Error('<p:timing> not present in slide1.xml')
  const timingXml = timingMatch[0]
  const presetIds = Array.from(timingXml.matchAll(/presetID="(\d+)"/g)).map((m) => m[1])
  console.log('presetIDs:', presetIds)
  if (!presetIds.includes('10')) throw new Error('expected presetID=10 for fade, not present')
  if (!presetIds.includes('2')) throw new Error('expected presetID=2 for flyIn, not present')
  if (presetIds.length < 2) throw new Error('expected at least 2 animEffects, got ' + presetIds.length)
  if (!/dur="2500"/.test(timingXml)) throw new Error('no dur="2500" in <p:timing>')
  if (!/dur="1000"/.test(timingXml)) throw new Error('no dur="1000" (default 1s) in <p:timing>')

  if (errs.length) console.log('PAGE ERRORS:', errs)
  console.log('ALL OK — slide animations: per-row edit + reorder + <p:timing> persisted')
  await browser.close()
}

main().catch((e) => { console.error('FAIL:', e.message || e); process.exit(1) })
