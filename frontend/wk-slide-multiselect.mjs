/**
 * wk-slide-multiselect.mjs — v0.7.101 SLIDE multi-select + distribute + match size.
 *
 * Flow (on a fresh slide to keep click targets clean):
 *   1. real admin login, open SLIDE doc
 *   2. "+ 新建幻灯片" -> empty slide 2
 *   3. add 3 rects: rectA left / rectB right / rectC centerH
 *   4. resize rectB via Konva transformer (drag left-middle anchor -60px)
 *   5. shift-click rectA, rectC, rectB -> multi-select (primary = rectB)
 *   6. "⇔ 分布" (distribute-h) -> x positions evenly spaced
 *   7. "⤢ 匹配宽" (match-width) -> all cx equal to rectB's new width
 *   8. "⇤ 左对齐" (multi align) -> all x = bbox min x
 *   9. download PPTX, unzip, parse slide2.xml -> verify each step
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync } from 'node:fs'

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()

const Buffer = (await import('node:buffer')).Buffer
const EMU_IN = 914400
const PX_IN = 96
const emuToPx = (emu) => (emu / EMU_IN) * PX_IN

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

function readShapesFromSlide(entries, slideName) {
  const xml = entries[slideName]?.toString('utf8')
  if (!xml) return null
  const shapes = []
  const re = /<p:sp\b[\s\S]*?<\/p:sp>/g
  let m
  while ((m = re.exec(xml))) {
    const block = m[0]
    const offMatch = /<a:off\s+x="(-?\d+)"\s+y="(-?\d+)"/.exec(block)
    const extMatch = /<a:ext\s+cx="(\d+)"\s+cy="(\d+)"/.exec(block)
    if (offMatch && extMatch) {
      shapes.push({ x: parseInt(offMatch[1]), y: parseInt(offMatch[2]), cx: parseInt(extMatch[1]), cy: parseInt(extMatch[2]) })
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
  const ctx = await browser.newContext({ viewport: { width: 2000, height: 1100 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // fresh slide 2
  await page.locator('button:has-text("+ 新建幻灯片")').first().click({ force: true })
  await page.waitForTimeout(3000)

  const baselineBuf = await downloadArchive()
  const baselineEntries = await extractArchive(baselineBuf)
  const slideSize = readSlideSize(baselineEntries)
  const slideFiles = Object.keys(baselineEntries).filter((k) => /^ppt\/slides\/slide\d+\.xml$/.test(k))
  const slide2 = slideFiles.sort((a, b) => parseInt(b.match(/slide(\d+)/)[1]) - parseInt(a.match(/slide(\d+)/)[1]))[0]
  console.log('slide size:', slideSize, 'new slide file:', slide2)
  if (!slide2) throw new Error('no slide file after addSlide')

  // --- add 3 rects: rectA left / rectB right / rectC centerH ---
  await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
  await page.waitForTimeout(400)
  await page.locator('[data-testid="slide-align-left"]').click({ force: true })
  await page.waitForTimeout(400)

  await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
  await page.waitForTimeout(400)
  await page.locator('[data-testid="slide-align-right"]').click({ force: true })
  await page.waitForTimeout(400)

  await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
  await page.waitForTimeout(400)
  await page.locator('[data-testid="slide-align-center-h"]').click({ force: true })
  await page.waitForTimeout(3000)

  const afterPosBuf = await downloadArchive()
  const afterPosEntries = await extractArchive(afterPosBuf)
  const afterPosShapes = readShapesFromSlide(afterPosEntries, slide2) || []
  console.log('after positioning, slide2 shapes:', afterPosShapes)
  const w0 = afterPosShapes[0].cx
  const slideW = slideSize.cx
  const expectPos = afterPosShapes.length === 3 &&
    afterPosShapes[0].x === 0 &&
    afterPosShapes[1].x === slideW - w0 &&
    Math.abs(afterPosShapes[2].x - Math.round((slideW - w0) / 2)) <= 1
  console.log('expect positioned left/right/centerH:', expectPos)

  const canvasRect = await page.evaluate(() => {
    const c = document.querySelector('.collab-slide-konva__stage canvas')
    if (!c) return null
    const r = c.getBoundingClientRect()
    return { left: r.left, top: r.top, width: r.width, height: r.height }
  })
  console.log('canvas rect:', canvasRect)
  if (!canvasRect) throw new Error('canvas not found')

  const centerOf = (s) => ({
    x: canvasRect.left + emuToPx(s.x + s.cx / 2),
    y: canvasRect.top + emuToPx(s.y + s.cy / 2),
  })

  // --- resize rectB (index 1) via transformer left-middle anchor ---
  const rectB = afterPosShapes[1]
  const cB = centerOf(rectB)
  await page.mouse.click(cB.x, cB.y)
  await page.waitForTimeout(400)
  const anchor = { x: canvasRect.left + emuToPx(rectB.x), y: canvasRect.top + emuToPx(rectB.y + rectB.cy / 2) }
  await page.mouse.move(anchor.x, anchor.y)
  await page.mouse.down()
  await page.mouse.move(anchor.x - 60, anchor.y, { steps: 8 })
  await page.mouse.up()
  await page.waitForTimeout(3000)

  const afterResizeBuf = await downloadArchive()
  const afterResizeEntries = await extractArchive(afterResizeBuf)
  const afterResizeShapes = readShapesFromSlide(afterResizeEntries, slide2) || []
  console.log('after resize, slide2 shapes:', afterResizeShapes)
  const rectBNewW = afterResizeShapes[1].cx
  const expectResize = rectBNewW > w0
  console.log('expect rectB resized wider:', expectResize, '(w0=' + w0 + ' new=' + rectBNewW + ')')

  // --- multi-select: shift-click rectA, rectC, rectB (primary = rectB) ---
  // Use keyboard.down('Shift') because page.mouse.click({modifiers}) doesn't
  // reliably set shiftKey on Konva canvas click events.
  const cA = centerOf(afterResizeShapes[0])
  const cC = centerOf(afterResizeShapes[2])
  const cB2 = centerOf(afterResizeShapes[1])
  await page.mouse.click(cA.x, cA.y)
  await page.waitForTimeout(200)
  await page.keyboard.down('Shift')
  await page.mouse.click(cC.x, cC.y)
  await page.waitForTimeout(200)
  await page.mouse.click(cB2.x, cB2.y)
  await page.keyboard.up('Shift')
  await page.waitForTimeout(400)

  const distDisabled = await page.locator('[data-testid="slide-distribute-h"]').isDisabled().catch(() => true)
  const matchDisabled = await page.locator('[data-testid="slide-match-width"]').isDisabled().catch(() => true)
  console.log('distribute-h disabled after multi-select:', distDisabled)
  console.log('match-width disabled after multi-select:', matchDisabled)
  await page.screenshot({ path: '/tmp/wk-shots/slide-multiselect-01.png', fullPage: false })

  // --- distribute-h ---
  await page.locator('[data-testid="slide-distribute-h"]').click({ force: true })
  await page.waitForTimeout(3000)
  const afterDistBuf = await downloadArchive()
  const afterDistEntries = await extractArchive(afterDistBuf)
  const afterDistShapes = readShapesFromSlide(afterDistEntries, slide2) || []
  const mineD = [...afterDistShapes].sort((a, b) => a.x - b.x)
  console.log('after distribute-h (sorted by x):', mineD)
  const gaps = [mineD[1].x - (mineD[0].x + mineD[0].cx), mineD[2].x - (mineD[1].x + mineD[1].cx)]
  const expectDist = Math.abs(gaps[0] - gaps[1]) <= 1
  console.log('gaps:', gaps, 'expect equal gaps:', expectDist)

  // --- match-width (primary = rectB) ---
  await page.locator('[data-testid="slide-match-width"]').click({ force: true })
  await page.waitForTimeout(3000)
  const afterMatchBuf = await downloadArchive()
  const afterMatchEntries = await extractArchive(afterMatchBuf)
  const afterMatchShapes = readShapesFromSlide(afterMatchEntries, slide2) || []
  const widths = afterMatchShapes.map((s) => s.cx)
  const expectMatch = widths.every((w) => w === widths[0]) && widths[0] === rectBNewW
  console.log('after match-width, cx:', widths, 'expect all =', rectBNewW, ':', expectMatch)

  // --- multi align-left (all x = bbox min) ---
  await page.locator('[data-testid="slide-align-left"]').click({ force: true })
  await page.waitForTimeout(3000)
  const afterAlignBuf = await downloadArchive()
  const afterAlignEntries = await extractArchive(afterAlignBuf)
  const afterAlignShapes = readShapesFromSlide(afterAlignEntries, slide2) || []
  const minX = Math.min(...afterAlignShapes.map((s) => s.x))
  const expectAlign = afterAlignShapes.every((s) => s.x === minX)
  console.log('after multi align-left, x:', afterAlignShapes.map((s) => s.x), 'expect all =', minX, ':', expectAlign)

  await page.screenshot({ path: '/tmp/wk-shots/slide-multiselect-99.png', fullPage: false })
  await browser.close()

  const ok = expectPos && expectResize && !distDisabled && !matchDisabled && expectDist && expectMatch && expectAlign && errs.length === 0
  console.log('---')
  console.log('page errors:', errs.length)
  for (const e of errs) console.log(' -', e.slice(0, 200))
  console.log(ok ? 'ALL OK — slide multi-select' : 'FAIL')
  process.exit(ok ? 0 : 1)
}

main().catch((e) => { console.error('ERROR', e); process.exit(1) })
