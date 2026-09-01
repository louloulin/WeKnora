import pkg from "/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js"
const { chromium } = pkg
const BASE = "http://127.0.0.1:5173"
const DOC_ID = "67fadefd-8f01-4f2b-aeab-a3ac3d050e39"

async function login(context, name) {
  const page = await context.newPage()
  await page.goto(`${BASE}/login`, { waitUntil: "domcontentloaded" })
  await page.waitForTimeout(1500)
  await page.locator("input[type=text]").first().fill("admin@example.com")
  await page.locator("input[type=password]").first().fill("Admin1234")
  await page.getByRole("button", { name: "登录" }).first().click()
  await page.waitForTimeout(3500)
  console.log(`${name} login: ${page.url()}`)
  return page
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] })
  const contextA = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const contextB = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const [pageA, pageB] = await Promise.all([login(contextA, "A"), login(contextB, "B")])
  const errors = []
  for (const [page, label] of [[pageA, "A"], [pageB, "B"]]) {
    page.on("pageerror", (error) => { errors.push(`${label}: ${error.message}`); console.log(`[${label} pageerror]`, error.message) })
    page.on("websocket", (socket) => console.log(`[${label} websocket]`, socket.url()))
  }
  await Promise.all([
    pageA.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: "domcontentloaded" }),
    pageB.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: "domcontentloaded" }),
  ])
  await Promise.all([pageA.waitForTimeout(7500), pageB.waitForTimeout(7500)])
  const initial = await Promise.all([pageA, pageB].map(async (page) => page.evaluate(() => ({
    connection: document.querySelector(".collab-doc-pro__connection")?.textContent?.trim(),
    peers: document.querySelectorAll(".collab-doc-pro__peer").length,
    editable: !!document.querySelector(".collab-doc-pro__surface[contenteditable=true], .collab-doc-pro .ProseMirror"),
    text: document.querySelector(".collab-doc-pro .ProseMirror")?.textContent || "",
  }))))
  console.log("initial:", JSON.stringify(initial))
  const editableA = pageA.locator(".collab-doc-pro .ProseMirror").first()
  await editableA.click()
  await pageA.keyboard.press("End")
  const marker = ` [CRDT-E2E-${Date.now()}]`
  await pageA.keyboard.type(marker)
  await pageB.waitForTimeout(4000)
  const remote = await pageB.locator(".collab-doc-pro .ProseMirror").textContent()
  console.log("remote contains marker:", remote?.includes(marker), "marker:", marker)
  console.log("peer counts:", await pageA.locator(".collab-doc-pro__peer").count(), await pageB.locator(".collab-doc-pro__peer").count())

  const syncResponses = []
  pageA.on("response", async (response) => {
    if (response.url().includes(`/collaborative-docs/${DOC_ID}/sync-to-kb`)) {
      syncResponses.push({ status: response.status(), body: (await response.text()).slice(0, 600) })
    }
  })
  await pageA.getByRole("button", { name: "同步到知识库" }).click()
  await pageA.waitForTimeout(2500)
  console.log("sync responses:", JSON.stringify(syncResponses))
  await pageA.screenshot({ path: "/tmp/wk-shots/collab-doc-a.png", fullPage: false })
  await pageB.screenshot({ path: "/tmp/wk-shots/collab-doc-b.png", fullPage: false })
  console.log("page errors:", JSON.stringify(errors))
  if (!remote?.includes(marker)) throw new Error("Second client did not receive CRDT marker")
  if (!syncResponses.length) throw new Error("No sync-to-kb response observed")
  if (errors.length) throw new Error(`page errors: ${errors.join("; ")}`)
  await browser.close()
}
main().catch((error) => { console.error("FAIL:", error.stack || error.message); process.exit(1) })
