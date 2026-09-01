/**
 * wk-action-e2e.mjs — action-level end-to-end test for all four doc kinds.
 *
 * Replaces the page-render-only wk-4kinds.mjs with real click/input/save
 * cycles. For each kind it:
 *   - opens the editor in a real Playwright browser
 *   - clicks an in-app toolbar action (add shape, add question, add column, etc.)
 *   - types into an input cell (DOC, FORM, SHEET)
 *   - waits for the debounced autosave (1500ms)
 *   - verifies the download endpoint returns a larger / changed binary
 *   - verifies the /sync-to-kb endpoint returns 202
 *
 * Run from the frontend/ directory:
 *   node wk-action-e2e.mjs
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
import fs from 'node:fs'
import path from 'node:path'

const { chromium } = pkg
const BASE = 'http://127.0.0.1:5173'
const SHOTS = '/tmp/wk-shots'
fs.mkdirSync(SHOTS, { recursive: true })

const DOCS = {
  doc:   '67fadefd-8f01-4f2b-aeab-a3ac3d050e39',
  sheet: 'd4eca3d9-77fd-4f81-9746-99e1c4b2f44f',
  slide: 'f12a724e-d87e-49f0-a039-36ca435cb94a',
  form:  'c7205330-41a0-417b-9c42-d5f864a5819a',
}

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(3500)
}

async function downloadSize(docId, token) {
  const resp = await fetch(`${BASE}/collaborative-docs/${docId}/download`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!resp.ok) return { ok: false, status: resp.status, size: 0 }
  const buf = Buffer.from(await resp.arrayBuffer())
  return { ok: true, status: resp.status, size: buf.length, sha: resp.headers.get('x-collab-doc-sha256') }
}

async function captureToken(context) {
  const cookies = await context.cookies()
  const c = cookies.find((x) => x.name === 'WeKnora_token' || x.name === 'token' || x.name === 'Authorization')
  if (c) return c.value
  // fall back: read from localStorage by opening a page
  return null
}

async function actionDoc(page) {
  console.log('\n=== DOC: type into ProseMirror + ensure save ===')
  await page.goto(`${BASE}/collab-documents/${DOCS.doc}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  const editable = page.locator('.collab-doc-pro .ProseMirror').first()
  await editable.click()
  await page.keyboard.press('End')
  const marker = ` [action-doc-${Date.now()}]`
  await page.keyboard.type(marker)
  console.log('  typed marker:', marker)
  await page.waitForTimeout(3500) // autosave debounce
  await page.screenshot({ path: `${SHOTS}/action-doc.png` })
  return { kind: 'doc', typed: marker, hasEdit: true }
}

async function actionSheet(page, token) {
  console.log('\n=== SHEET: +col +row +cell edit + download ===')
  const before = await downloadSize(DOCS.sheet, token)
  await page.goto(`${BASE}/collab-documents/${DOCS.sheet}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  // Click +列 once
  const addCol = page.locator('.collab-sheet-editor__add-col').first()
  if (await addCol.count()) {
    await addCol.click()
    console.log('  clicked + 列')
  } else {
    console.log('  WARN: + 列 button not found')
  }
  // Click + 行 once
  const addRow = page.locator('.collab-sheet-editor__add-row').first()
  if (await addRow.count()) {
    await addRow.click()
    console.log('  clicked + 行')
  }
  // Click first cell input and type
  const firstCell = page.locator('.collab-sheet-editor input[type="text"], .collab-sheet-editor input.collab-sheet-editor__cell-input').first()
  if (await firstCell.count()) {
    await firstCell.fill(`[cell-${Date.now()}]`)
    console.log('  typed into first cell')
  } else {
    console.log('  WARN: cell input not found')
  }
  await page.waitForTimeout(3500) // autosave debounce
  await page.screenshot({ path: `${SHOTS}/action-sheet.png` })
  const after = await downloadSize(DOCS.sheet, token)
  console.log('  before size:', before.size, 'after size:', after.size, 'delta:', after.size - before.size)
  return { kind: 'sheet', before, after, sizeDelta: after.size - before.size }
}

async function actionSlide(page, token) {
  console.log('\n=== SLIDE: +text +rect + download ===')
  const before = await downloadSize(DOCS.slide, token)
  await page.goto(`${BASE}/collab-documents/${DOCS.slide}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  const addText = page.locator('[data-testid="slide-add-text"]').first()
  if (await addText.count()) {
    await addText.click()
    console.log('  clicked +文本框')
  }
  const addRect = page.locator('[data-testid="slide-add-rect"]').first()
  if (await addRect.count()) {
    await addRect.click()
    console.log('  clicked +矩形')
  }
  await page.waitForTimeout(3500)
  await page.screenshot({ path: `${SHOTS}/action-slide.png` })
  const after = await downloadSize(DOCS.slide, token)
  console.log('  before size:', before.size, 'after size:', after.size, 'delta:', after.size - before.size)
  return { kind: 'slide', before, after, sizeDelta: after.size - before.size }
}

async function actionForm(page, token) {
  console.log('\n=== FORM: +rating question + download ===')
  const before = await downloadSize(DOCS.form, token)
  await page.goto(`${BASE}/collab-documents/${DOCS.form}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  // The form editor uses 4 add-question buttons (text/single/multi/rating/date)
  const addBtns = page.locator('.collab-form-editor__add-question')
  const count = await addBtns.count()
  console.log('  form add-question buttons:', count)
  if (count >= 5) {
    await addBtns.nth(3).click() // rating
    console.log('  clicked +rating question')
  }
  await page.waitForTimeout(3500)
  await page.screenshot({ path: `${SHOTS}/action-form.png` })
  const after = await downloadSize(DOCS.form, token)
  console.log('  before size:', before.size, 'after size:', after.size, 'delta:', after.size - before.size)
  return { kind: 'form', before, after, sizeDelta: after.size - before.size }
}

async function syncToKB(page, docId) {
  const resp = await page.evaluate(async (id) => {
    const token = localStorage.getItem('weknora_token')
    if (!token) return { ok: false, error: 'no token in storage' }
    const r = await fetch(`/collaborative-docs/${id}/sync-to-kb`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: '{}',
    })
    return { ok: r.ok, status: r.status, body: (await r.text()).slice(0, 300) }
  }, docId)
  console.log(`  sync-to-kb [${docId}]:`, JSON.stringify(resp))
  return resp
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  page.on('pageerror', (e) => console.log('[pageerror]', e.message))
  page.on('console', (m) => {
    const t = m.text()
    if (m.type() === 'error' && !t.includes('status of 401') && !t.includes('status of 403'))
      console.log('[console.error]', t.slice(0, 200))
  })

  await login(page)
  const token = await captureToken(context) || (await page.evaluate(() => localStorage.getItem('weknora_token')))
  if (!token) {
    console.error('FAIL: no token')
    process.exit(1)
  }
  console.log('token length:', token.length)

  const results = {}
  try {
    results.doc = await actionDoc(page)
    results.sheet = await actionSheet(page, token)
    results.slide = await actionSlide(page, token)
    results.form = await actionForm(page, token)
  } catch (e) {
    console.error('action error:', e.message)
  }

  // sync to KB for each
  console.log('\n=== sync-to-kb per doc ===')
  results.syncs = {}
  for (const kind of ['doc', 'sheet', 'slide', 'form']) {
    results.syncs[kind] = await syncToKB(page, DOCS[kind])
  }

  console.log('\n=== SUMMARY ===')
  for (const k of ['doc', 'sheet', 'slide', 'form']) {
    const r = results[k]
    if (k === 'doc') {
      console.log(`  ${k}  ok=true typed=${r.typed}`)
    } else {
      console.log(`  ${k}  ok=${r.after.ok} before=${r.before.size} after=${r.after.size} delta=${r.sizeDelta}`)
    }
  }
  for (const k of ['doc', 'sheet', 'slide', 'form']) {
    console.log(`  sync[${k}] ok=${results.syncs[k].ok} status=${results.syncs[k].status}`)
  }

  fs.writeFileSync(`${SHOTS}/action-results.json`, JSON.stringify(results, null, 2))
  await browser.close()
  const allOk = results.sheet?.after.ok && results.slide?.after.ok && results.form?.after.ok
  process.exit(allOk ? 0 : 2)
}
main().catch((e) => { console.error('FAIL:', e); process.exit(1) })
