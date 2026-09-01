import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg
const DOC_ID = '67fadefd-8f01-4f2b-aeab-a3ac3d050e39'

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  const reqs = []
  page.on('request', (r) => {
    if (r.url().includes('collaborative-docs') && r.method() !== 'GET')
      reqs.push({ method: r.method(), url: r.url().slice(0, 120) })
  })
  page.on('response', async (r) => {
    if (r.url().includes('collaborative-docs') && r.request().method() !== 'GET')
      console.log(`  [resp] ${r.status()} ${r.request().method()} ${r.url().slice(0, 100)}`)
  })

  console.log('=== Login ===')
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(4000)

  console.log('=== Open DOC editor ===')
  await page.goto(`http://127.0.0.1:5173/collab-documents/${DOC_ID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(7000)

  const init = await page.evaluate(() => {
    const proEditor = document.querySelector('.collab-doc-pro')
    const tiptap = document.querySelector('.ProseMirror, .tiptap, [contenteditable="true"]')
    return {
      hasProEditor: !!proEditor,
      hasEditable: !!tiptap,
      editableTag: tiptap?.tagName || null,
      editableContent: tiptap?.textContent?.slice(0, 80) || '',
      proEditorChildren: proEditor?.children?.length || 0,
    }
  })
  console.log('  Init state:', JSON.stringify(init))
  await page.screenshot({ path: '/tmp/wk-shots/doc-init.png', fullPage: false })

  console.log('=== Type into doc ===')
  const editable = page.locator('.ProseMirror, .tiptap, [contenteditable="true"]').first()
  await editable.click()
  await page.waitForTimeout(300)
  await page.keyboard.press('End')
  await page.keyboard.type(' [E2E-EDIT]')
  await page.waitForTimeout(500)
  await page.screenshot({ path: '/tmp/wk-shots/doc-typed.png', fullPage: false })

  console.log('=== Wait for autosave ===')
  await page.waitForTimeout(5000)
  console.log('  mutations observed:', reqs.length)
  for (const r of reqs) console.log('   -', r.method, r.url)

  console.log('=== Verify via API download ===')
  const token = await page.evaluate(() => localStorage.getItem('weknora_token'))
  const r = await fetch(`http://127.0.0.1:5173/collaborative-docs/${DOC_ID}/download`, {
    headers: { 'Authorization': `Bearer ${token}` }
  })
  console.log('  status:', r.status, 'bytes:', (await r.arrayBuffer()).byteLength)

  await browser.close()
}
main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
