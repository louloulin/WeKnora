/**
 * wk-sheet-sort.mjs — v0.7.100 SHEET sort by column verification.
 *
 * Flow:
 *   1. real admin login
 *   2. open /collab-documents/<sheet id>
 *   3. write test data row 0-3: A col = 3/1/2/4 with B labels delta/alpha/charlie/bravo
 *   4. open sort modal, sort descending by column A from row 1 to row 8
 *   5. expect: first 4 rows A column = 4, 3, 2, 1 and B column = bravo, delta, charlie, alpha
 *      (B follows A — proves row reorder syncs to Yjs correctly)
 *   6. download xlsx, unzip sheet1.xml, verify <c r="A1..A4"> values
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync } from 'node:fs'

const DOC_ID = 'd4eca3d9-77fd-4f81-9746-99e1c4b2f44f'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()

const Buffer = (await import('node:buffer')).Buffer

async function extractArchive(buf) {
  const b = Buffer.from(buf)
  let eocd = -1
  for (let i = b.length - 22; i >= 0 && i >= b.length - 65557; i--) {
    if (b[i] === 0x50 && b[i + 1] === 0x4b && b[i + 2] === 0x05 && b[i + 3] === 0x06) { eocd = i; break }
  }
  if (eocd < 0) throw new Error('EOCD not found')
  const total = b.readUInt16LE(eocd + 10)
  const cdOff = b.readUInt32LE(eocd + 16)
  const out = {}
  let cursor = cdOff
  for (let i = 0; i < total && cursor + 46 <= b.length; i++) {
    const nameLen = b.readUInt16LE(cursor + 28)
    const extraLen = b.readUInt16LE(cursor + 30)
    const commentLen = b.readUInt16LE(cursor + 32)
    const lhOff = b.readUInt32LE(cursor + 42)
    const lhNameLen = b.readUInt16LE(lhOff + 26)
    const lhExtraLen = b.readUInt16LE(lhOff + 28)
    const compMethod = b.readUInt16LE(lhOff + 8)
    const compSize = b.readUInt32LE(lhOff + 18)
    const dataStart = lhOff + 30 + lhNameLen + lhExtraLen
    const name = b.slice(cursor + 46, cursor + 46 + nameLen).toString('utf8')
    cursor = cursor + 46 + nameLen + extraLen + commentLen
    let data
    if (compMethod === 0) data = b.slice(dataStart, dataStart + compSize)
    else if (compMethod === 8) {
      const zlib = await import('node:zlib')
      data = zlib.inflateRawSync(b.slice(dataStart, dataStart + compSize))
    }
    else continue
    out[name] = data
  }
  return out
}

async function downloadArchive() {
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, { headers: { Authorization: `Bearer ${TOKEN}` } })
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

async function setCellText(page, rowIdx, colIdx, text) {
  const input = page.locator(`input[data-cell="${rowIdx}-${colIdx}"]`).first()
  await input.click({ force: true })
  await input.fill(text)
  await page.keyboard.press('Tab')
  await page.waitForTimeout(200)
}

async function readColumnFromXlsx(entries, col) {
  const sheetPath = Object.keys(entries).find((p) => /^xl\/worksheets\/sheet\d+\.xml$/.test(p))
  if (!sheetPath) {
    console.log('  (debug) no sheet1.xml found, entries keys:', Object.keys(entries).slice(0, 10))
    return []
  }
  const xml = Buffer.from(entries[sheetPath]).toString('utf8')
  const out = []
  // Use a single non-greedy regex for <c r="A1" ...>...</c>
  const cellRe = new RegExp('<c\\s+r="' + col + '(\\d+)"(?:\\s+s="\\d+")?(?:\\s+t="(s|inlineStr|str|n)")?[^>]*>([\\s\\S]*?)<\\/c>', 'g')
  let m
  while ((m = cellRe.exec(xml))) {
    const row = parseInt(m[1])
    const t = m[2] || 'n'
    const body = m[3]
    let val = ''
    if (t === 's' || t === 'str' || t === 'inlineStr') {
      const vm = /<v[^>]*>([^<]*)<\/v>/.exec(body) || /<t[^>]*>([^<]*)<\/t>/.exec(body)
      if (vm) val = vm[1]
    } else {
      const vm = /<v>([^<]+)<\/v>/.exec(body)
      if (vm) val = vm[1]
    }
    out.push({ row, val })
  }
  return out.sort((a, b) => a.row - b.row).map((x) => x.val)
}

const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
const page = await ctx.newPage()
const errs = []
page.on('pageerror', (e) => errs.push(e.message))

await login(page)
await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(8000)

// Write test data: row 0-3 with A col 3/1/2/4 and B labels in non-sorted order
await setCellText(page, 0, 0, '3')
await setCellText(page, 0, 1, 'delta')
await setCellText(page, 1, 0, '1')
await setCellText(page, 1, 1, 'alpha')
await setCellText(page, 2, 0, '2')
await setCellText(page, 2, 1, 'charlie')
await setCellText(page, 3, 0, '4')
await setCellText(page, 3, 1, 'bravo')
await page.waitForTimeout(3000)

// Open sort modal, descending by column A, rows 1-8
await page.locator('[data-testid="sheet-sort-btn"]').click({ force: true })
await page.waitForTimeout(500)
await page.locator('[data-testid="sheet-sort-col"]').fill('A')
await page.locator('[data-testid="sheet-sort-direction"]').selectOption('desc')
await page.locator('[data-testid="sheet-sort-start-row"]').fill('1')
await page.locator('[data-testid="sheet-sort-end-row"]').fill('8')
await page.waitForTimeout(300)
await page.locator('[data-testid="sheet-sort-apply-btn"]').click({ force: true })
await page.waitForTimeout(3500)

// Verify UI state
const afterDescA = []
const afterDescB = []
for (let r = 0; r < 8; r++) {
  const a = page.locator(`input[data-cell="${r}-0"]`).first()
  const b = page.locator(`input[data-cell="${r}-1"]`).first()
  afterDescA.push((await a.count()) > 0 ? await a.inputValue() : '')
  afterDescB.push((await b.count()) > 0 ? await b.inputValue() : '')
}
console.log('after desc sort row 0-7:')
for (let r = 0; r < 8; r++) console.log(`  row ${r}: A=${afterDescA[r]} B=${afterDescB[r]}`)
await page.screenshot({ path: '/tmp/wk-shots/sheet-sort-desc.png', fullPage: false })

// Verify xlsx
const buf = await downloadArchive()
const entries = await extractArchive(buf)
const xlsxA = await readColumnFromXlsx(entries, 'A')
const xlsxB = await readColumnFromXlsx(entries, 'B')
console.log('xlsx A first 8:', xlsxA.slice(0, 8))
console.log('xlsx B first 8:', xlsxB.slice(0, 8))

await browser.close()

// Asserts:
//   - UI: row 0-3 A column = [4, 3, 2, 1], B column = [bravo, delta, charlie, alpha]
//   - xlsx: same first 4 rows
//   - desc order is non-increasing throughout first 8 rows
let descSorted = true
for (let i = 1; i < afterDescA.length; i++) {
  const a = afterDescA[i]
  const p = afterDescA[i - 1]
  if (a !== '' && p !== '' && Number(a) > Number(p)) { descSorted = false; break }
}
const expectUI_A = afterDescA.slice(0, 4).join(',') === '4,3,2,1'
const expectUI_B = afterDescB.slice(0, 4).join(',') === 'bravo,delta,charlie,alpha'
const expectXlsxA = xlsxA.slice(0, 4).join(',') === '4,3,2,1'
const expectXlsxB = xlsxB.slice(0, 4).join(',') === 'bravo,delta,charlie,alpha'
console.log('---')
console.log('descSorted:', descSorted)
console.log('expectUI_A:', expectUI_A, '(', afterDescA.slice(0, 4).join(','), ')')
console.log('expectUI_B:', expectUI_B, '(', afterDescB.slice(0, 4).join(','), ')')
console.log('expectXlsxA:', expectXlsxA, '(', xlsxA.slice(0, 4).join(','), ')')
console.log('expectXlsxB:', expectXlsxB, '(', xlsxB.slice(0, 4).join(','), ')')
console.log('page errors:', errs.length)
for (const e of errs) console.log(' -', e.slice(0, 200))

const ok = descSorted && expectUI_A && expectUI_B && expectXlsxA && expectXlsxB && errs.length === 0
console.log(ok ? 'ALL OK — SHEET sort' : 'FAIL')
process.exit(ok ? 0 : 2)
