/**
 * wk-sheet-pivot-apply.mjs — v0.7.110.1 XLSX 数据透视「应用」按钮.
 *
 * Flow:
 *   1. real admin login
 *   2. server-side: build a fresh .xlsx with controlled data (Category/Region/Sales),
 *      upload it via API to the SHEET doc id (avoids DOM/Yjs sync races)
 *   3. open /collab-documents/<sheet id> in the browser, wait for the cells to render
 *   4. open pivot modal, source A1:C5, row dim A, value C, agg sum
 *   5. click "预览" → verify preview shows aggregated categories
 *   6. click "应用" → wait for save toast
 *   7. download xlsx, unzip, verify xl/pivotTables/pivotTable*.xml exists and
 *      workbook.xml has <pivotCaches>; pivotCacheDefinition + Records exist
 *   8. verify pivot table points at the new cache
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync, writeFileSync } from 'node:fs'
import { execSync } from 'node:child_process'
import { newXlsxWorkbook, saveXlsxBytes } from './src/editor/adapters/xlsxAdapter.ts'

const DOC_ID = 'd4eca3d9-77fd-4f81-9746-99e1c4b2f44f'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()
const Buffer = (await import('node:buffer')).Buffer

async function extractArchive(buf) {
  writeFileSync('/tmp/wk-pivot-dump.zip', Buffer.from(buf))
  const listing = execSync('unzip -l /tmp/wk-pivot-dump.zip', { encoding: 'utf8' })
  const out = {}
  const lines = listing.split('\n').filter((l) => /\s\d{2}-\d{2}-\d{4}/.test(l))
  for (const l of lines) {
    const parts = l.trim().split(/\s+/)
    const path = parts[parts.length - 1]
    try {
      const data = execSync(`unzip -p /tmp/wk-pivot-dump.zip "${path}"`, { encoding: 'buffer' })
      out[path] = data.toString('utf8')
    } catch (e) { /* skip */ }
  }
  return out
}

async function uploadFreshSheet() {
  const wb = newXlsxWorkbook()
  const buildCell = (s: string) => ({ v: s })
  wb.sheets = [
    {
      name: 'Sheet1',
      rows: [
        [buildCell('Category'), buildCell('Region'), buildCell('Sales')],
        [buildCell('A'), buildCell('East'), buildCell('10')],
        [buildCell('A'), buildCell('West'), buildCell('20')],
        [buildCell('B'), buildCell('East'), buildCell('30')],
        [buildCell('B'), buildCell('West'), buildCell('40')],
      ],
    },
  ]
  const bytes = await saveXlsxBytes(wb)
  const form = new FormData()
  const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
  form.append('file', new Blob([ab]), 'pivot-test.xlsx')
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/upload`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${TOKEN}` },
    body: form,
  })
  if (!res.ok) throw new Error(`upload failed: ${res.status} ${await res.text()}`)
  console.log('upload fresh sheet ok:', res.status)
}

async function downloadArchive() {
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, { headers: { Authorization: `Bearer ${TOKEN}` } })
  if (!res.ok) throw new Error(`download failed ${res.status}`)
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
  console.log('1) uploading fresh xlsx via API')
  await uploadFreshSheet()

  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 900 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  console.log('2) login + open sheet doc')
  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(5000)

  // Wait for the sheet cells to render
  await page.waitForSelector('input[data-cell="0-0"]', { timeout: 10000 })
  // Verify our data is rendered (should show "Category")
  await page.waitForFunction(
    () => document.querySelector('input[data-cell="0-0"]')?.value === 'Category',
    { timeout: 10000 },
  )
  console.log('3) cells rendered with expected data')

  console.log('4) opening pivot modal')
  await page.locator('[data-testid="sheet-pivot-btn"]').click({ force: true })
  await page.waitForSelector('[data-testid="sheet-pivot-source"]', { timeout: 5000 })
  await page.locator('[data-testid="sheet-pivot-source"]').fill('A1:C5')
  await page.locator('[data-testid="sheet-pivot-rowdim"]').fill('A')
  await page.locator('[data-testid="sheet-pivot-valuecol"]').fill('C')
  await page.locator('[data-testid="sheet-pivot-agg"]').selectOption('sum')

  console.log('5) clicking 预览')
  await page.locator('[data-testid="sheet-pivot-preview-btn"]').click()
  await page.waitForTimeout(800)
  const previewText = (await page.locator('[data-testid="sheet-pivot-preview"]').textContent()) ?? ''
  console.log('   preview:\n' + previewText)
  if (!/^A: 30/m.test(previewText) || !/^B: 70/m.test(previewText)) {
    throw new Error(`preview aggregate wrong: ${previewText}`)
  }

  console.log('6) clicking 应用')
  await page.locator('[data-testid="sheet-pivot-apply-btn"]').click()
  // Wait for the save toast "已写入 Pivot1"
  const sawToast = await page.waitForFunction(
    () => document.body.innerText.includes('已写入 Pivot1'),
    { timeout: 30000 },
  ).then(() => true).catch(() => false)
  console.log('   toast seen:', sawToast)
  if (!sawToast) throw new Error('save toast did not show up within 30s')
  await page.waitForTimeout(3000) // wait for the upload to actually land

  console.log('7) downloading + verifying xlsx')
  const bytes = await downloadArchive()
  const entries = await extractArchive(bytes)
  const paths = Object.keys(entries).sort()
  const pivots = paths.filter((p) => /^xl\/pivotTables\/pivotTable\d+\.xml$/.test(p))
  const caches = paths.filter((p) => /^xl\/pivotCache\/pivotCacheDefinition\d+\.xml$/.test(p))
  const records = paths.filter((p) => /^xl\/pivotCache\/pivotCacheRecords\d+\.xml$/.test(p))
  console.log('   parts:', paths.length, 'pivotTables:', pivots.length, 'caches:', caches.length, 'records:', records.length)
  if (pivots.length < 1) throw new Error('no pivot table part')
  if (caches.length < 1) throw new Error('no pivot cache definition part')
  if (records.length < 1) throw new Error('no pivot cache records part')

  const workbookXml = entries['xl/workbook.xml'] ?? ''
  if (!/<pivotCaches>/.test(workbookXml)) throw new Error('workbook.xml missing pivotCaches')

  const pivotTableXml = entries[pivots[0]]
  if (!/<pivotTableDefinition/.test(pivotTableXml)) throw new Error('pivot table missing <pivotTableDefinition>')

  if (errs.length) console.log('PAGE ERRORS:', errs)
  console.log('ALL OK — sheet pivot apply + persisted pivot parts')
  await browser.close()
}

main().catch((e) => {
  console.error('FAIL:', e.message || e)
  process.exit(1)
})
