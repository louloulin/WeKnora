import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

const DOC_ID = '323ba255-b77a-4bfe-b9f1-c90e02151700'
const BASE = 'http://127.0.0.1:5173'

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForTimeout(3500)
}

async function openClient(context, label) {
  const page = await context.newPage()
  page.on('pageerror', (error) => console.log(`[${label} pageerror]`, error.message))
  await login(page)
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  return page
}

const sheetTab = (page, name) => page.locator(`.collab-sheet-editor__tab[title="${name}"]`)
const cell = (page, row, column) => page.locator(`input[data-cell="${row}-${column}"]`)

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const aliceContext = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const bobContext = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const [alice, bob] = await Promise.all([openClient(aliceContext, 'alice'), openClient(bobContext, 'bob')])
  await alice.waitForTimeout(5000)

  await sheetTab(alice, 'Sheet2').click()
  await cell(alice, 0, 0).fill('Alice-S2')
  await cell(alice, 0, 0).press('Enter')
  await sheetTab(bob, 'Sheet2').click()
  await bob.waitForTimeout(8000)
  const sheet2Value = await cell(bob, 0, 0).inputValue()

  await sheetTab(alice, 'Sheet1').click()
  await sheetTab(bob, 'Sheet1').click()
  const sheet1Value = await cell(bob, 0, 0).inputValue()

  await alice.screenshot({ path: '/tmp/wk-shots/sheet-cells-alice.png' })
  await bob.screenshot({ path: '/tmp/wk-shots/sheet-cells-bob.png' })
  await browser.close()

  const ok = sheet2Value === 'Alice-S2' && sheet1Value === ''
  console.log({ sheet2Value, sheet1Value, ok })
  console.log(ok ? 'ALL OK -- per-sheet Yjs cells sync' : 'FAIL')
  process.exit(ok ? 0 : 2)
}

main().catch((error) => {
  console.error('FAIL:', error.stack || error.message)
  process.exit(1)
})
