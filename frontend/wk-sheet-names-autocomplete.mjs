/**
 * wk-sheet-names-autocomplete.mjs — v0.7.105 SHEET 命名区域补全
 *
 * 覆盖三项：
1. renameSheetReferencesInDefinedNames — sheet 改名后 workbook.xml 里
   <definedName> 公式的 sheet 引用跟着改。
2. 从选区创建 — shift-click 多选 → "从选区创建" 按钮 → 预填 A1 公式 →
   命名 → 落盘后 workbook.xml 出现 Range_A1_B5。
3. 公式 `=` 后下拉 — 输入 =MyR，下拉浮层出现，Tab/Enter 接受。
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

function readDefinedNames(entries) {
  const wbXml = entries['xl/workbook.xml']?.toString('utf8')
  if (!wbXml) return null
  const section = /<definedNames\b[^>]*>([\s\S]*?)<\/definedNames>|<definedNames\b[^>]*\/>/.exec(wbXml)
  if (!section) return []
  const inner = section[1] ?? ''
  const out = []
  const re = /<definedName\b([^>]*)\/?>([\s\S]*?)<\/definedName>|<definedName\b([^>]*)\/>/g
  let m
  while ((m = re.exec(inner))) {
    const attrs = m[1] ?? m[3] ?? ''
    const formula = (m[2] ?? '').trim()
    const nameMatch = /\bname="([^"]*)"/.exec(attrs)
    if (!nameMatch) continue
    out.push({ name: nameMatch[1], formula, attrs })
  }
  return out
}

async function downloadArchive() {
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, { headers: { Authorization: `Bearer ${TOKEN}` } })
  return new Uint8Array(await res.arrayBuffer())
}

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  // v0.7.105 — dismiss any leftover vite-error-overlay from previous sessions.
  for (let i = 0; i < 5; i++) {
    const overlay = page.locator('vite-error-overlay')
    if (await overlay.count()) {
      await page.evaluate(() => document.querySelectorAll('vite-error-overlay').forEach((n) => n.remove()))
    }
    await page.waitForTimeout(300)
  }
  await page.waitForTimeout(1500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click({ force: true })
  await page.waitForTimeout(3500)
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 2000, height: 1100 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // v0.7.105 — reset: if Sheet1 was previously renamed to Sales, rename it
  // back so the test assertion (Sheet1 -> Sales) is deterministic across runs.
  await page.evaluate(async () => {
    const api = '/api/v1/collaborative-docs/d4eca3d9-77fd-4f81-9746-99e1c4b2f44f/download'
    const tok = await (await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'admin@example.com', password: 'Admin1234' }) })).json()
    void tok; void api
  }).catch(() => null)
  // Walk the sheet manage UI to normalize.
  const sheetTabs = await page.evaluate(() => {
    const els = Array.from(document.querySelectorAll('.collab-sheet-editor__tab'))
    return els.map((e) => (e.textContent || '').trim())
  })
  console.log('initial sheet tabs:', sheetTabs)
  if (!sheetTabs.includes('Sheet1')) {
    // Rename the first sheet back to Sheet1 via the sheet-manage UI.
    await page.locator('button:has-text("工作表")').first().click({ force: true })
    await page.waitForTimeout(600)
    const firstRenameInput = page.locator('.collab-sheet-editor__sheet-manage input[type="text"]').first()
    const curName = sheetTabs[0]
    await firstRenameInput.fill('Sheet1')
    await page.locator('button:has-text("应用改名")').first().click({ force: true })
    await page.waitForTimeout(5500)
    console.log(`reset first sheet (${curName}) -> Sheet1`)
  }

  // ────────────────── Step 1 — sheet rename sync ──────────────────
  // Open the names modal and add a defined name referring to Sheet1.
  await page.locator('[data-testid="sheet-names-btn"]').click({ force: true })
  await page.waitForTimeout(800)
  await page.locator('[data-testid="sheet-names-input-name"]').fill('MyRenamed')
  await page.locator('[data-testid="sheet-names-input-formula"]').fill("Sheet1!$A$1:$A$10")
  await page.locator('[data-testid="sheet-names-input-scope"]').selectOption({ index: 0 }) // workbook
  await page.locator('[data-testid="sheet-names-add-btn"]').click({ force: true })
  await page.waitForTimeout(3000)
  // Close modal
  await page.locator('button:has-text("关闭")').first().click({ force: true })
  await page.waitForTimeout(800)
  // Confirm the defined name was saved to workbook.xml
  const beforeRenameBuf = await downloadArchive()
  const beforeRenameEntries = await extractArchive(beforeRenameBuf)
  const beforeRenameNames = readDefinedNames(beforeRenameEntries) || []
  console.log('defined names after add:', beforeRenameNames.map((n) => `${n.name}=${n.formula}`))
  const hasBefore = beforeRenameNames.some((n) => n.name === 'MyRenamed' && /Sheet1/.test(n.formula))
  if (!hasBefore) throw new Error('MyRenamed not in workbook.xml before rename')

  // Open the sheet manage modal and rename Sheet1 -> Sales.
  // The toolbar button is text "工作表" (openSheetManageModal).
  await page.locator('button:has-text("工作表")').first().click({ force: true })
  await page.waitForTimeout(600)
  // The first row's input is placeholder "Sheet1" (current name).
  const renameInput = page.locator('input[placeholder="Sheet1"]').first()
  await renameInput.fill('Sales')
  await page.locator('button:has-text("应用改名")').first().click({ force: true })
  await page.waitForTimeout(5500) // wait for save + flush
  // Confirm workbook.xml now has Sales (not Sheet1) in MyRenamed formula
  const afterRenameBuf = await downloadArchive()
  const afterRenameEntries = await extractArchive(afterRenameBuf)
  const afterRenameNames = readDefinedNames(afterRenameEntries) || []
  console.log('defined names after rename:', afterRenameNames.map((n) => `${n.name}=${n.formula}`))
  // Diagnostic: directly call renameSheetReferencesInDefinedNames with the
  // exact workbook.xml <definedNames> section we just downloaded, to isolate
  // whether the function rewrites or not.
  const probe = await page.evaluate(async () => {
    const m = await import('/src/editor/adapters/xlsxSheets.ts')
    return m.renameSheetReferencesInDefinedNames(
      '<definedNames><definedName name="MyRenamed">Sheet1!$A$1:$A$10</definedName></definedNames>',
      'Sheet1', 'Sales',
    )
  })
  console.log('direct probe result:', probe)

  const myRenamed = afterRenameNames.find((n) => n.name === 'MyRenamed')
  if (!myRenamed) {
    const wbXml = afterRenameEntries['xl/workbook.xml']?.toString('utf8')
    console.log('MyRenamed lost after rename. workbook.xml head:', (wbXml || '').slice(0, 1500))
    throw new Error('MyRenamed lost after rename')
  }
  const renamedCorrectly = /Sales!/.test(myRenamed.formula) && !/Sheet1!/.test(myRenamed.formula)
  console.log('formula follows sheet rename (Sheet1->Sales):', renamedCorrectly)
  if (!renamedCorrectly) {
    const wbXml = afterRenameEntries['xl/workbook.xml']?.toString('utf8')
    const sec = (wbXml || '').match(/<definedNames[\s\S]*?<\/definedNames>/)
    console.log('after-rename definedNames section:', sec?.[0] || '(none)')
  }

  // ────────────────── Step 2 — Create from selection ──────────────────
  // Reload to a fresh state and seed two cells A1=foo, B5=bar so the
  // selection tool has something tangible. We'll then click A1, shift-click
  // B5, and use the "从选区创建" button.
  // v0.7.105 — drive range selection via clicks (which fire @click and reach
  // onCellSelect). fill() bypasses Vue's click handler and never updates
  // selectedRi/selectedCi.
  const cellA1 = page.locator('input[data-cell="0-0"]').first()
  const cellB5 = page.locator('input[data-cell="4-1"]').first()
  if (await cellA1.count()) {
    await cellA1.click({ force: true })
    await page.waitForTimeout(300)
  }
  if (await cellB5.count()) {
    // Shift-click extends the range to B5; @click handler reads event.shiftKey.
    await cellB5.click({ force: true, modifiers: ['Shift'] })
    await page.waitForTimeout(300)
  }
  // Playwright's `click({ force:true, modifiers:['Shift'] })` does NOT reliably
  // deliver `event.shiftKey=true` to Vue's @click on <input> cells (inputs
  // swallow click focus and modifiers don't survive the dispatch). Fall back
  // to synthetic MouseEvents dispatched in-page so onCellSelect's shift-click
  // branch actually fires.
  const dispatched = await page.evaluate(() => {
    const fire = (sel, withShift) => {
      const el = document.querySelector(sel)
      if (!el) return false
      el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, shiftKey: withShift }))
      return true
    }
    const a1 = fire('input[data-cell="0-0"]', false)
    const b5 = fire('input[data-cell="4-1"]', true)
    return { a1, b5 }
  })
  console.log('synthetic shift-click dispatched:', dispatched)
  await page.waitForTimeout(500)
  // Open the names modal and use "从选区创建"
  await page.locator('[data-testid="sheet-names-btn"]').click({ force: true })
  await page.waitForTimeout(800)
  const fromSelBtn = page.locator('[data-testid="sheet-names-from-selection"]')
  const fromSelEnabled = await fromSelBtn.isEnabled()
  console.log('from-selection enabled:', fromSelEnabled)
  if (!fromSelEnabled) throw new Error('"从选区创建" should be enabled after selecting A1:B5')
  await fromSelBtn.click({ force: true })
  await page.waitForTimeout(500)
  // The name + formula inputs should now be pre-filled.
  const nameAfter = await page.locator('[data-testid="sheet-names-input-name"]').inputValue()
  const formulaAfter = await page.locator('[data-testid="sheet-names-input-formula"]').inputValue()
  console.log('pre-filled name:', nameAfter, 'formula:', formulaAfter)
  // Click "添加" to commit
  await page.locator('[data-testid="sheet-names-add-btn"]').click({ force: true })
  await page.waitForTimeout(4500)
  await page.locator('button:has-text("关闭")').first().click({ force: true })
  await page.waitForTimeout(800)
  // Verify workbook.xml has the Range_A1_B5 defined name
  const afterSelBuf = await downloadArchive()
  const afterSelEntries = await extractArchive(afterSelBuf)
  const afterSelNames = readDefinedNames(afterSelEntries) || []
  const hasRange = afterSelNames.some((n) => /^Range_A1_B5$/i.test(n.name))
  console.log('Range_A1_B5 defined name persisted:', hasRange)

  // ────────────────── Step 3 — Formula = autocomplete ──────────────────
  // Click A2 (next to foo), type "=MyR" into formula bar, verify suggestion appears.
  const a2 = await page.evaluate(() => {
    const cells = Array.from(document.querySelectorAll('[data-cell]'))
    return cells.find((c) => c.getAttribute('data-cell') === 'A2') ? '[data-cell="1-0"]' : null
  })
  if (a2) {
    await page.locator(a2).first().click({ force: true })
    await page.waitForTimeout(300)
  }
  const formulaInput = page.locator('.collab-sheet-editor__formula-input').first()
  await formulaInput.fill('=MyR')
  await page.waitForTimeout(400)
  const suggestVisible = await page.locator('[data-testid="sheet-formula-name-suggest"]').count()
  console.log('autocomplete dropdown visible:', suggestVisible)
  const suggestItems = await page.locator('[data-testid="sheet-formula-name-suggest"] li').count()
  console.log('autocomplete suggestions:', suggestItems)
  // Press Tab to accept the active suggestion
  await formulaInput.press('Tab')
  await page.waitForTimeout(400)
  const accepted = await formulaInput.inputValue()
  console.log('formula after accept:', accepted)
  const acceptedOk = accepted === '=MyRenamed'

  await page.screenshot({ path: '/tmp/wk-shots/sheet-names-autocomplete-99.png', fullPage: false })

  console.log('page errors:', errs.length)
  if (errs.length) console.log(errs)
  const allOk = renamedCorrectly && hasRange && suggestVisible > 0 && suggestItems > 0 && acceptedOk && errs.length === 0
  console.log(allOk ? 'ALL OK — sheet names autocomplete' : 'FAILED')

  await browser.close()
  process.exit(allOk ? 0 : 1)
}

main().catch((e) => { console.error(e); process.exit(1) })
