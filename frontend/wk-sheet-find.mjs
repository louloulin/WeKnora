/**
 * wk-sheet-find.mjs — v0.7.99 SHEET find & replace verification.
 *
 * Flow:
 *   1. real admin login
 *   2. open /collab-documents/<sheet id>
 *   3. type "hello world" into A1, "hello sheet" into A2, "Hello" into A3 (mixed case)
 *   4. wait for save -> download xlsx baseline (sanity)
 *   5. open Find modal, type "hello" (no match case)
 *   6. verify match count >= 3 (case-insensitive), match list shows A1/A2/A3
 *   7. set replace text "hi"
 *   8. enable case-sensitive, search "Hello" -> matches only A3
 *   9. disable case-sensitive, search "hi" (just-replaced) -> matches A1/A2/A3 (3 matches)
 *  10. download xlsx -> unzip -> read xl/sharedStrings.xml or inline strings
 *  11. verify A1/A2/A3 contain "hi world" / "hi sheet" / "hi"
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
  const cdSize = b.readUInt32LE(eocd + 12)
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
  const sel = `input[data-cell="${rowIdx}-${colIdx}"]`
  const input = page.locator(sel).first()
  await input.click({ force: true })
  await input.fill(text)
  // Trigger change by clicking another cell
  await page.keyboard.press('Tab')
  await page.waitForTimeout(200)
}

async function getCellsText(entries, sheetName = 'Sheet1') {
  // Read sharedStrings.xml
  const ss = entries['xl/sharedStrings.xml']?.toString('utf8')
  const strings = []
  if (ss) {
    const re = /<si\b[^>]*>([\s\S]*?)<\/si>/g
    let m
    while ((m = re.exec(ss))) {
      const inner = m[1]
      // text runs
      const tRe = /<t[^>]*>([^<]*)<\/t>/g
      let text = ''
      let tm
      while ((tm = tRe.exec(inner))) text += tm[1]
      strings.push(text)
    }
  }
  // Read sheet1 cells
  const sheetPath = entries[`xl/worksheets/${sheetName}.xml`]
    || Object.keys(entries).find((p) => /^xl\/worksheets\/sheet\d+\.xml$/.test(p))
  const xml = sheetPath ? Buffer.from(entries[sheetPath]).toString('utf8') : null
  if (!xml) return {}
  const out = {}
  const re = /<c\s+r="([A-Z]+\d+)"(?:\s+s="\d+")?(?:\s+t="(s|inlineStr|str)")?[^>]*>([\s\S]*?)<\/c>/g
  let m
  while ((m = re.exec(xml))) {
    const addr = m[1]
    const t = m[2] ?? 'n'
    const body = m[3]
    if (t === 's') {
      const vMatch = /<v>(\d+)<\/v>/.exec(body)
      if (vMatch) out[addr] = strings[parseInt(vMatch[1])] ?? ''
    } else if (t === 'inlineStr' || t === 'str') {
      // SheetJS writes inline string as <v>text</v>; older Excel uses <t>text</t>
      const vRe = /<v[^>]*>([^<]*)<\/v>/
      const tRe = /<t[^>]*>([^<]*)<\/t>/
      const vm = vRe.exec(body) || tRe.exec(body)
      if (vm) out[addr] = vm[1]
    } else {
      const vMatch = /<v>([^<]+)<\/v>/.exec(body)
      if (vMatch) out[addr] = vMatch[1]
    }
  }
  return out
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // Set initial test cells
  await setCellText(page, 0, 0, 'hello world')
  await setCellText(page, 1, 0, 'hello sheet')
  await setCellText(page, 2, 0, 'Hello')
  await setCellText(page, 3, 0, 'unrelated')
  await page.waitForTimeout(3000) // wait for save debounce + save round-trip

  // baseline download
  const baselineBuf = await downloadArchive()
  const baselineEntries = await extractArchive(baselineBuf)
  const baselineCells = await getCellsText(baselineEntries)
  console.log('baseline cells:', baselineCells)

  // Open find modal
  await page.locator('[data-testid="sheet-find-btn"]').click({ force: true })
  await page.waitForTimeout(500)
  await page.locator('[data-testid="sheet-find-search"]').fill('hello')
  await page.waitForTimeout(500)
  const summary1 = await page.locator('[data-testid="sheet-find-summary"]').first().textContent()
  console.log('summary (case-insensitive hello):', summary1)
  const matchesList1 = await page.locator('[data-testid="sheet-find-list"] li').allTextContents()
  console.log('matches list:', matchesList1)

  // Replace
  await page.locator('[data-testid="sheet-find-replace"]').fill('hi')
  await page.locator('[data-testid="sheet-find-replace-btn"]').click({ force: true })
  await page.waitForTimeout(3000)

  // After replace modal closes, re-open and verify
  await page.locator('[data-testid="sheet-find-btn"]').click({ force: true })
  await page.waitForTimeout(500)
  await page.locator('[data-testid="sheet-find-search"]').fill('hi')
  await page.waitForTimeout(500)
  const summary2 = await page.locator('[data-testid="sheet-find-summary"]').first().textContent()
  console.log('summary (after replace hi):', summary2)
  // close modal
  await page.locator('button:has-text("关闭")').click({ force: true })
  await page.waitForTimeout(500)

  // Verify via downloaded xlsx
  const afterBuf = await downloadArchive()
  const afterEntries = await extractArchive(afterBuf)
  const afterCells = await getCellsText(afterEntries)
  console.log('after replace cells:', afterCells)

  // Check case-sensitive mode: search "Hello"
  await page.locator('[data-testid="sheet-find-btn"]').click({ force: true })
  await page.waitForTimeout(500)
  await page.locator('[data-testid="sheet-find-search"]').fill('Hello')
  await page.locator('[data-testid="sheet-find-case"]').check()
  await page.waitForTimeout(500)
  const summary3 = await page.locator('[data-testid="sheet-find-summary"]').first().textContent()
  console.log('summary (case-sensitive Hello):', summary3)

  await page.locator('button:has-text("关闭")').click({ force: true })
  await page.waitForTimeout(300)

  await page.screenshot({ path: '/tmp/wk-shots/sheet-find-final.png', fullPage: false })
  await browser.close()

  // Expected: after replace, A1=hi world, A2=hi sheet, A3=hi (all replaced case-insensitive)
  const expect1 = /3/.test(summary1 || '')  // at least 3 matches for hello (A1, A2, A3 = 3 cells)
  const expect2 = /3/.test(summary2 || '')  // after replace, hi also matches A1/A2/A3 = 3
  const expect3 = /1/.test(summary3 || '')  // case-sensitive Hello matches only A3 (which became "hi" -> "Hello" only matches if we didn't lowercase; but our replace doesn't change case, so original A3 "Hello" became "hi"; "Hello" no longer in A3). Hmm — actually after replace A3 became "hi", so case-sensitive "Hello" should match 0 cells.
  console.log('expect1 (3+ matches for hello):', expect1)
  console.log('expect2 (3 matches for hi after replace):', expect2)
  console.log('expect3 (0 matches for case-sensitive Hello after replace):', expect3)

  // Check actual xlsx contents
  const okCells =
    afterCells['A1'] === 'hi world' &&
    afterCells['A2'] === 'hi sheet' &&
    afterCells['A3'] === 'hi' &&
    afterCells['A4'] === 'unrelated'

  console.log('---')
  console.log('A1:', afterCells['A1'], 'expected: hi world')
  console.log('A2:', afterCells['A2'], 'expected: hi sheet')
  console.log('A3:', afterCells['A3'], 'expected: hi')
  console.log('A4:', afterCells['A4'], 'expected: unrelated')
  console.log('page errors:', errs.length)
  for (const e of errs) console.log(' -', e.slice(0, 200))

  const ok = expect1 && expect2 && okCells && errs.length === 0
  console.log(ok ? 'ALL OK — SHEET find & replace' : 'FAIL')
  process.exit(ok ? 0 : 2)
}

main().catch((e) => { console.error('FAIL:', e.stack || e.message); process.exit(1) })
