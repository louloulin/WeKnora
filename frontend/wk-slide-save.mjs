import pkg from "/Users/louloulin/.npm-global/lib/node_modules/playwright/index.js"
import { execFileSync } from "node:child_process"
const { chromium } = pkg

const BASE = "http://127.0.0.1:5173"
const DOC_ID = "f12a724e-d87e-49f0-a039-36ca435cb94a"
const DB = "/Users/louloulin/appx/WeKnora/data/weknora.db"

function dbVersions() {
  try {
    return execFileSync("sqlite3", ["-header", "-column", DB, `SELECT doc_id, version, size_bytes, format FROM collab_doc_files WHERE doc_id = '${DOC_ID}' ORDER BY version;`], { encoding: "utf8" }).trim()
  } catch (error) {
    return `sqlite3 unavailable: ${error.message}`
  }
}

async function main() {
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] })
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  const pageErrors = []
  const saves = []
  page.on("pageerror", (error) => {
    pageErrors.push(error.message)
    console.log(`[pageerror] ${error.message}`)
  })
  page.on("console", (message) => {
    if (message.type() === "error") console.log(`[console.error] ${message.text()}`)
  })
  page.on("request", (request) => {
    if (request.url().includes("collaborative-docs")) console.log(`[request] ${request.method()} auth=${request.headers().authorization || "none"} tenant=${request.headers()["x-tenant-id"] || "none"}`)
  })
  page.on("response", async (response) => {
    if (response.url().includes(`/collaborative-docs/${DOC_ID}/upload`)) {
      saves.push({ status: response.status(), method: response.request().method(), url: response.url() })
      console.log(`[upload] ${response.status()} ${response.request().method()} ${response.status() >= 400 ? (await response.text()).slice(0, 300) : ""}`)
    }
  })

  console.log("=== Login ===")
  await page.goto(`${BASE}/login`, { waitUntil: "domcontentloaded" })
  await page.waitForTimeout(1500)
  await page.locator("input[type=text]").first().fill("admin@example.com")
  await page.locator("input[type=password]").first().fill("Admin1234")
  await page.getByRole("button", { name: "登录" }).first().click()
  await page.waitForTimeout(3500)
  console.log(`login url: ${page.url()}`)
  console.log("storage:", await page.evaluate(() => ({ token: (localStorage.getItem("weknora_token") || "").slice(0, 20), tokenLength: (localStorage.getItem("weknora_token") || "").length, tenant: localStorage.getItem("weknora_selected_tenant_id"), keys: Object.keys(localStorage) })))

  console.log("=== Open slide ===")
  await page.goto(`${BASE}/collab-documents/${DOC_ID}`, { waitUntil: "domcontentloaded" })
  await page.waitForTimeout(7000)
  await page.screenshot({ path: "/tmp/wk-shots/slide-init.png", fullPage: false })
  const initial = await page.evaluate(() => ({
    title: document.querySelector(".collab-slide-konva__title")?.textContent?.trim(),
    save: document.querySelector(".collab-slide-konva__savetag")?.textContent?.trim(),
    slides: document.querySelectorAll(".collab-slide-konva__thumb").length,
    canvas: document.querySelectorAll("canvas").length,
    buttons: [...document.querySelectorAll("button")].map((button) => button.textContent?.trim()).filter(Boolean).slice(0, 30),
  }))
  console.log("initial:", JSON.stringify(initial))
  if (!initial.canvas) throw new Error("Konva canvas did not render")

  console.log("=== Add text, rect, rotate, transition, slide ===")
  await page.getByTestId("slide-add-text").click()
  await page.waitForTimeout(250)
  await page.getByTestId("slide-add-rect").click()
  await page.waitForTimeout(250)
  const rotate = page.getByTestId("slide-rotate-cw")
  if (await rotate.isEnabled()) await rotate.click()
  const transition = page.locator('select[title="幻灯片切换效果"]')
  if (await transition.count()) {
    await transition.selectOption("fade")
    await page.waitForTimeout(250)
  }
  await page.getByRole("button", { name: "+ 新建幻灯片" }).click()
  await page.waitForTimeout(350)
  await page.screenshot({ path: "/tmp/wk-shots/slide-after-edit.png", fullPage: false })
  console.log("edited save tag:", await page.locator(".collab-slide-konva__savetag").textContent())
  await page.waitForTimeout(4500)
  await page.screenshot({ path: "/tmp/wk-shots/slide-save-final.png", fullPage: false })
  console.log("final save tag:", await page.locator(".collab-slide-konva__savetag").textContent())
  console.log("save error:", await page.locator(".collab-slide-konva__error").textContent().catch(() => ""))
  console.log("slide thumbnails:", await page.locator(".collab-slide-konva__thumb").count())
  console.log("uploads:", JSON.stringify(saves))
  console.log("page errors:", pageErrors.length)
  console.log("db versions:\n" + dbVersions())
  if (!saves.some((save) => save.status === 201)) throw new Error("No successful slide upload observed")
  if (pageErrors.length) throw new Error(`Page errors: ${pageErrors.join("; ")}`)
  await browser.close()
}

main().catch((error) => {
  console.error("FAIL:", error.stack || error.message)
  process.exit(1)
})
