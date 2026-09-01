/**
 * wk-slide-theme.mjs — verify the v0.7.92 SLIDE theme gallery renders
 * in the /collab-slides view and the theme panel exposes the 8 OOXML
 * scheme presets vendored from genoffice.
 */
import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

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
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  page.on('pageerror', (e) => console.log('[pageerror]', e.message))

  await login(page)
  await page.goto(`${BASE}/collab-slides`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)
  await page.screenshot({ path: '/tmp/wk-shots/slide-theme-gallery.png', fullPage: true })

  const panelVisible = await page.locator('[data-testid="slide-theme-panel"]').count()
  const themeButtons = await page.locator('[data-testid^="slide-theme-"]').count()
  const themeNames = await page.evaluate(() => {
    return Array.from(document.querySelectorAll('[data-testid^="slide-theme-"]'))
      .map((el) => el.getAttribute('data-testid'))
  })
  console.log('theme panel visible:', panelVisible)
  console.log('theme button count:', themeButtons)
  console.log('theme button ids:', themeNames)

  // Click "indigo" to test the apply event fires
  let eventFired = false
  await page.exposeFunction('recordEvent', () => { eventFired = true })
  await page.evaluate(() => {
    window.addEventListener('wk-slide-theme-apply', () => {
      // @ts-ignore
      window.recordEvent()
    })
  })
  if (themeButtons >= 3) {
    await page.locator('[data-testid="slide-theme-indigo"]').click()
    await page.waitForTimeout(800)
    console.log('wk-slide-theme-apply event fired:', eventFired)
    await page.screenshot({ path: '/tmp/wk-shots/slide-theme-indigo.png', fullPage: true })
  }

  await browser.close()
  const ok = panelVisible === 1 && themeButtons >= 8 && eventFired
  console.log(ok ? '\nALL OK — slide theme gallery renders + apply event' : '\nFAIL')
  process.exit(ok ? 0 : 2)
}
main().catch((e) => { console.error('FAIL:', e); process.exit(1) })
