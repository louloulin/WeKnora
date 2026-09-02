/**
 * wk-sheet-names-ranges.mjs — v0.7.102 SHEET 命名区域 (workbook Defined Names) verification.
 *
 * Flow:
 *   1. real admin login
 *   2. open /collab-documents/<sheet id>
 *   3. click "命名" -> open modal
 *   4. add workbook-scoped "Revenue" -> Sheet1!$A$1:$D$10
 *   5. add sheet-scoped "LocalTax" on Sheet1 -> Sheet1!$B$2:$B$5
 *   6. delete "Revenue" by index
 *   7. wait for save, download xlsx, unzip xl/workbook.xml
 *   8. verify <definedNames> contains LocalTax (workbook-scoped removed, sheet-scoped remains)
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

function readDefinedNames(workbookXml) {
  const section = /<definedNames\b[^>]*>([\s\S]*?)<\/definedNames>|<definedNames\b[^>]*\/>/.exec(workbookXml)
  if (!section) return []
  const inner = section[1] ?? ''
  const out = []
  const re = /<definedName\b([^>]*)\/?>([\s\S]*?)<\/definedName>|<definedName\b([^>]*)\/>/g
  let m
  while ((m = re.exec(inner))) {
    const attrs = m[1] ?? m[3] ?? ''
    const formula = (m[2] ?? '').trim()
    const nm = /\bname="([^"]*)"/.exec(attrs)
    if (!nm) continue
    const name = nm[1].replace(/&quot;/g, '"').replace(/&amp;/g, '&')
    if (name.startsWith('_xlnm')) continue
    const hidden = /\bhidden="(?:1|true)"/.test(attrs)
    if (hidden) continue
    const sm = /\blocalSheetId="(-?\d+)"/.exec(attrs)
    out.push({ name, formula, sheetIndex: sm ? Number(sm[1]) : undefined })
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

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // baseline: ensure doc has at least one sheet named "Sheet1"
  const baseBuf = await downloadArchive()
  const baseEntries = await extractArchive(baseBuf)
  const baseWb = baseEntries['xl/workbook.xml']?.toString('utf8')
  const baseNames = baseWb ? readDefinedNames(baseWb) : []
  console.log('baseline defined names:', baseNames)

  // open the "命名" modal
  await page.locator('[data-testid="sheet-names-btn"]').click({ force: true })
  await page.waitForTimeout(500)
  const modalTitle = await page.locator('h3:has-text("命名区域")').first().isVisible().catch(() => false)
  console.log('modal opened:', modalTitle)
  // v0.7.102 — clean any leftover entries from prior runs so assertions are deterministic.
  let safety = baseNames.length + 5
  while (safety-- > 0) {
    const cnt = await page.locator('[data-testid^="sheet-names-del-"]').count()
    if (cnt === 0) break
    await page.locator('[data-testid^="sheet-names-del-"]').first().click({ force: true })
    await page.waitForTimeout(250)
  }
  await page.waitForTimeout(2500) // let cleanup save flush

  // 1. add workbook-scoped Revenue
  await page.locator('[data-testid="sheet-names-input-name"]').fill('RevA')
  await page.locator('[data-testid="sheet-names-input-formula"]').fill('Sheet1!$A$1:$D$10')
  await page.locator('[data-testid="sheet-names-input-scope"]').selectOption({ value: '-1' })
  await page.locator('[data-testid="sheet-names-add-btn"]').click({ force: true })
  await page.waitForTimeout(400)

  // 2. add sheet-scoped LocalTax on Sheet1 (index 0)
  await page.locator('[data-testid="sheet-names-input-name"]').fill('TaxA')
  await page.locator('[data-testid="sheet-names-input-formula"]').fill('Sheet1!$B$2:$B$5')
  await page.locator('[data-testid="sheet-names-input-scope"]').selectOption({ value: '0' })
  await page.locator('[data-testid="sheet-names-add-btn"]').click({ force: true })
  await page.waitForTimeout(400)

  // 3. delete Revenue (index 0 in the rendered list)
  await page.locator('[data-testid="sheet-names-del-0"]').click({ force: true })
  await page.waitForTimeout(5000) // wait for save debounce

  // 4. download and verify
  const afterBuf = await downloadArchive()
  const afterEntries = await extractArchive(afterBuf)
  const afterWb = afterEntries['xl/workbook.xml']?.toString('utf8')
  if (!afterWb) throw new Error('no workbook.xml in download')
  const afterNames = readDefinedNames(afterWb)
  console.log('after defined names:', afterNames)

  const hasTaxA = afterNames.some((n) => n.name === 'TaxA' && n.formula === 'Sheet1!$B$2:$B$5' && n.sheetIndex === 0)
  const noRevA = !afterNames.some((n) => n.name === 'RevA')
  console.log('LocalTax workbook-scoped? hasTaxA:', hasTaxA)
  console.log('Revenue removed? noRevA:', noRevA)

  // close modal
  await page.locator('.collab-sheet-editor__modal-bg').first().click({ position: { x: 5, y: 5 } }).catch(() => {})
  await page.screenshot({ path: '/tmp/wk-shots/sheet-names-ranges.png', fullPage: false })
  await browser.close()

  const ok = modalTitle && hasTaxA && noRevA && errs.length === 0
  console.log('---')
  console.log('page errors:', errs.length)
  for (const e of errs) console.log(' -', e.slice(0, 200))
  console.log(ok ? 'ALL OK — sheet named ranges' : 'FAIL')
  process.exit(ok ? 0 : 1)
}

main().catch((e) => { console.error('ERROR', e); process.exit(1) })
