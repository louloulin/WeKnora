import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

const DOCS = {
  doc:   '67fadefd-8f01-4f2b-aeab-a3ac3d050e39',
  sheet: 'd4eca3d9-77fd-4f81-9746-99e1c4b2f44f',
  slide: 'f12a724e-d87e-49f0-a039-36ca435cb94a',
  form:  'c7205330-41a0-417b-9c42-d5f864a5819a',
}

async function checkKind(page, kind) {
  const docId = DOCS[kind]
  console.log(`\n=== ${kind.toUpperCase()} (${docId}) ===`)
  const errs = []
  page.on('pageerror', (e) => errs.push(e.message))
  await page.goto(`http://127.0.0.1:5173/collab-documents/${docId}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)
  await page.screenshot({ path: `/tmp/wk-shots/kind-${kind}-01.png`, fullPage: false })
  const info = await page.evaluate((k) => {
    return {
      url: location.href,
      hasDocEditor: !!document.querySelector('.collab-doc-editor, [class*="doc-editor"], [class*="doc-pro"]'),
      hasSheetEditor: !!document.querySelector('.collab-sheet-editor, [class*="sheet-editor"]'),
      hasSlideEditor: !!document.querySelector('.collab-slide-editor, [class*="slide"], canvas'),
      hasFormEditor: !!document.querySelector('.collab-form-editor'),
      sidebar: document.querySelector('.collab-editor-view__sidebar')?.textContent?.replace(/\s+/g,' ').trim().slice(0, 100) || 'NONE',
      mainTagName: document.querySelector('.collab-editor-view__main')?.firstElementChild?.className || 'NONE',
      bodyText: document.body.textContent?.replace(/\s+/g,' ').trim().slice(0, 300) || '',
    }
  }, kind)
  console.log('  URL:', info.url)
  console.log('  Main element:', info.mainTagName)
  console.log('  Sidebar:', info.sidebar)
  console.log('  Body (first 200):', info.bodyText.slice(0, 200))
  console.log('  page errors:', errs.length)
  for (const e of errs) console.log('   -', e.slice(0, 200))
  return { kind, ok: errs.length === 0 && info.mainTagName !== 'NONE', info, errs }
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  page.on('console', (msg) => {
    if (msg.type() === 'error') console.log('[console.error]', msg.text().slice(0, 150))
  })

  console.log('=== Login ===')
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)
  console.log('  URL:', page.url())

  const results = []
  for (const kind of ['doc', 'sheet', 'slide', 'form']) {
    results.push(await checkKind(page, kind))
  }

  console.log('\n=== SUMMARY ===')
  for (const r of results) {
    console.log(`  ${r.kind.padEnd(6)} ok=${r.ok} main=${r.info.mainTagName.slice(0,60)} errs=${r.errs.length}`)
  }

  await browser.close()
}

main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
