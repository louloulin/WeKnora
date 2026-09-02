/**
 * wk-slide-master-view.mjs — v0.7.113 PPT 母版视图 (genoffice vendor)
 *
 * Flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide id> (fresh doc f12a724e-...)
 *   3. click "📐 母版" toolbar button -> modal opens
 *   4. verify list has at least 1 master + at least 1 layout
 *   5. click the first master item -> right pane shows path + element summary
 *   6. click the second item (a layout) -> right pane updates
 *   7. type new name, click "重命名" -> saveLabel flips to 已保存
 *   8. close modal
 *   9. download .pptx, unzip, verify <p:cSld name=...> reflects the new name
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
  writeFileSync('/tmp/wk-master-dump.zip', Buffer.from(buf))
  const out = {}
  for (const path of execSync('unzip -Z1 /tmp/wk-master-dump.zip', { encoding: 'utf8' }).split('\n').filter(Boolean)) {
    try {
      const data = execSync(`unzip -p /tmp/wk-master-dump.zip "${path}"`, { encoding: 'buffer' })
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

  // 1. open master modal
  await page.waitForSelector('[data-testid="slide-master-btn"]', { timeout: 10000 })
  await page.locator('[data-testid="slide-master-btn"]').click()
  await page.waitForSelector('[data-testid="slide-master-modal"]', { timeout: 5000 })
  console.log('master modal opened')

  // 2. verify list has master + layouts
  const items = await page.locator('[data-testid^="slide-master-item-"]').count()
  console.log('list items:', items)
  if (items < 2) throw new Error('expected at least 1 master + 1 layout, got ' + items)

  // 3. select first master
  await page.locator('[data-testid="slide-master-item-0"]').click()
  await page.waitForTimeout(400)
  const path0 = await page.locator('[data-testid="slide-master-item-0"] .collab-slide-konva__master-path').textContent()
  console.log('master #0 path:', path0)
  if (!/slideMaster\d+\.xml/.test(path0 || '')) throw new Error('master #0 is not a slideMaster part')

  // 4. select first layout
  await page.locator('[data-testid="slide-master-item-1"]').click()
  await page.waitForTimeout(400)
  const path1 = await page.locator('[data-testid="slide-master-item-1"] .collab-slide-konva__master-path').textContent()
  console.log('layout #1 path:', path1)
  if (!/slideLayout\d+\.xml/.test(path1 || '')) throw new Error('master #1 is not a slideLayout part')

  // 5. select back the master and rename it
  await page.locator('[data-testid="slide-master-item-0"]').click()
  await page.waitForTimeout(300)
  const nameInput = page.locator('[data-testid="slide-master-name-input"]')
  await nameInput.fill('E2E Test Master')
  await page.waitForTimeout(200)
  const renameBtn = page.locator('[data-testid="slide-master-rename-btn"]')
  if (await renameBtn.isDisabled()) throw new Error('rename button should be enabled after typing')
  await renameBtn.click()
  await page.waitForTimeout(400)

  // 6. wait for saveLabel 已保存
  const saw = await page.waitForFunction(() => document.body.innerText.includes('已保存'), { timeout: 30000 }).then(() => true).catch(() => false)
  console.log('saveLabel 已保存:', saw)
  if (!saw) throw new Error('saveLabel never reached 已保存')
  await page.waitForTimeout(1500)

  // 7. close modal
  await page.locator('[data-testid="slide-master-close-btn"]').click()
  await page.waitForTimeout(400)

  // 8. verify in .pptx
  const bytes = await downloadArchive()
  const entries = await extractArchive(bytes)
  const masterPath = 'ppt/slideMasters/slideMaster1.xml'
  if (!entries[masterPath]) throw new Error('slideMaster1.xml missing in pptx')
  const xml = entries[masterPath]
  console.log('slideMaster1.xml length:', xml.length)
  if (!/name="E2E Test Master"/.test(xml)) throw new Error('renamed master name not in pptx')

  if (errs.length) console.log('PAGE ERRORS:', errs)
  console.log('ALL OK — slide master view: list + select + rename + persist')
  await browser.close()
}

main().catch((e) => { console.error('FAIL:', e.message || e); process.exit(1) })
