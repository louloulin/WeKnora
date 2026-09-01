import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
const DOC_ID = '67fadefd-8f01-4f2b-aeab-a3ac3d050e39'
async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const page = await browser.newPage()
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)
  await page.goto(`http://127.0.0.1:5173/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  const r = await page.evaluate(() => {
    function cs(el, prop) { return el ? getComputedStyle(el).getPropertyValue(prop).trim() : null }
    function rect(el) {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: r.x, y: r.y, w: r.width, h: r.height }
    }
    const wrap = document.querySelector('.collab-doc-pro__surface-wrap')
    const surf = document.querySelector('.tiptap.ProseMirror')
    const pro = document.querySelector('.collab-doc-pro')
    const tb = document.querySelector('.collab-doc-pro__toolbar')
    return {
      pro: { ...rect(pro), display: cs(pro, 'display'), flexDir: cs(pro, 'flex-direction'), height: cs(pro, 'height') },
      toolbar: { ...rect(tb), flexShrink: cs(tb, 'flex-shrink'), height: cs(tb, 'height'), childCount: tb?.children?.length },
      wrap: { ...rect(wrap), display: cs(wrap, 'display'), flexDir: cs(wrap, 'flex-direction'), alignItems: cs(wrap, 'align-items'), flex: cs(wrap, 'flex'), minH: cs(wrap, 'min-height') },
      surface: { ...rect(surf), display: cs(surf, 'display'), flex: cs(surf, 'flex'), maxWidth: cs(surf, 'max-width'), margin: cs(surf, 'margin') },
    }
  })
  console.log(JSON.stringify(r, null, 2))
  await browser.close()
}
main().catch(e => { console.error(e.message); process.exit(1) })
