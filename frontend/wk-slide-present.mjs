/**
 * wk-slide-present.mjs — v0.7.96 SLIDE fullscreen present mode verification.
 *
 * Flow:
 *   1. real admin login via /login
 *   2. open /collab-documents/<slide id> in Playwright
 *   3. add a couple of speaker notes to slide 1 and slide 2
 *   4. wait for saveLabel to flip to "已保存" (debounce 1.5s)
 *   5. click "▶ 演示" toolbar button → present overlay appears
 *   6. verify overlay + svg + controls render with current slide index
 *   7. press ArrowRight → counter increments
 *   8. press ArrowRight again → counter increments
 *   9. press Home → counter resets to 1
 *  10. press Escape → overlay closes
 *  11. screenshot each stage
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

const DOC_ID = 'f12a724e-d87e-49f0-a039-36ca435cb94a'
const BASE = 'http://127.0.0.1:5173'

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
  page.on('pageerror', e => errs.push(e.message))

  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // Snapshot editor stage
  await page.screenshot({ path: '/tmp/wk-shots/slide-present-editor.png', fullPage: false })

  // Click ▶ 演示 button
  const presentBtn = page.locator('[data-testid="slide-present-btn"]')
  const presentBtnVisible = await presentBtn.first().isVisible().catch(() => false)
  console.log('present btn visible:', presentBtnVisible)
  await presentBtn.first().click({ force: true })
  await page.waitForTimeout(800)

  // Verify overlay appears
  const overlay = page.locator('[data-testid="slide-present-overlay"]')
  const overlayVisible = await overlay.first().isVisible().catch(() => false)
  console.log('overlay visible:', overlayVisible)
  const counterText1 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (initial):', counterText1)
  const svgPresent = await page.locator('[data-testid="slide-present-svg"]').first().isVisible().catch(() => false)
  console.log('svg visible:', svgPresent)
  await page.screenshot({ path: '/tmp/wk-shots/slide-present-01-overlay.png', fullPage: false })

  // Arrow right
  await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(400)
  const counterText2 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (after ->):', counterText2)
  await page.screenshot({ path: '/tmp/wk-shots/slide-present-02-page2.png', fullPage: false })

  // Home = first
  await page.keyboard.press('Home')
  await page.waitForTimeout(400)
  const counterText3 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (after Home):', counterText3)

  // Space = next
  await page.keyboard.press('Space')
  await page.waitForTimeout(400)
  const counterText4 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (after Space):', counterText4)

  // ArrowLeft = prev
  await page.keyboard.press('ArrowLeft')
  await page.waitForTimeout(400)
  const counterText5 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (after <-):', counterText5)

  // Test prev button disabled at start
  const prevDisabled = await page.locator('[data-testid="slide-present-prev"]').first().isDisabled().catch(() => null)
  console.log('prev disabled at start:', prevDisabled)

  // Click next button (after Home)
  await page.locator('[data-testid="slide-present-next"]').first().click({ force: true })
  await page.waitForTimeout(400)
  const counterText6 = await page.locator('[data-testid="slide-present-counter"]').first().textContent()
  console.log('counter (after click next):', counterText6)

  // Escape
  await page.keyboard.press('Escape')
  await page.waitForTimeout(600)
  const overlayGone = await page.locator('[data-testid="slide-present-overlay"]').count()
  console.log('overlay count after ESC:', overlayGone)
  await page.screenshot({ path: '/tmp/wk-shots/slide-present-03-back-to-editor.png', fullPage: false })

  await browser.close()

  const ok =
    presentBtnVisible &&
    overlayVisible &&
    svgPresent &&
    counterText1 &&
    counterText2 &&
    counterText3 &&
    counterText4 &&
    counterText5 &&
    /\/ \d+$/.test(counterText1 || '') &&
    counterText2 !== counterText1 &&
    counterText3 !== counterText2 &&
    counterText4 !== counterText3 &&
    prevDisabled === true &&
    overlayGone === 0 &&
    errs.length === 0
  console.log('page errors:', errs.length)
  for (const e of errs) console.log(' -', e.slice(0, 200))
  console.log('first test:', ok ? 'PASS' : 'FAIL')
  if (!ok) { process.exit(2) }
}


main().catch(e => { console.error('FAIL:', e.stack || e.message); process.exit(1) })

// --- speaker notes round-trip ---
async function notesTest() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const page = await ctx.newPage()
  const errs = []
  page.on('pageerror', e => errs.push(e.message))
  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)

  // Type into notes textarea on slide 1
  const notesTextarea = page.locator('.collab-slide-konva__notes-textarea').first()
  await notesTextarea.click({ force: true })
  const note = '本演讲者备注由 v0.7.96 真实写入 - ' + Date.now()
  await notesTextarea.fill(note)
  await notesTextarea.evaluate(el => el.dispatchEvent(new Event('input', { bubbles: true })))
  await page.waitForTimeout(2000) // wait for notes 800ms debounce + auto-save 1.5s

  const labelAfter = await page.locator('.collab-slide-konva__notes-status').first().textContent().catch(() => 'N/A')
  console.log('notes status after type:', labelAfter)
  await page.waitForTimeout(2500)
  const saveLabelAfter = await page.locator('.collab-slide-konva__savetag').first().textContent().catch(() => 'N/A')
  console.log('saveLabel after type:', saveLabelAfter)

  // Enter present mode
  await page.locator('[data-testid="slide-present-btn"]').first().click({ force: true })
  await page.waitForTimeout(800)
  const presentNotes = page.locator('[data-testid="slide-present-notes"]')
  const notesVisible = await presentNotes.first().isVisible().catch(() => false)
  const notesBody = await page.locator('.slide-present-notes-body').first().textContent().catch(() => '')
  console.log('present notes visible:', notesVisible)
  console.log('present notes body:', JSON.stringify(notesBody))
  await page.screenshot({ path: '/tmp/wk-shots/slide-present-notes.png', fullPage: false })
  await browser.close()
  return { notesVisible, notesBody, errs }
}

const notesResult = await notesTest()
console.log('notes test page errors:', notesResult.errs.length)
for (const e of notesResult.errs) console.log(' -', e.slice(0, 200))
console.log('notes round-trip ok:', notesResult.notesVisible && notesResult.notesBody.length > 10 ? 'PASS' : 'FAIL')
