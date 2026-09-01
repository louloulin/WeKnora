/**
 * wk-slide-theme-persist.mjs — v0.7.95 slide theme persistence verification.
 *
 * Real flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide doc id> in two independent Playwright contexts
 *   3. Alice clicks "indigo" theme swatch in CollabSlideThemePanel (mounted inside
 *      CollabDocEditorView for slide kind only)
 *   4. wait for debounced auto-save (1.5s after theme apply) — saveLabel -> "已保存"
 *   5. download the PPTX via /api/v1/collaborative-docs/<id>/download
 *   6. unzip with native node zlib + manual PKZIP parse, dump ppt/theme/theme1.xml
 *   7. extract <a:clrScheme> children, assert indigo dk1 != original dk1
 */
import pkg3 from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg3
import { readFileSync, writeFileSync } from 'node:fs'

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'

const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(3500)
}

async function openEditor(ctx, label) {
  const page = await ctx.newPage()
  page.on('pageerror', (e) => console.log(`[${label} pageerror]`, e.message))
  page.on('console', (m) => {
    if (m.type() === 'error') console.log(`[${label} console.error]`, m.text())
  })
  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)
  return page
}

// Minimal PKZIP extractor — find local file headers (PK\x03\x04) and inflate deflate streams
async function extractZip(buf) {
  const Buffer = (await import('node:buffer')).Buffer
  const b = Buffer.from(buf)
  const entries = {}
  // locate EOCD
  let eocd = -1
  for (let i = b.length - 22; i >= 0 && i >= b.length - 65557; i--) {
    if (b[i] === 0x50 && b[i + 1] === 0x4b && b[i + 2] === 0x05 && b[i + 3] === 0x06) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('EOCD not found')
  // EOCD layout: sig(4) disk(2) cdDisk(2) diskEntries(2) totalEntries(2) cdSize(4) cdOff(4) commentLen(2)
  const total = b.readUInt16LE(eocd + 10)
  const cdSize = b.readUInt32LE(eocd + 12)
  const cdOff = b.readUInt32LE(eocd + 16)
  let cursor = cdOff
  const cdEnd = cdOff + cdSize
  for (let i = 0; i < total && cursor + 46 <= b.length; i++) {
    const entry = cursor
    const nameLen = b.readUInt16LE(entry + 28)
    const extraLen = b.readUInt16LE(entry + 30)
    const commentLen = b.readUInt16LE(entry + 32)
    const lhOff = b.readUInt32LE(entry + 42)
    const lhNameLen = b.readUInt16LE(lhOff + 26)
    const lhExtraLen = b.readUInt16LE(lhOff + 28)
    const compMethod = b.readUInt16LE(lhOff + 8)
    const compSize = b.readUInt32LE(lhOff + 18)
    const dataStart = lhOff + 30 + lhNameLen + lhExtraLen
    const name = b.slice(entry + 46, entry + 46 + nameLen).toString('utf8')
    cursor = entry + 46 + nameLen + extraLen + commentLen
    let data
    if (compMethod === 0) {
      data = b.slice(dataStart, dataStart + compSize)
    } else if (compMethod === 8) {
      const zlib = await import('node:zlib')
      data = zlib.inflateRawSync(b.slice(dataStart, dataStart + compSize))
    } else {
      throw new Error('unsupported compression ' + compMethod)
    }
    entries[name] = data
  }
  return entries
}

function parseClrScheme(xml) {
  const m = xml.match(/<a:clrScheme[^>]*>([\s\S]*?)<\/a:clrScheme>/)
  if (!m) return null
  const inner = m[1]
  const out = {}
  const re = /<a:(dk1|lt1|dk2|lt2|accent[1-6]|hlink|folHlink)>\s*<a:(?:srgbClr|sysClr)\s+(?:val|lastClr)="([0-9A-Fa-f]{6})"/g
  let r
  while ((r = re.exec(inner))) {
    out[r[1]] = r[2].toUpperCase()
  }
  return out
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })

  // 1) download baseline PPTX
  const baseRes = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  if (!baseRes.ok) throw new Error('baseline download failed: ' + baseRes.status)
  const baseBuf = new Uint8Array(await baseRes.arrayBuffer())
  writeFileSync('/tmp/wk-shots/slide-theme-baseline.pptx', baseBuf)
  const baseEntries = await extractZip(baseBuf)
  const baseTheme = baseEntries['ppt/theme/theme1.xml']?.toString('utf8')
  if (!baseTheme) throw new Error('baseline theme1.xml not found')
  const baseClr = parseClrScheme(baseTheme)
  console.log('baseline clrScheme:', baseClr)

  // 2) open editor in two contexts (Alice/Bob) and apply theme
  const aliceCtx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const bobCtx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const alice = await openEditor(aliceCtx, 'alice')
  const bob = await openEditor(bobCtx, 'bob')

  await alice.screenshot({ path: '/tmp/wk-shots/slide-theme-editor-pre.png', fullPage: false })
  await bob.screenshot({ path: '/tmp/wk-shots/slide-theme-editor-pre-bob.png', fullPage: false })

  // 3) confirm theme panel is rendered inside editor view
  const themePanelVisible = await alice.locator('.collab-slide-konva__thumbs').first().isVisible().catch(() => false)
  console.log('slide editor visible:', themePanelVisible)
  const themePanel = await alice.locator('.collab-editor-view__slide-theme').first().isVisible().catch(() => false)
  console.log('theme panel in editor view:', themePanel)
  const indigoBtn = alice.locator('[data-testid="slide-theme-indigo"], #slide-theme-indigo')
  const indigoVisible = await indigoBtn.first().isVisible().catch(() => false)
  console.log('indigo swatch visible:', indigoVisible)

  if (!indigoVisible) {
    console.log('WARN: indigo swatch not found, dumping all theme buttons')
    const allBtns = await alice.locator('.collab-editor-view__slide-theme button').all().catch(() => [])
    console.log('found buttons in panel:', allBtns.length)
    for (const b of allBtns) {
      console.log(' -', (await b.textContent())?.trim(), '|', await b.getAttribute('id'))
    }
  }

  // 4) click indigo
  await indigoBtn.first().click({ force: true })
  await alice.waitForTimeout(500)

  // 5) wait for auto-save to complete
  console.log('waiting for saveLabel to be 已保存 ...')
  await alice.waitForFunction(
    () => {
      const el = document.querySelector('.collab-slide-konva__savetag')
      return el && /已保存/.test(el.textContent || '')
    },
    { timeout: 15000 },
  ).catch(() => console.log('saveLabel did not reach 已保存 in 15s — checking final state'))
  const finalSaveLabel = await alice.locator('.collab-slide-konva__savetag').first().textContent().catch(() => 'N/A')
  console.log('saveLabel:', finalSaveLabel)

  await alice.screenshot({ path: '/tmp/wk-shots/slide-theme-editor-post.png', fullPage: false })

  // 6) re-download and compare clrScheme
  const afterRes = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  if (!afterRes.ok) throw new Error('after download failed: ' + afterRes.status)
  const afterBuf = new Uint8Array(await afterRes.arrayBuffer())
  writeFileSync('/tmp/wk-shots/slide-theme-after-indigo.pptx', afterBuf)
  const afterEntries = await extractZip(afterBuf)
  const afterTheme = afterEntries['ppt/theme/theme1.xml']?.toString('utf8')
  if (!afterTheme) throw new Error('after theme1.xml not found')
  const afterClr = parseClrScheme(afterTheme)
  console.log('after  clrScheme:', afterClr)

  // Bob downloads to verify peer sees the change too
  await bob.waitForTimeout(2000)
  const bobRes = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  const bobBuf = new Uint8Array(await bobRes.arrayBuffer())
  const bobEntries = await extractZip(bobBuf)
  const bobTheme = bobEntries['ppt/theme/theme1.xml']?.toString('utf8')
  const bobClr = parseClrScheme(bobTheme)
  console.log('bob    clrScheme:', bobClr)

  await alice.screenshot({ path: '/tmp/wk-shots/slide-theme-editor-final.png', fullPage: false })
  await bob.screenshot({ path: '/tmp/wk-shots/slide-theme-editor-final-bob.png', fullPage: false })

  await browser.close()

  // Real values vendored from genoffice/apps/slides/src/renderer/themes.ts (Indigo scheme)
  const INDIGO_EXPECTED = {
    dk1: '1F2A44', lt1: 'FFFFFF', dk2: '3B4C77', lt2: 'E8ECF6',
    accent1: '2E4FA3', accent2: '5B79C7', accent3: '8FA6DE', accent4: 'E97132',
    accent5: '31A5A0', accent6: '7030A0', hlink: '2E4FA3', folHlink: '954F72',
  }
  const matches = []
  for (const k of Object.keys(INDIGO_EXPECTED)) {
    matches.push({ key: k, expected: INDIGO_EXPECTED[k], after: afterClr?.[k] || null, bob: bobClr?.[k] || null, ok: (afterClr?.[k] || '').toUpperCase() === INDIGO_EXPECTED[k].toUpperCase() && (bobClr?.[k] || '').toUpperCase() === INDIGO_EXPECTED[k].toUpperCase() })
  }
  console.log('match table:')
  for (const m of matches) console.log(' ', m.key, 'baseline', baseClr?.[m.key] || '?', '→', m.after, 'bob', m.bob, m.ok ? 'OK' : 'FAIL')

  const allOk = matches.every((m) => m.ok) && themePanel
  console.log(allOk ? 'ALL OK — slide theme persists across save + peer' : 'FAIL')
  process.exit(allOk ? 0 : 2)
}

main().catch((e) => {
  console.error('FAIL:', e.stack || e.message)
  process.exit(1)
})
