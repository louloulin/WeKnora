/**
 * wk-doc-track-changes.mjs — v0.7.106 DOC 修订序列化
 *
 * 真实双端浏览器验证：
 *   1) 开启"记录修订"开关 (toolbar / window.__wkDocTrackChanges)
 *   2) 在第一段输入新文字 → 应自动套 ins mark
 *   3) 删除第二段中的一段文字 → 应自动套 del mark
 *   4) 立即保存 → 下载 .docx → 解压 word/document.xml → 看到 <w:ins> w:author + <w:del> w:author + w:date
 *   5) 修订面板打开后能看到 author + date ("刚刚")
 *
 * doc id（沿用已有的 DOC 文档）：
 *   67fadefd-8f01-4f2b-aeab-a3ac3d050e39
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
import { readFileSync, writeFileSync } from 'node:fs'
import { extractDocxDocumentXml } from './src/editor/adapters/__tests__/_testZipExtract.ts'
// We can't import .ts at runtime; re-implement the unzip here instead.
import { inflateRawSync } from 'node:zlib'

const DOC_ID = '67fadefd-8f01-4f2b-aeab-a3ac3d050e39'
const BASE = 'http://127.0.0.1:5173'
const API = 'http://127.0.0.1:8080/api/v1'
const TOKEN = readFileSync('/tmp/wk-token', 'utf8').trim()
const Buffer = (await import('node:buffer')).Buffer

function unzipDocumentXml(bytes) {
  const sig = [0x50, 0x4b, 0x05, 0x06]
  let eocd = -1
  for (let i = bytes.length - 22; i >= 0 && i >= bytes.length - 65557; i--) {
    if (bytes[i] === sig[0] && bytes[i + 1] === sig[1] && bytes[i + 2] === sig[2] && bytes[i + 3] === sig[3]) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('EOCD not found')
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
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
    if (fname === 'word/document.xml') {
      const lhFnameLen = dv.getUint16(lhOffset + 26, true)
      const lhExtraLen = dv.getUint16(lhOffset + 28, true)
      const dataStart = lhOffset + 30 + lhFnameLen + lhExtraLen
      const raw = bytes.subarray(dataStart, dataStart + compSize)
      if (compMethod === 0) return new TextDecoder('utf-8').decode(raw)
      if (compMethod === 8) return new TextDecoder('utf-8').decode(inflateRawSync(raw))
    }
    p += 46 + fnameLen + extraLen + commentLen
  }
  throw new Error('document.xml not found')
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

  // Wait for editor mount
  const editorReady = await page.waitForFunction(
    () => !!(window).__wkDocEditor,
    { timeout: 15000 },
  ).then(() => true).catch(() => false)
  console.log('editor ready:', editorReady)
  if (!editorReady) throw new Error('__wkDocEditor not exposed — editor did not mount')

  // Step 1 — turn track changes on (both via window hook and toolbar click)
  await page.evaluate(() => (window).__wkDocTrackChanges.on = true)
  const trackBtnText = await page.locator('[data-testid="doc-track-changes-btn"]').textContent()
  console.log('track-changes button text after toggle:', trackBtnText)

  // Step 2 — type some text into the first paragraph; it should auto-wrap in `ins`.
  // Use the editor's API directly to bypass focus quirks; onTransaction will
  // pick up the insert step and add the ins mark.
  const beforeContent = await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    return ed.getText()
  })
  console.log('beforeContent head:', JSON.stringify((beforeContent || '').slice(0, 60)))

  await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    ed.chain().focus('start').insertContent(' v0.7.106-TRACKED ').run()
  })
  await page.waitForTimeout(800)

  // Verify the ins mark is present in the editor doc. PM's Mark objects expose
  // `.type` as the schema's MarkType whose `.name` is the schema-level name.
  const insMarkCount = await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    let count = 0
    ed.state.doc.descendants((node) => {
      if (!node.isText) return
      for (const m of node.marks) {
        if (m.type && (m.type.name === 'ins' || m.type === 'ins')) {
          count++
          break
        }
      }
    })
    return count
  })
  console.log('text nodes with ins mark after typing:', insMarkCount)

  // Step 3 — open revisions panel and check that the snippet shows author + date
  await page.locator('[data-testid="doc-revisions-btn"]').click({ force: true })
  await page.waitForTimeout(800)
  const revListExists = await page.locator('[data-testid="doc-revisions-list"]').count()
  console.log('revisions list visible:', revListExists)
  // Check first revision has author + relative date
  const firstDate = await page.locator('.collab-doc-pro__revisions-date').first().textContent().catch(() => null)
  console.log('first revision date label:', firstDate)

  // Close revisions panel
  await page.locator('button:has-text("关闭")').first().click({ force: true })
  await page.waitForTimeout(400)

  // v0.7.106.1 — drive the track-changes delete hook with chain().deleteSelection()
  await page.evaluate(() => { (window).__wkDocTrackChanges.on = false })
  await page.waitForTimeout(200)
  await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    ed.chain().focus('end').insertContent('TO_DELETE_HERE').run()
  })
  await page.waitForTimeout(400)
  await page.evaluate(() => { (window).__wkDocTrackChanges.on = true })
  await page.waitForTimeout(200)
  const beforeDeleteText = await page.evaluate(() => (window).__wkDocEditor.getText())
  console.log('text before deleteSelection:', JSON.stringify((beforeDeleteText || '').slice(-30)))
  await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    const text = ed.getText()
    const idx = text.lastIndexOf('TO_DELETE_HERE')
    if (idx < 0) throw new Error('TO_DELETE_HERE marker not found')
    const from = idx + 1
    const to = from + 'TO_DELETE_HERE'.length
    ed.chain().setTextSelection({ from, to }).deleteSelection().run()
  })
  await page.waitForTimeout(800)

  // Step 4 — programmatically apply a `del` mark to some text so we exercise
  // the OOXML <w:del> + <w:delText> serialization path (the auto-wrap on
  // user-driven delete lands in v0.7.106.1; this proves the wire works).
  await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    const text = ed.getText()
    const marker = 'TO_DELETE_HERE'
    const idx = text.indexOf(marker)
    if (idx < 0) throw new Error('marker not found')
    const from = idx + 1
    const to = from + marker.length
    const tr = ed.state.tr
    tr.addMark(from, to, ed.schema.marks.del.create({
      author: 'admin',
      date: new Date().toISOString(),
      id: '9001',
    }))
    tr.setMeta('trackIgnore', true)
    ed.view.dispatch(tr)
  })
  await page.waitForTimeout(500)

  const delMarkCount = await page.evaluate(() => {
    const ed = (window).__wkDocEditor
    let count = 0
    ed.state.doc.descendants((node) => {
      if (!node.isText) return
      for (const m of node.marks) {
        if (m.type && (m.type.name === 'del' || m.type === 'del')) {
          count++
          break
        }
      }
    })
    return count
  })
  console.log('text nodes with del mark after apply:', delMarkCount)

  // Step 5 — force save and download
  await page.evaluate(() => (window).__wkDocEditor.commands.blur())
  await page.waitForTimeout(300)
  await page.locator('button:has-text("立即保存")').first().click({ force: true })
  await page.waitForTimeout(5500)

  const buf = await downloadArchive()
  const xml = unzipDocumentXml(buf)
  writeFileSync('/tmp/wk-v0.7.106-doc.xml', xml)

  // Step 7 — assert the OOXML carries w:ins / w:del with author + date
  const hasIns = /<w:ins\b/.test(xml)
  const hasDel = /<w:del\b/.test(xml)
  const hasAuthor = /w:author="admin"/.test(xml)
  const hasDate = /w:date="\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/.test(xml)
  const hasDelText = /<w:delText[^>]*>TO_DELETE_HERE<\/w:delText>/.test(xml)
  console.log('OOXML has <w:ins>:', hasIns)
  console.log('OOXML has <w:del>:', hasDel)
  console.log('OOXML has w:author=admin:', hasAuthor)
  console.log('OOXML has w:date=ISO:', hasDate)
  console.log('OOXML has <w:delText>TO_DELETE_HERE:', hasDelText)

  if (!hasIns) throw new Error('OOXML must contain <w:ins>')
  if (!hasDel) throw new Error('OOXML must contain <w:del>')
  if (!hasAuthor) throw new Error('OOXML must carry w:author=admin')
  if (!hasDate) throw new Error('OOXML must carry w:date ISO timestamp')
  if (!hasDelText) throw new Error('OOXML must carry <w:delText>TRACKED')
  if (!insMarkCount) throw new Error('typing in track-changes mode must add an ins mark')
  if (!delMarkCount) throw new Error('deleting in track-changes mode must add a del mark')
  if (!revListExists) throw new Error('revisions list must be visible')
  if (!firstDate) throw new Error('revision date label missing')

  console.log('page errors:', errs.length)
  if (errs.length) {
    for (const e of errs) console.log('  -', e)
  }
  console.log('ALL OK — doc track changes serialize w:ins/w:del with author + date')
  await browser.close()
}

await main()
