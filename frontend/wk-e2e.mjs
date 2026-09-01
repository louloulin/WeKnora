import pkg from '/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js'
const { chromium } = pkg

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  page.on('pageerror', (err) => console.log(`[pageerror] ${err.message}`))
  page.on('request', (req) => {
    if (req.url().includes('collaborative-docs') && req.method() !== 'GET') {
      console.log(`[req] ${req.method()} ${req.url().slice(0, 120)}`)
    }
  })
  page.on('response', async (resp) => {
    if (resp.url().includes('collaborative-docs') && resp.request().method() !== 'GET') {
      console.log(`[resp] ${resp.status()} ${resp.request().method()} ${resp.url().slice(0, 120)}`)
    }
  })

  // 1) Login with longer waits
  console.log('=== 1) Login ===')
  await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(3000)
  await page.locator('input[type="text"]').first().fill('admin@example.com')
  await page.locator('input[type="password"]').first().fill('Admin1234')
  await page.waitForTimeout(500)
  await page.locator('button:has-text("登录")').first().click()
  await page.waitForTimeout(8000)
  console.log('  URL after login:', page.url())
  if (!page.url().includes('/platform')) {
    console.log('  Login seems to have failed. Saving screenshot.')
    await page.screenshot({ path: '/tmp/wk-shots/login-fail.png' })
    return
  }

  // 2) Navigate to form doc
  console.log('=== 2) Open form doc ===')
  await page.goto('http://127.0.0.1:5173/collab-documents/c7205330-41a0-417b-9c42-d5f864a5819a', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(5000)
  await page.screenshot({ path: '/tmp/wk-shots/e2e-01-editor.png' })
  console.log('  URL:', page.url())

  // 3) Verify form editor
  const editorInfo = await page.evaluate(() => {
    const titleEl = document.querySelector('.collab-form-editor__title')
    const kindEl = document.querySelector('.collab-form-editor__kind')
    const items = document.querySelectorAll('.collab-form-editor__item')
    const buttons = document.querySelectorAll('.collab-form-editor__add-question')
    return {
      title: titleEl?.textContent || 'N/A',
      kind: kindEl?.textContent || 'N/A',
      itemCount: items.length,
      addButtonCount: buttons.length,
    }
  })
  console.log('  Editor state:', JSON.stringify(editorInfo))

  // 4) Add a single-choice question
  console.log('=== 3) Add a single-choice question ===')
  const addBtns = await page.locator('.collab-form-editor__add-question').all()
  if (addBtns.length >= 2) {
    await addBtns[1].click()
    await page.waitForTimeout(2500)
    
    const titles = await page.locator('.collab-form-editor__question-title').all()
    if (titles.length >= 2) {
      await titles[1].fill('你最喜欢的编程语言？')
      await page.waitForTimeout(500)
    }
    await page.screenshot({ path: '/tmp/wk-shots/e2e-02-single.png' })
  }

  // 5) Add a rating question
  console.log('=== 4) Add a rating question ===')
  const addBtns2 = await page.locator('.collab-form-editor__add-question').all()
  if (addBtns2.length >= 4) {
    await addBtns2[3].click()
    await page.waitForTimeout(2500)
    
    const titles = await page.locator('.collab-form-editor__question-title').all()
    if (titles.length >= 3) {
      await titles[2].fill('请给本次会议打分')
      await page.waitForTimeout(500)
    }
  }

  // 6) Wait for autosave
  console.log('=== 5) Wait for autosave (3s) ===')
  await page.waitForTimeout(3500)
  await page.screenshot({ path: '/tmp/wk-shots/e2e-03-final.png' })
  
  const saveLabel = await page.locator('.collab-form-editor__savetag').first().textContent().catch(() => 'N/A')
  console.log('  Save status:', saveLabel)

  // 7) Verify via API
  console.log('=== 6) Verify via API ===')
  const token = await page.evaluate(() => localStorage.getItem('weknora_token'))
  console.log('  Token:', (token || 'N/A').slice(0, 30))
  
  const dl = await fetch('http://127.0.0.1:5173/collaborative-docs/c7205330-41a0-417b-9c42-d5f864a5819a/download', {
    headers: { 'Authorization': `Bearer ${token}` }
  })
  console.log('  Download status:', dl.status)
  if (dl.ok) {
    const text = await dl.text()
    console.log('  Content (first 800 chars):')
    console.log(text.slice(0, 800))
  } else {
    console.log('  Download failed:', await dl.text())
  }

  await browser.close()
}

main().catch(e => { console.error('FAIL:', e.message); process.exit(1) })
