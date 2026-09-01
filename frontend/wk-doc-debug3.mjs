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
    const pro = document.querySelector('.collab-doc-pro')
    return {
      proOuterHTML: pro?.outerHTML?.slice(0, 800),
      proInnerChildren: [...(pro?.children ?? [])].map(c => ({
        tag: c.tagName,
        cls: c.className,
        rect: c.getBoundingClientRect().toJSON(),
      })),
      proComputed: {
        height: getComputedStyle(pro).height,
        display: getComputedStyle(pro).display,
        flexDirection: getComputedStyle(pro).flexDirection,
      }
    }
  })
  console.log(JSON.stringify(r, null, 2))
  await browser.close()
}
main().catch(e => { console.error(e.message); process.exit(1) })
