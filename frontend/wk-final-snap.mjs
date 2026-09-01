import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1200 } })
  const page = await ctx.newPage()
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)
  await page.goto('http://127.0.0.1:5173/collab-documents/c7205330-41a0-417b-9c42-d5f864a5819a', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(5000)
  // dump structured DOM
  const dump = await page.evaluate(() => {
    function text(el) { return el ? el.textContent.replace(/\s+/g,' ').trim() : null }
    const items = [...document.querySelectorAll('.collab-form-editor__item')].map(it => ({
      kind: it.querySelector('.collab-form-editor__qkind')?.textContent || null,
      title: it.querySelector('.collab-form-editor__question-title')?.value || it.querySelector('.collab-form-editor__question-title')?.textContent || null,
      required: !!it.querySelector('input[type="checkbox"]:checked'),
      body: it.textContent?.replace(/\s+/g,' ').trim().slice(0, 200),
    }))
    return {
      title: text(document.querySelector('.collab-form-editor__title')),
      kind: text(document.querySelector('.collab-form-editor__kind')),
      saveTag: text(document.querySelector('.collab-form-editor__savetag')),
      items,
      sidebar: text(document.querySelector('.collab-editor-view__sidebar')),
    }
  })
  console.log(JSON.stringify(dump, null, 2))
  await page.screenshot({ path: '/tmp/wk-shots/final-full.png', fullPage: true })
  await browser.close()
}
main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
