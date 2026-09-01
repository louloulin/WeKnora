import pkg from "/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js"
const { chromium } = pkg
const BASE = "http://127.0.0.1:5173"
const DOC_ID = "f12a724e-d87e-49f0-a039-36ca435cb94a"
async function login(context, name) {
  const page = await context.newPage()
  await page.goto(`${BASE}/login`, { waitUntil: "domcontentloaded" })
  await page.waitForTimeout(1500)
  await page.locator("input[type=text]").first().fill("admin@example.com")
  await page.locator("input[type=password]").first().fill("Admin1234")
  await page.getByRole("button", { name: "登录" }).first().click()
  await page.waitForTimeout(3000)
  console.log(`${name} login: ${page.url()}`)
  return page
}
async function main() {
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] })
  const aCtx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const bCtx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const [a, b] = await Promise.all([login(aCtx, "A"), login(bCtx, "B")])
  const errors = []
  for (const [page, label] of [[a, "A"], [b, "B"]]) {
    page.on("pageerror", e => { errors.push(`${label}: ${e.message}`); console.log(`[${label} pageerror]`, e.message) })
    page.on("websocket", s => { if (s.url().includes("collaborative-docs")) console.log(`[${label} websocket] connected`) })
  }
  await Promise.all([
    a.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: "domcontentloaded" }),
    b.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: "domcontentloaded" }),
  ])
  await Promise.all([a.waitForTimeout(8500), b.waitForTimeout(8500)])
  const initial = await Promise.all([a, b].map(page => page.evaluate(() => ({
    canvas: document.querySelectorAll("canvas").length,
    slides: document.querySelectorAll(".collab-slide-konva__thumb").length,
    connection: document.querySelector(".collab-slide-konva__connection")?.textContent?.trim(),
    peers: document.querySelectorAll(".collab-slide-konva__peer").length,
    recovery: document.querySelector(".collab-slide-konva__recovery")?.textContent?.trim() || "",
  }))))
  console.log("initial:", JSON.stringify(initial))
  if (!initial.every(x => x.canvas && x.slides)) throw new Error("slide editor did not render for both clients")
  await a.getByTestId("slide-add-text").click()
  await a.waitForTimeout(1200)
  const peerSeen = await b.locator(".collab-slide-konva__peer").count()
  const bSlides = await b.locator(".collab-slide-konva__thumb").count()
  const bCanvas = await b.locator("canvas").count()
  console.log("after A add: B peers=", peerSeen, "slides=", bSlides, "canvas=", bCanvas)
  const syncResponses = []
  a.on("response", async r => {
    if (r.url().includes(`/collaborative-docs/${DOC_ID}/sync-to-kb`)) syncResponses.push({ status: r.status(), body: (await r.text()).slice(0, 600) })
  })
  await a.getByRole("button", { name: "同步到知识库" }).click()
  await a.waitForTimeout(2500)
  console.log("sync responses:", JSON.stringify(syncResponses))
  await a.screenshot({ path: "/tmp/wk-shots/slide-collab-a.png", fullPage: false })
  await b.screenshot({ path: "/tmp/wk-shots/slide-collab-b.png", fullPage: false })
  console.log("errors:", JSON.stringify(errors))
  if (!peerSeen) throw new Error("B did not observe A awareness peer")
  if (!syncResponses.length) throw new Error("No sync-to-kb response")
  if (errors.length) throw new Error(errors.join("; "))
  await browser.close()
}
main().catch(e => { console.error("FAIL:", e.stack || e.message); process.exit(1) })
