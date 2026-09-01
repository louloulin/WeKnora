/**
 * wk-slide-layout.mjs — v0.7.97 SLIDE layout switcher verification.
 *
 * Flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide id>
 *   3. read the layout <select> option list (existing + missing builtins)
 *   4. pick a builtin (e.g. "Title and Content") — ensureBuiltinLayout + setSlideLayout
 *   5. wait for saveLabel -> 已保存
 *   6. download PPTX, unzip, parse slide1.xml.rels -> verify Target = new layout path
 *   7. verify the new layout file exists in the package (ppt/slideLayouts/slideLayoutN.xml)
 *   8. pick another builtin -> confirm rels target updates
 *   9. second peer re-downloads and sees the new layout path
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync } from 'node:fs'

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
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

function readLayoutTarget(entries, slidePath) {
  const relsPath = 'ppt/slides/_rels/' + slidePath.replace(/^ppt\/slides\//, '') + '.rels'
  const xml = entries[relsPath]?.toString('utf8')
  if (!xml) return null
  const m = xml.match(/<Relationship[^>]*Type="[^"]*slideLayout"[^>]*Target="([^"]+)"/)
  return m ? m[1] : null
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

  // 1) baseline
  const baselineBuf = await downloadArchive()
  const baselineEntries = await extractArchive(baselineBuf)
  const baselineTarget = readLayoutTarget(baselineEntries, 'ppt/slides/slide1.xml')
  const baselineLayouts = Object.keys(baselineEntries).filter((p) => /^ppt\/slideLayouts\/slideLayout\d+\.xml$/.test(p))
  console.log('baseline slide1 layout target:', baselineTarget)
  console.log('baseline layouts in package:', baselineLayouts.sort())

  // 2) open editor (alice)
  const aliceCtx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const alice = await aliceCtx.newPage()
  alice.on('pageerror', (e) => console.log('[alice pageerror]', e.message))
  await login(alice)
  await alice.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await alice.waitForTimeout(8000)

  // 3) read layout select options
  const options = await alice.locator('[data-testid="slide-layout-select"] option').all()
  console.log('layout option count:', options.length)
  for (const o of options) {
    const v = await o.getAttribute('value')
    const t = (await o.textContent())?.trim()
    console.log(' - value=' + (v ?? '') + ' text=' + t)
  }

  // 4) pick a builtin via optgroup
  const titleContentOption = alice.locator('[data-testid="slide-layout-select"] option').filter({ hasText: 'Title and Content' }).first()
  const titleContentValue = await titleContentOption.getAttribute('value')
  console.log('picking:', titleContentValue)
  await alice.locator('[data-testid="slide-layout-select"]').first().selectOption(titleContentValue ?? '')
  await alice.waitForTimeout(2500) // wait for ensureBuiltinLayout + setSlideLayout + auto-save
  await alice.waitForFunction(() => {
    const el = document.querySelector('.collab-slide-konva__savetag')
    return el && /已保存/.test(el.textContent || '')
  }, { timeout: 15000 }).catch(() => console.log('saveLabel did not reach 已保存 in 15s'))

  // 5) download + verify
  const after1Buf = await downloadArchive()
  const after1Entries = await extractArchive(after1Buf)
  const after1Target = readLayoutTarget(after1Entries, 'ppt/slides/slide1.xml')
  const after1Layouts = Object.keys(after1Entries).filter((p) => /^ppt\/slideLayouts\/slideLayout\d+\.xml$/.test(p)).sort()
  console.log('after apply slide1 layout target:', after1Target)
  console.log('after apply layouts in package:', after1Layouts)
  const after1TitleContent = after1Entries[after1Target?.replace(/^\.\.\//, 'ppt/') ?? '']?.toString('utf8')

  // 6) second layout switch
  const blankOption = alice.locator('[data-testid="slide-layout-select"] option').filter({ hasText: 'Blank' }).first()
  const blankValue = await blankOption.getAttribute('value')
  console.log('picking:', blankValue)
  await alice.locator('[data-testid="slide-layout-select"]').first().selectOption(blankValue ?? '')
  await alice.waitForTimeout(2500)
  await alice.waitForFunction(() => {
    const el = document.querySelector('.collab-slide-konva__savetag')
    return el && /已保存/.test(el.textContent || '')
  }, { timeout: 15000 }).catch(() => console.log('saveLabel did not reach 已保存 in 15s'))

  const after2Buf = await downloadArchive()
  const after2Entries = await extractArchive(after2Buf)
  const after2Target = readLayoutTarget(after2Entries, 'ppt/slides/slide1.xml')
  const after2Layouts = Object.keys(after2Entries).filter((p) => /^ppt\/slideLayouts\/slideLayout\d+\.xml$/.test(p)).sort()
  console.log('after blank slide1 layout target:', after2Target)
  console.log('after blank layouts in package:', after2Layouts)
  const after2Switched = after2Entries[after1Target?.replace(/^\.\.\//, 'ppt/') ?? '']?.toString('utf8')

  await alice.screenshot({ path: '/tmp/wk-shots/slide-layout-after.png', fullPage: false })

  // 7) bob downloads to verify peer
  const bobCtx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const bob = await bobCtx.newPage()
  await login(bob)
  const bobBuf = await downloadArchive()
  const bobEntries = await extractArchive(bobBuf)
  const bobTarget = readLayoutTarget(bobEntries, 'ppt/slides/slide1.xml')
  console.log('bob slide1 layout target:', bobTarget)
  await bob.screenshot({ path: '/tmp/wk-shots/slide-layout-bob.png', fullPage: false })

  await browser.close()

  const ok =
    baselineTarget &&
    after1Target &&
    after2Target &&
    after1Target !== baselineTarget &&
    after2Target !== after1Target &&
    after1Layouts.length >= baselineLayouts.length &&
    bobTarget === after2Target &&
    /Title and Content/.test(after1TitleContent || '') &&
    /Title and Content/.test(after2Switched || '')
  console.log('---summary---')
  console.log('baseline target ->', baselineTarget, '|', baselineLayouts.length, 'layouts')
  console.log('after title&content ->', after1Target, '|', after1Layouts.length, 'layouts')
  console.log('after blank ->', after2Target, '|', after2Layouts.length, 'layouts')
  console.log('bob sees ->', bobTarget)
  console.log(ok ? 'ALL OK — slide layout switcher' : 'FAIL')
  process.exit(ok ? 0 : 2)
}

main().catch((e) => { console.error('FAIL:', e.stack || e.message); process.exit(1) })
