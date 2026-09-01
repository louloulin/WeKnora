import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
const DOC_ID = 'd4eca3d9-77fd-4f81-9746-99e1c4b2f44f'

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const page = await browser.newPage()
  page.on('pageerror', (e) => console.log(`[pageerror] ${e.message}`))
  page.on('response', async (r) => {
    if (r.url().includes('collaborative-docs') && r.request().method() !== 'GET')
      console.log(`  [resp] ${r.status()} ${r.request().method()} ${r.url().slice(0, 100)}`)
  })

  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)

  await page.goto(`http://127.0.0.1:5173/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)

  console.log('=== Click + 列 button (triggers scheduleSave) ===')
  await page.locator('.collab-sheet-editor__add-col').first().click()
  await page.waitForTimeout(500)
  await page.screenshot({ path: '/tmp/wk-shots/sheet-after-click.png', fullPage: false })
  console.log('  Waiting 3s for autosave debounce (1500ms)...')
  await page.waitForTimeout(3500)
  await page.screenshot({ path: '/tmp/wk-shots/sheet-save-final.png', fullPage: false })
  console.log('=== saveLabel text ===')
  const labelText = await page.locator('.collab-sheet-editor__savetag').first().textContent().catch(() => 'N/A')
  console.log('  savetag:', labelText)
  await browser.close()
}
main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
