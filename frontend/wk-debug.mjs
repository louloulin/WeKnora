import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  page.on('pageerror', (err) => console.log(`[pageerror]\n${err.stack || err.message}\n---`))
  page.on('console', (msg) => {
    if (msg.type() === 'error' || msg.type() === 'warning') {
      console.log(`[console.${msg.type()}] ${msg.text()}`)
    }
  })

  console.log('=== Login ===')
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(3000)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.waitForTimeout(500)
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(5000)
  console.log('URL:', page.url())

  console.log('=== Open form doc ===')
  await page.goto('http://127.0.0.1:5173/collab-documents/c7205330-41a0-417b-9c42-d5f864a5819a', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(8000)
  await page.screenshot({ path: '/tmp/wk-shots/debug-01.png', fullPage: true })
  console.log('URL:', page.url())

  const bodyHtml = await page.evaluate(() => {
    return document.querySelector('.collab-editor-view')?.outerHTML?.slice(0, 800) || 'NO .collab-editor-view'
  })
  console.log('Body html (first 800):')
  console.log(bodyHtml)

  const hasForm = await page.evaluate(() => {
    return {
      title: document.querySelector('.collab-form-editor__title')?.textContent || 'N/A',
      items: document.querySelectorAll('.collab-form-editor__item').length,
      sidebar: document.querySelector('.collab-editor-view__sidebar')?.textContent?.slice(0, 200) || 'NO sidebar',
      main: document.querySelector('.collab-editor-view__main')?.innerHTML?.slice(0, 500) || 'NO main',
    }
  })
  console.log('Form check:', JSON.stringify(hasForm, null, 2))

  await browser.close()
}
main().catch(e => { console.error('FAIL:', e.message, e.stack); process.exit(1) })
