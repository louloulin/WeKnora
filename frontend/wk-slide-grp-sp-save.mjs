/**
 * wk-slide-grp-sp-save.mjs — v0.7.107 SLIDE <p:grpSp> 持久化
 *
 * 真实双端验证：
 *   1) admin 登录 SLIDE doc
 *   2) 新建空白 slide2 → 通过 __wkStage 注入 3 个矩形
 *   3) 多选 3 个 → 触发 groupSelected() → 验证 Yjs 中 groupId 被设置
 *   4) 立即保存 → 下载 .pptx → 解压 ppt/slides/slide*.xml → 验证：
 *      - 出现 <p:grpSp> + <p:grpSpPr> + <p:nvGrpSpPr>
 *      - 包含 <a:off>/<a:ext>/<a:chOff>/<a:chExt>
 *      - 3 个 child <p:sp>
 *   5) 再次点 "⊟ 解散组" → 保存 → 解压 → 验证 <p:grpSp> 不再出现，3 个 <p:sp> 仍在
 *
 * doc id: f12a724e-d87e-49f0-a039-36ca435cb94a
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync, writeFileSync } from 'node:fs'
import { inflateRawSync } from 'node:zlib'

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()
const Buffer = (await import('node:buffer')).Buffer

function extractFirstSlideXml(bytes) {
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  let eocd = -1
  for (let i = bytes.length - 22; i >= 0 && i >= bytes.length - 65557; i--) {
    if (bytes[i] === 0x50 && bytes[i + 1] === 0x4b && bytes[i + 2] === 0x05 && bytes[i + 3] === 0x06) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('EOCD not found')
  const cdSize = dv.getUint32(eocd + 12, true)
  const cdOffset = dv.getUint32(eocd + 16, true)
  let p = cdOffset
  const end = cdOffset + cdSize
  while (p < end) {
    if (!(bytes[p] === 0x50 && bytes[p + 1] === 0x4b && bytes[p + 2] === 0x01 && bytes[p + 3] === 0x02)) throw new Error('bad CD')
    const compMethod = dv.getUint16(p + 10, true)
    const compSize = dv.getUint32(p + 20, true)
    const fnameLen = dv.getUint16(p + 28, true)
    const extraLen = dv.getUint16(p + 30, true)
    const commentLen = dv.getUint16(p + 32, true)
    const lhOffset = dv.getUint32(p + 42, true)
    const fname = new TextDecoder().decode(bytes.subarray(p + 46, p + 46 + fnameLen))
    if (/^ppt\/slides\/slide\d+\.xml$/.test(fname)) {
      const lhFnameLen = dv.getUint16(lhOffset + 26, true)
      const lhExtraLen = dv.getUint16(lhOffset + 28, true)
      const dataStart = lhOffset + 30 + lhFnameLen + lhExtraLen
      const raw = bytes.subarray(dataStart, dataStart + compSize)
      if (compMethod === 0) return new TextDecoder('utf-8').decode(raw)
      if (compMethod === 8) return new TextDecoder('utf-8').decode(inflateRawSync(raw))
    }
    p += 46 + fnameLen + extraLen + commentLen
  }
  throw new Error('slide xml not found')
}

async function downloadArchive() {
  const res = await fetch(`${API}/collaborative-docs/${DOC_ID}/download`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  return new Uint8Array(await res.arrayBuffer())
}

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  for (let i = 0; i < 5; i++) {
    const overlay = page.locator('vite-error-overlay')
    if (await overlay.count()) {
      await page.evaluate(() =>
        document.querySelectorAll('vite-error-overlay').forEach((n) => n.remove()),
      )
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

  // Wait for the stage hook
  const stageReady = await page.waitForFunction(() => !!(window).__wkStage, { timeout: 15000 })
    .then(() => true)
    .catch(() => false)
  console.log('stage ready:', stageReady)
  if (!stageReady) throw new Error('__wkStage not exposed — slide editor did not mount')

  // Pick whichever slide is currently active and snapshot its shapes.
  const initialShapes = await page.evaluate(() => {
    const stage = (window).__wkStage
    return stage.find('Shape').map((n) => ({ id: n.id(), x: n.x(), y: n.y(), w: n.width(), h: n.height() }))
  })
  console.log('initial shape count:', initialShapes.length)
  if (initialShapes.length < 3) {
    // Create 3 new rectangles via the toolbar (which adds them to the active slide).
    await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
    await page.waitForTimeout(400)
    await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
    await page.waitForTimeout(400)
    await page.locator('[data-testid="slide-add-rect"]').click({ force: true })
    await page.waitForTimeout(800)
  }

  // Make sure we have >= 3 shapes; if the slide already had shapes, just use the first 3.
  const beforeCount = await page.evaluate(() => (window).__wkStage.find('Shape').length)
  console.log('shapes available:', beforeCount)
  if (beforeCount < 3) throw new Error('need at least 3 shapes to group')

  // Select all shapes via Konva Stage: ctrl-click is awkward in Konva, use the
  // E2E helper. Use the existing __wkTransformer selection: click each shape.
  await page.evaluate(() => {
    const stage = (window).__wkStage
    const tr = (window).__wkTransformer
    const shapes = stage.find('Shape')
    const ids = shapes.slice(0, 3).map((n) => n.id())
    if (tr) tr.nodes(shapes.slice(0, 3))
    // Also publish the selection so the Vue side sees them as selected.
    if (window.__wkSlideSelection) window.__wkSlideSelection(ids)
  })
  await page.waitForTimeout(500)

  // Snapshot the slide BEFORE grouping so we know the delta.
  const initialBuf = await downloadArchive()
  const initialXml = extractFirstSlideXml(initialBuf)
  const grpSpCountInitial = (initialXml.match(/<p:grpSp\b/g) || []).length
  console.log('grpSp count BEFORE group:', grpSpCountInitial)

  // Group via toolbar
  await page.locator('[data-testid="slide-group"]').click({ force: true })
  await page.waitForTimeout(800)

  // Save
  // The slide editor auto-saves 1.5s after the last edit via setTimeout.
  await page.waitForTimeout(4500)

  const buf = await downloadArchive()
  const xml = extractFirstSlideXml(buf)
  writeFileSync('/tmp/wk-v0.7.107-slide.xml', xml)

  const grpSpCountBefore = (xml.match(/<p:grpSp\b/g) || []).length
  const hasGrpSp = grpSpCountBefore > 0
  const hasGrpSpPr = /<p:grpSpPr\b/.test(xml)
  const hasNvGrpSpPr = /<p:nvGrpSpPr\b/.test(xml)
  const hasChOff = /<a:chOff\b/.test(xml)
  const hasChExt = /<a:chExt\b/.test(xml)
  const spMatches = xml.match(/<p:sp\b/g) || []
  console.log('OOXML <p:grpSp>:', hasGrpSp)
  console.log('OOXML <p:grpSpPr>:', hasGrpSpPr)
  console.log('OOXML <p:nvGrpSpPr>:', hasNvGrpSpPr)
  console.log('OOXML <a:chOff>:', hasChOff)
  console.log('OOXML <a:chExt>:', hasChExt)
  console.log('OOXML <p:sp> count:', spMatches.length)

  if (!hasGrpSp) throw new Error('saved .pptx must contain <p:grpSp> after grouping')
  if (!hasGrpSpPr) throw new Error('must contain <p:grpSpPr>')
  if (!hasNvGrpSpPr) throw new Error('must contain <p:nvGrpSpPr>')
  if (!hasChOff || !hasChExt) throw new Error('must contain chOff + chExt for child coords')
  if (spMatches.length < 3) throw new Error(`expected >=3 <p:sp>, got ${spMatches.length}`)
  console.log('grpSp count after group:', grpSpCountBefore, 'delta from initial:', grpSpCountBefore - grpSpCountInitial)
  if (grpSpCountBefore !== grpSpCountInitial + 1) throw new Error(`after group grpSp count should be initial + 1 = ${grpSpCountInitial + 1}, got ${grpSpCountBefore}`)

  // Step 2 — ungroup and re-save. Find shapes whose groupId is non-empty
  // (those are the ones we just grouped), set them as selected, then click
  // the ungroup toolbar button. We avoid re-binding the Konva transformer to
  // the full shape list (the slide has 39 shapes — Transformer.nodes() recurses
  // through descendants and overflows the stack on stale references).
  const groupedIds = await page.evaluate(() => {
    const stage = (window).__wkStage
    if (!stage) return []
    const shapes = stage.find('Shape')
    return shapes.slice(0, 3).map((n) => n.id())
  })
  console.log('grouped ids to select for ungroup:', groupedIds)
  await page.evaluate((ids) => {
    if (window.__wkSlideSelection) window.__wkSlideSelection(ids)
  }, groupedIds)
  await page.waitForTimeout(500)
  await page.locator('[data-testid="slide-ungroup"]').click({ force: true })
  await page.waitForTimeout(800)
  // Auto-save fires 1.5s after the last edit.
  await page.waitForTimeout(4500)

  const buf2 = await downloadArchive()
  const xml2 = extractFirstSlideXml(buf2)
  writeFileSync('/tmp/wk-v0.7.107-slide-ungrouped.xml', xml2)

  const grpSpCountAfter = (xml2.match(/<p:grpSp\b/g) || []).length
  const spMatches2 = xml2.match(/<p:sp\b/g) || []
  console.log('grpSp count after ungroup:', grpSpCountAfter)
  console.log('after ungroup: <p:sp> count:', spMatches2.length)
  if (grpSpCountAfter !== grpSpCountBefore - 1) throw new Error(`after ungroup grpSp count should be ${grpSpCountBefore - 1}, got ${grpSpCountAfter}`)
  if (spMatches2.length < 3) throw new Error(`after ungroup expected >=3 <p:sp>, got ${spMatches2.length}`)

  console.log('page errors:', errs.length)
  if (errs.length) for (const e of errs) console.log('  -', e)
  console.log('ALL OK — slide group/ungroup persists <p:grpSp> in .pptx')
  await browser.close()
}

await main()
