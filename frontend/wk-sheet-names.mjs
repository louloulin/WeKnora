import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
const SHEET_DOC_ID = '323ba255-b77a-4bfe-b9f1-c90e02151700'
const BASE = 'http://127.0.0.1:5173'
async function login(page) {
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(3500)
}
async function openSheet(ctx, label) {
  const page = await ctx.newPage()
  page.on('pageerror', (e) => console.log('[' + label + ' pageerror]', e.message))
  await login(page)
  await page.goto(BASE + '/collab-documents/' + SHEET_DOC_ID, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  return page
}
async function sheetNames(page) {
  return page.evaluate(() => {
    const tabs = Array.from(document.querySelectorAll('.collab-sheet-editor__tab'))
    return tabs.map((el) => (el.textContent || '').replace(/\s+/g, ' ').trim())
  })
}
async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctxA = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const ctxB = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const [alice, bob] = await Promise.all([openSheet(ctxA, 'alice'), openSheet(ctxB, 'bob')])
  await alice.locator('.collab-sheet-editor__peer').first().waitFor({ state: 'attached', timeout: 10000 })
  const aliceBefore = await sheetNames(alice)
  const bobBefore = await sheetNames(bob)
  console.log('alice before:', aliceBefore)
  console.log('bob   before:', bobBefore)
  await alice.locator('.collab-sheet-editor__tab[title="新增 sheet"]').click()
  await alice.waitForTimeout(3000)
  const aliceAfterAdd = await sheetNames(alice)
  console.log('alice after add:', aliceAfterAdd)
  await alice.screenshot({ path: '/tmp/wk-shots/sheet-alice-add.png' })
  await bob.waitForTimeout(10000)
  const bobAfterAdd = await sheetNames(bob)
  console.log('bob   after add:', bobAfterAdd)
  await bob.screenshot({ path: '/tmp/wk-shots/sheet-bob-add.png' })
  await browser.close()
  const before = JSON.stringify(aliceBefore) === JSON.stringify(bobBefore)
  const addedOk = aliceAfterAdd.length === aliceBefore.length + 1
  const bobSawAdd = JSON.stringify(bobAfterAdd) === JSON.stringify(aliceAfterAdd)
  console.log({ before, addedOk, bobSawAdd })
  const ok = before && addedOk && bobSawAdd
  console.log(ok ? 'ALL OK -- sheet names sync between collaborators' : 'FAIL')
  process.exit(ok ? 0 : 2)
}
main().catch((e) => { console.error('FAIL:', e); process.exit(1) })
