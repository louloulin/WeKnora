/**
 * wk-form-public.mjs — public form responder end-to-end test.
 *
 * 1. Open the public form page (no auth) at /form/<token>
 * 2. Verify the schema loads (questions rendered)
 * 3. Fill in text + rating + multi
 * 4. Submit and verify thanks message
 * 5. Re-login as owner and verify response list shows the new submission
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

const TOKEN = '9e65002867a11ce3ef58049e23e88462'
const DOC_ID = 'c7205330-41a0-417b-9c42-d5f864a5819a'
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

  // Public anonymous submit
  const ctxA = await browser.newContext({ viewport: { width: 1024, height: 800 } })
  const pageA = await ctxA.newPage()
  pageA.on('pageerror', (e) => console.log('[anon pageerror]', e.message))
  await pageA.goto(`${BASE}/form/${TOKEN}`, { waitUntil: 'domcontentloaded' })
  await pageA.waitForTimeout(3000)
  const schema = await pageA.evaluate(() => ({
    url: location.href,
    title: document.querySelector('h2')?.textContent?.trim() || '',
    itemCount: document.querySelectorAll('[data-testid^="responder-q-"]').length,
  }))
  console.log('anon schema:', JSON.stringify(schema))
  await pageA.screenshot({ path: '/tmp/wk-shots/form-anon-loaded.png' })

  // Fill the first text question
  const firstInput = pageA.locator('[data-testid="responder-q-0-input"]').first()
  if (await firstInput.count()) {
    await firstInput.fill('Tencent Docs parity via Playwright')
    console.log('  typed text')
  } else {
    console.log('  WARN: q0 text input not found')
  }

  // Click 5-star rating (q-1)
  const ratingStar = pageA.locator('[data-testid="responder-q-1-star-5"]').first()
  if (await ratingStar.count()) {
    await ratingStar.click()
    console.log('  clicked 5-star')
  }

  // Submit
  const submitBtn = pageA.locator('[data-testid="responder-submit"]').first()
  await submitBtn.click()
  await pageA.waitForTimeout(2500)
  await pageA.screenshot({ path: '/tmp/wk-shots/form-anon-thanks.png' })
  const thanks = await pageA.evaluate(() => ({
    thanksVisible: !!document.querySelector('[data-testid="responder-thanks"]'),
    error: document.querySelector('[data-testid="responder-error"]')?.textContent?.trim() || '',
  }))
  console.log('anon submit:', JSON.stringify(thanks))

  // Owner-side: login + open form editor + verify response in list
  const ctxB = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const pageB = await ctxB.newPage()
  pageB.on('pageerror', (e) => console.log('[owner pageerror]', e.message))
  await login(pageB)
  await pageB.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await pageB.waitForTimeout(7000)
  // Open responses panel
  const responsesBtn = pageB.locator('[data-testid="form-responses-btn"]').first()
  await responsesBtn.click()
  await pageB.waitForTimeout(2000)
  await pageB.screenshot({ path: '/tmp/wk-shots/form-responses-owner.png' })
  const listInfo = await pageB.evaluate(() => ({
    panelVisible: !!document.querySelector('[data-testid="form-responses-panel"]'),
    rowCount: document.querySelectorAll('[data-testid^="response-row-"]').length,
    rows: Array.from(document.querySelectorAll('[data-testid^="response-row-"]'))
      .slice(0, 3)
      .map((el) => el.textContent?.trim().slice(0, 80) || ''),
  }))
  console.log('owner list:', JSON.stringify(listInfo))

  // Switch to summary tab
  const summaryBtn = pageB.locator('[data-testid="responses-tab-summary"]').first()
  await summaryBtn.click()
  await pageB.waitForTimeout(2000)
  const summary = await pageB.evaluate(() => ({
    summaryVisible: !!document.querySelector('[data-testid="responses-summary"]'),
    totalText: document.querySelector('[data-testid="responses-summary"] p')?.textContent?.trim() || '',
  }))
  console.log('owner summary:', JSON.stringify(summary))
  await pageB.screenshot({ path: '/tmp/wk-shots/form-responses-summary.png' })

  await browser.close()
  const ok = thanks.thanksVisible && listInfo.panelVisible && listInfo.rowCount >= 1
  console.log(ok ? '\nALL OK' : '\nFAIL')
  process.exit(ok ? 0 : 2)
}
main().catch((e) => { console.error('FAIL:', e); process.exit(1) })
