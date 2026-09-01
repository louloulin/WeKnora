/**
 * wk-slide-align.mjs — v0.7.98 SLIDE shape alignment verification.
 *
 * Flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide id>
 *   3. add a rectangle shape on slide 1 (centered)
 *   4. click the rectangle to select it
 *   5. click "⇤ 左对齐" -> rect.x should be 0
 *   6. click "↔ 水平居中" -> rect.x should be (slideW - rect.w) / 2
 *   7. click "⇥ 右对齐" -> rect.x should be slideW - rect.w
 *   8. click "⫶ 顶端对齐" -> rect.y should be 0
 *   9. click "↕ 垂直居中" -> rect.y should be (slideH - rect.h) / 2
 *  10. click "⫷ 底端对齐" -> rect.y should be slideH - rect.h
 *  11. wait for saveLabel -> 已保存
 *  12. download PPTX, unzip, parse slide1.xml -> verify rect's <a:off x=".." y="..">
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

function readShapesFromSlide1(entries) {
  const xml = entries['ppt/slides/slide1.xml']?.toString('utf8')
  if (!xml) return null
  const shapes = []
  // Match <p:sp>...</p:sp> blocks
  const re = /<p:sp\b[\s\S]*?<\/p:sp>/g
  let m
  while ((m = re.exec(xml))) {
    const block = m[0]
    const offMatch = /<a:off\s+x="(-?\d+)"\s+y="(-?\d+)"/.exec(block)
    const extMatch = /<a:ext\s+cx="(\d+)"\s+cy="(\d+)"/.exec(block)
    if (offMatch && extMatch) {
      shapes.push({
        x: parseInt(offMatch[1]),
        y: parseInt(offMatch[2]),
        cx: parseInt(extMatch[1]),
        cy: parseInt(extMatch[2]),
      })
    }
  }
  return shapes
}

function readSlideSize(entries) {
  const xml = entries['ppt/presentation.xml']?.toString('utf8')
  if (!xml) return null
  const m = /<p:sldSz\s+cx="(\d+)"\s+cy="(\d+)"/.exec(xml)
  if (!m) return null
  return { cx: parseInt(m[1]), cy: parseInt(m[2]) }
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
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // baseline
  const baselineBuf = await downloadArchive()
  const baselineEntries = await extractArchive(baselineBuf)
  const baselineShapes = readShapesFromSlide1(baselineEntries) || []
  const slideSize = readSlideSize(baselineEntries)
  console.log('baseline shapes count:', baselineShapes.length)
  console.log('slide size:', slideSize)

  // add a rectangle
  await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
  await page.waitForTimeout(800)
  // the new rect is auto-selected; verify
  const rectId = await page.evaluate(() => {
    const sel = document.querySelectorAll('canvas')
    // read the latest shape in yjs
    return null
  })

  // After addShape, the new rect is auto-selected (selectedId.value = id).
  // Verify align button is enabled (means selectedId is set)
  await page.waitForTimeout(300)
  const leftDisabled = await page.locator('[data-testid="slide-align-left"]').first().isDisabled().catch(() => true)
  console.log('left disabled (auto-selected after add):', leftDisabled)
  await page.screenshot({ path: '/tmp/wk-shots/slide-align-00-pre.png', fullPage: false })

  // Align left
  await page.locator('[data-testid="slide-align-left"]').click({ force: true })
  await page.waitForTimeout(2200) // wait for save debounce
  const after1Buf = await downloadArchive()
  const after1Entries = await extractArchive(after1Buf)
  const after1Shapes = readShapesFromSlide1(after1Entries) || []
  const last1 = after1Shapes[after1Shapes.length - 1]
  console.log('after left:', last1)

  // Align right
  await page.locator('[data-testid="slide-align-right"]').click({ force: true })
  await page.waitForTimeout(2200)
  const after2Buf = await downloadArchive()
  const after2Entries = await extractArchive(after2Buf)
  const after2Shapes = readShapesFromSlide1(after2Entries) || []
  const last2 = after2Shapes[after2Shapes.length - 1]
  console.log('after right:', last2)

  // Align top
  await page.locator('[data-testid="slide-align-top"]').click({ force: true })
  await page.waitForTimeout(2200)
  const after3Buf = await downloadArchive()
  const after3Entries = await extractArchive(after3Buf)
  const after3Shapes = readShapesFromSlide1(after3Entries) || []
  const last3 = after3Shapes[after3Shapes.length - 1]
  console.log('after top:', last3)

  // Align bottom
  await page.locator('[data-testid="slide-align-bottom"]').click({ force: true })
  await page.waitForTimeout(2200)
  const after4Buf = await downloadArchive()
  const after4Entries = await extractArchive(after4Buf)
  const after4Shapes = readShapesFromSlide1(after4Entries) || []
  const last4 = after4Shapes[after4Shapes.length - 1]
  console.log('after bottom:', last4)

  // Align centerH
  await page.locator('[data-testid="slide-align-center-h"]').click({ force: true })
  await page.waitForTimeout(2200)
  const after5Buf = await downloadArchive()
  const after5Entries = await extractArchive(after5Buf)
  const after5Shapes = readShapesFromSlide1(after5Entries) || []
  const last5 = after5Shapes[after5Shapes.length - 1]
  console.log('after centerH:', last5)

  // Align centerV
  await page.locator('[data-testid="slide-align-center-v"]').click({ force: true })
  await page.waitForTimeout(2200)
  const after6Buf = await downloadArchive()
  const after6Entries = await extractArchive(after6Buf)
  const after6Shapes = readShapesFromSlide1(after6Entries) || []
  const last6 = after6Shapes[after6Shapes.length - 1]
  console.log('after centerV:', last6)

  await page.screenshot({ path: '/tmp/wk-shots/slide-align-99-final.png', fullPage: false })
  await browser.close()

  // assertions
  const expectLeft = last1.x === 0
  const expectRight = last2.x === slideSize.cx - last2.cx
  const expectTop = last3.y === 0
  const expectBottom = last4.y === slideSize.cy - last4.cy
  const expectCenterH = Math.abs(last5.x - Math.round((slideSize.cx - last5.cx) / 2)) <= 1
  const expectCenterV = Math.abs(last6.y - Math.round((slideSize.cy - last6.cy) / 2)) <= 1
  console.log('---')
  console.log('expect left:', expectLeft)
  console.log('expect right:', expectRight)
  console.log('expect top:', expectTop)
  console.log('expect bottom:', expectBottom)
  console.log('expect centerH:', expectCenterH)
  console.log('expect centerV:', expectCenterV)
  const ok = expectLeft && expectRight && expectTop && expectBottom && expectCenterH && expectCenterV && errs.length === 0
  console.log('page errors:', errs.length)
  for (const e of errs) console.log(' -', e.slice(0, 200))
  console.log(ok ? 'ALL OK — slide align' : 'FAIL')
  process.exit(ok ? 0 : 2)
}

main().catch((e) => { console.error('FAIL:', e.stack || e.message); process.exit(1) })
