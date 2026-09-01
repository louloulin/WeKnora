import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
const DOC_ID = '67fadefd-8f01-4f2b-aeab-a3ac3d050e39'

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  page.on('pageerror', (e) => console.log(`[pageerror] ${e.message}`))

  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)

  await page.goto(`http://127.0.0.1:5173/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)

  const dump = await page.evaluate(() => {
    function rect(el) {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: r.x, y: r.y, w: r.width, h: r.height, visible: r.width > 0 && r.height > 0 }
    }
    function cs(el, prop) {
      if (!el) return null
      return getComputedStyle(el).getPropertyValue(prop).trim()
    }
    const surface = document.querySelector('.tiptap.ProseMirror')
    const wrap = document.querySelector('.collab-doc-pro__surface-wrap')
    const pro = document.querySelector('.collab-doc-pro')
    const main = document.querySelector('.collab-editor-view__main')
    return {
      hasSurface: !!surface,
      surface: rect(surface),
      surfaceDisplay: cs(surface, 'display'),
      surfaceVisibility: cs(surface, 'visibility'),
      surfaceOpacity: cs(surface, 'opacity'),
      surfaceHTML: surface?.outerHTML?.slice(0, 200),
      wrap: rect(wrap),
      wrapDisplay: cs(wrap, 'display'),
      pro: rect(pro),
      proDisplay: cs(pro, 'display'),
      main: rect(main),
      mainDisplay: cs(main, 'display'),
      bodyHTMLHeight: document.body.offsetHeight,
      htmlScrollHeight: document.documentElement.scrollHeight,
    }
  })
  console.log(JSON.stringify(dump, null, 2))
  await page.screenshot({ path: '/tmp/wk-shots/doc-debug.png', fullPage: true })

  await browser.close()
}
main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
