# WeKnora 飞书文档协作 — 完整盘点与 v0.7.41+ 路线图

> 第 4 轮全面盘点。承接 `ANALYSE_V2.md` / `ROADMAP_V2.md` / `STATUS.md`。
> 本轮新事实:**已确认 v0.7.38 收尾,v0.7.39 / v0.7.40 计划仍在文档层面;此前被标为"critical blocker"的 share 测试编译失败,经 git stash 验证是 PRE-EXISTING 的 wiki_*_test.go helper 冲突,与 v0.7.38 share 改动无关**。

---

## 0. 结论先行(一句话)

**WeKnora 已经把 genoffice 三件套引擎全部 vendor + adapter 化(35+ 函数 + 51 个测试)。genoffice 本身没有协作层,本项目以 Yjs + TipTap 自研叠加。飞书级差距已缩到 12 项;按 v0.7.41 → v0.7.45 分 5 个版本 / 10 周推进可达飞书文档 2026 入门级。**

---

## 1. 双项目结构实测(2026-09-01)

### 1.1 WeKnora 当前 collab 资产

```
internal/
  application/
    service/
        collaborative_doc.go         # 主 service (含 share 密码、token、过期、WebHook emit)
        collaborative_doc_authz.go   # 权限分流
        collaborative_doc_share_test.go  # 3 个新 share 单测
        webhooks/webhooks.go         # WebHook delivery + retry
      slides/service.go          # Slides (Notion AI Slides 类) — v0.7.37 独立产品
    repository/
        collab_doc.go              # 含 share_password_hash / share_expires_at 字段
        webhooks.go                # WebHook 持久化
    middleware/
        collab_ratelimit.go        # per-tenant / per-doc / per-IP
  handler/
    collaborative_doc.go          # REST 主入口 + EnableShare / DisableShare
    collaborative_doc_bytes.go    # upload/download + share X-Share-Password 校验
    collaborative_doc_audit.go    # audit timeline
    collaborative_doc_comments.go # threaded comments
    collaborative_doc_sync.go     # sync-to-kb
    collaborative_doc_ws.go       # Yjs WS endpoint
    slides.go                     # slide deck handler
    webhooks.go                   # webhook CRUD
  types/
    collaborative_doc.go          # +SharePasswordHash +ShareExpiresAt
    audit_log.go                  # 9 个新 AuditAction 常量 (collab_doc.* + slide_deck.*)
    webhooks.go                   # WebHookEventList 自定义 Scanner/Valuer
    interfaces/webhooks.go
  router/
    routes_collaborative_doc.go   # 14 REST + 1 WS + 4 audit + 5 comments + 2 share
    routes_webhooks.go
  migrations/
    sqlite/000051_webhooks.{up,down}.sql
    sqlite/000053_collab_share_protect.{up,down}.sql
    mysql/000046_webhooks.{up,down}.sql
    mysql/000047_collab_share_protect.{up,down}.sql

frontend/
  src/components/collab/
    CollabDocProEditor.vue        # DOC + TipTap + Yjs (809 行)
    CollabSheetEditor.vue         # SHEET + SheetJS + Y.Map (793 行)
    CollabSlideKonvaEditor.vue    # PPT + Konva + Y.Map per shape (1644 行)
    CollabSlideDeckEditor.vue     # Slides (Notion AI Slides 类) — v0.7.37 (377 行)
    CollabCommentsPanel.vue       # 共享评论 (432 行)
    CollabAuditTimeline.vue       # 审计时间线 (379 行)
    CollabAiPolishDialog.vue      # AI 段落级润色 (200 行)
    CollabDocEditor.vue           # 简易 fallback (99 行)
    CollabSlideEditor.vue         # 简易 PPT fallback (434 行)
  src/views/collab/
    CollabDocEditorView.vue
    CollabDocListView.vue
    CollabDocShareView.vue
    CollabSlidesView.vue          # 19 行 wrapper
  src/api/
    collabDoc/index.ts            # 全部 18 个端点封装 (253 行)
    slides/index.ts               # slide deck 端点封装
  src/composables/
    useYjsCollabDoc.ts            # v0.7.25 Yjs WS composable (含 publishSelection)
    useYjsCollabDocPersistence.ts # v0.7.27 IndexedDB 持久化
  src/editor/
    engines/                      # genoffice 引擎 vendor (docx 27 文件 / pptx 42 文件 / pptx-render 12 文件)
    adapters/
      docxAdapter.ts              # 36 函数,含 math/chart/image/patch
      pptxAdapter.ts              # 简单 wrapper
      pptxShapeAdapter.ts         # 31 函数,11 形状 + 表格 + 11 动画 + 11 图表 + 母版/主题/版式
      xlsxAdapter.ts              # 6 函数,open/save + cellExtras
    adapters/__tests__/           # 10 个 test 文件,51 tests pass
```

### 1.2 genoffice 资产盘点(对照表)

| 维度 | genoffice 实现 | WeKnora 现状 | 移植方式 |
| --- | --- | --- | --- |
| docx 引擎 | `packages/docx-engine/` ~20k 行 + `apps/docs/src/renderer/editor/convert.ts` (PM↔SaveBlock) | vendor 完毕 + adapter 36 函数 | ✅ 已移植 |
| pptx 引擎 | `packages/pptx-engine/` ~21k 行 + `packages/pptx-render/` ~9k 行 | vendor 完毕 + 31 函数 shape adapter | ✅ 已移植 |
| xlsx 引擎 | `packages/file-parse/xlsx.ts` + `packages/sheets` | 仅 vendor + 6 函数 adapter | ⚠️ 部分(Sheets app 未移植) |
| PDF | `packages/pdf2docx/` + `apps/pdf/` | 未移植 | ❌ 不在范围 |
| 协作 | **genoffice 无 Yjs — 单用户 Electron + 本地 project-store** | Yjs + TipTap 自研 | ⚠️ 自研,不可移植 |
| 引擎适配 | TS 直接调 (PM doc 双向序列化) | Vue3 重写 (ProseMirror ↔ docx-engine Block 桥) | ✅ 适配完成 |
| AI 集成 | `apps/*/src/renderer/ai/` | CollabAiPolishDialog (段落级) | ⚠️ 段落级已接 |
| 撤销/历史 | PM History + 自研 `doc-cache.ts` | Yjs UndoManager (未接) | ❌ 未接 |
| 主题/母版 | pptx-engine 内置 | pptxShapeAdapter 已暴露 | ✅ 已暴露,前端 UI 缺 |
| SmartArt | `pptx-engine/dgm-hier.ts` 295 行 | **未移植** | ❌ 未移植 |
| 公式 | docx-engine/math.ts 1087 行 + LaTeX 渲染 | **仅用 ~30%** (mathDisplayParagraph 已接,UI 输入缺) | ⚠️ 半移植 |
| 11 图表 | docx-engine/chart.ts 951 行 + pptx-engine/build-chart 3059 行 | docx 3 类 + pptx 11 类 | ⚠️ DOC 11 类未全 |
| 形状/几何 | pptx-render/preset-geometry 1548 行 + build-slide 805 行 | adapter 已暴露 | ✅ |

### 1.3 genoffice 不可移植 / 不移植的部分

| 功能 | 原因 |
| --- | --- |
| Electron shell + tab-manager | WeKnora 是 Web SaaS,不要 desktop |
| Local project-store | WeKnora 有 KB/DB 后端,不要本地 JSON |
| Cloud projects sync | 不在本项目范围 |
| Univer (genoffice 引入) | 同感不引入 — 包大小 + Yjs provider 冲突 + 飞书级目标对不上 |
| PDF 解析 (pdf2docx 308 行) | 不在范围 |
| Real-time collaboration | genoffice 无,WeKnora 自研 |

---

## 2. 飞书级差距 — 实测 12 项

| # | 类别 | 当前 | 目标 | 优先级 |
| --- | --- | --- | --- | --- |
| G1 | SHEET 条件格式 | ❌ | ✅ cellRules JSON in Y.Map | P0 |
| G2 | SHEET 数据验证 | ❌ | ✅ dropdown + number range | P0 |
| G3 | SHEET 冻结窗格 | ❌ | ✅ rows/cols 锁定 | P0 |
| G4 | SHEET 图表 | ❌ | ✅ 自绘 SVG bar/line/pie | P1 |
| G5 | DOC 公式 UI 输入 | ❌ | ✅ `/math` 触发 mathDisplayParagraph | P0 |
| G6 | DOC 图表 11 类 | 部分 (3/11) | ✅ surface/radar/area/doughnut/scatter/bubble | P1 |
| G7 | DOC 表格 round-trip | 部分 | ✅ cell 颜色 + 合并保留 | P1 |
| G8 | PPT SmartArt | ❌ | ✅ dgm-hier 移植 | P1 |
| G9 | PPT 动画播放预览 | ❌ | ✅ timeline 预览 | P1 |
| G10 | DOC 文档级批注 mark | 部分 | ✅ mark + panel | P0 |
| G11 | WebHook share events | ❌ | ✅ collab.doc.shared emit | P0 |
| G12 | Slides 服务端 .pptx export | ❌ | ✅ pptxgenjs | P1 |

---

## 3. v0.7.41 → v0.7.45 完整路线图(10 周 / 1 人月+)

### v0.7.41 — 修脚手架 + 修补 P0 短板(1 周)

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 41.1 | 修 PRE-EXISTING wiki_*_test.go helper 冲突(8 个 contains/fakeClock/stub* 改名) | `internal/application/service/*_test.go` | `go test ./internal/application/service/` build pass |
| 41.2 | 接 share WebHook emit `collab.doc.shared` | `collaborative_doc.go` EnableShare/DisableShare | publishEvent 已存在,补 callsite |
| 41.3 | CollabDocProEditor 接 CollabCommentsPanel(DOC 文档级批注 mark) | `CollabDocProEditor.vue` + `CollabCommentsPanel.vue` | 选段 → 「批注」→ 显示 |
| 41.4 | SHEET 选区 cell outline + comments panel 接入 | `CollabSheetEditor.vue` | 单元格右键 → 批注 |
| 41.5 | run share_test (修复后) | `internal/application/service/collaborative_doc_share_test.go` | 3 tests pass |
| 41.6 | 文档:`STATUS.md` v0.7.38 + 41 增量 + 修正 blocker 描述 | `docs/collab-docs/STATUS.md` | — |

### v0.7.42 — SHEET 飞书级(2 周)

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 42.1 | SHEET 条件格式 (cellRules JSON in Y.Map) | `CollabSheetEditor.vue` + `xlsxAdapter.ts` | 选中区段 → 规则 → 实时渲染 |
| 42.2 | SHEET 数据验证 (dropdown + number range) | 同上 | — |
| 42.3 | SHEET 冻结窗格 (rows/cols 锁定 scroll) | 同上 | — |
| 42.4 | SHEET 图表自绘 SVG (bar/line/pie) | 新 `xlsxChartAdapter.ts` | 选区 → 「插入图表」→ SVG |
| 42.5 | SHEET 公式栏 UI 完善 (`=A1+B2` 输入 + 结果 + 跨表引用) | `CollabSheetEditor.vue` | — |
| 42.6 | adapter 测试 +4 (`xlsxChart.test.ts` 等) | `__tests__/` | pass |

### v0.7.43 — DOC 飞书级(2 周)

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 43.1 | DOC 公式 UI 输入 (`/math` 触发 → mathDisplayParagraph) | `CollabDocProEditor.vue` + `docxAdapter.ts` | 输入 LaTeX → 渲染 |
| 43.2 | DOC 图表 11 类扩 (surface/radar/area/doughnut/scatter/bubble) | `docxAdapter.ts` | 11 类全 |
| 43.3 | DOC 表格 round-trip 完整 (cell 颜色 + 合并 + 行高) | `docxAdapter.ts` + `pmDocToSavePlan` | open/edit/save 不丢 |
| 43.4 | DOC 选区 range awareness 远端高亮 | `CollabDocProEditor.vue` + `useYjsCollabDoc.ts` | 双客户端可见 |
| 43.5 | DOC TOC / 大纲视图 | 新 `CollabDocOutline.vue` | 显示标题层级 |
| 43.6 | adapter 测试 +5 | `__tests__/` | pass |

### v0.7.44 — PPT 飞书级(2 周)

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 44.1 | PPT SmartArt (移植 dgm-hier 295 行 → `pptxShapeAdapter.ts`) | 同上 | 插入组织结构图 |
| 44.2 | PPT 动画播放预览 timeline | `CollabSlideKonvaEditor.vue` + 新 `CollabAnimPreview.vue` | 选中动画 → 预览 |
| 44.3 | PPT 母版/主题/版式 前端 UI 面板 | `CollabSlideKonvaEditor.vue` | 切换母版生效 |
| 44.4 | PPT 演讲者备注 UI 完善 | 同上 | 备注与讲义双视图 |
| 44.5 | adapter 测试 +3 (`pptxSmartArt.test.ts` 等) | `__tests__/` | pass |

### v0.7.45 — 生产化 + Slides 闭环(2 周)

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 45.1 | Slides 服务端 export .pptx (接 pptx-engine generate) | `internal/application/service/slides/service.go` | export 字节 |
| 45.2 | Slides 前端可视化编辑器完善 (transition + 主题 + 备注) | `CollabSlideDeckEditor.vue` | 创建 deck → 编辑 → export |
| 45.3 | Audit 查询 UI filter panel + export CSV | `CollabAuditTimeline.vue` | — |
| 45.4 | 冲突解决策略 (Yjs undo manager + auto-merge) | `useYjsCollabDoc.ts` | 双客户端冲突提示 |
| 45.5 | E2E Playwright 冒烟 (doc/sheet/slide/slides 全跑) | `scripts/smoke-collab-docs.sh` | 全通过 |
| 45.6 | 性能:adapter 批量化 / SSE 增量 | `useYjsCollabDoc.ts` | — |

---

## 4. 关键技术决策(继承 + 新增)

### 4.1 继承(已确认)

1. **Adapter-first** — 引擎能力全经 `frontend/src/editor/adapters/{docx,pptx,xlsx}Adapter.ts` 暴露
2. **vendor 持续同步** — 每月 1 次从 genoffice master 取差,apply 后跑 51 个 adapter test
3. **WebHook 签名** — HMAC-SHA256 `sha256=...`,兼容 GitHub X-Hub-Signature-256
4. **DOC/SHEET 选区 awareness** — 独立字段 `selection: {from,to}` (DOC) / `cell: {ri,ci}` (SHEET)
5. **rate limit 三档** — 600/min tenant + 300/min doc + 120/min IP,Redis + local fallback
6. **Slides 模块独立** — v0.7.37 Notion-AI-Slides 类独立产品,不与 collab_doc pptx 合并

### 4.2 新增(本轮)

7. **修 PRE-EXISTING wiki test 冲突** — 这是本轮首要技术债,**非 share 改动造成**。8 个 helper 名加 `wiki` / `audit` / `cache` 前缀
8. **公式输入走 `/math` 触发** — 不弹 modal,直接 inline;与飞书/Microsoft Loop 一致
9. **条件格式 JSON 序列化到 Y.Map** — 与 Univer 行为对得上,但底层用 SheetJS 渲染
10. **SmartArt 移植策略** — 把 `dgm-hier.ts` 直接 vendor 到 `frontend/src/editor/engines/pptx-engine/dgm-hier.ts`,adapter 调用
11. **Slides export .pptx** — 服务端直接调 `pptx-engine/generate.ts` 的纯 JS 版本(已 vendor),不引入 pptxgenjs

---

## 5. 验证命令(单入口)

```bash
cd /Users/louloulin/appx/WeKnora

# 1. 后端编译 + 全部测试
go build ./... 2>&1 | tail -3
go test ./internal/application/... -count=1 2>&1 | tail -10

# 2. 前端 TS + adapter 测试 + build
cd frontend &&   ./node_modules/.bin/vue-tsc --build 2>&1 | grep -cE "error TS" &&   ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/*.test.ts &&   npm run build-only

# 3. E2E
cd .. && bash scripts/smoke-collab-docs.sh
```

---

## 6. Token 经济

- 单 turn vue-tsc ~12-25s;adapter 测试 ~0.5s;backend test ~0.6s;build ~19-21s
- 每版本独立 commit;每个 task 影响 1-3 文件
- 不 commit(除非用户明确要求)

---

## 7. 不做的事(明确 out-of-scope)

- ❌ 飞书 / 腾讯 / Lark / Notion 的 connector(v0.7.36 connector 仅 M365/Google)
- ❌ 桌面客户端(desktop/ 二进制属于个人本地构建产物)
- ❌ 实时音视频会议
- ❌ 移动端原生 App
- ❌ PDF 解析 / 转换(genoffice pdf2docx 不移植)
- ❌ Univer 替换 SheetJS(自研 + 适配器路线,已在 ANALYSE_V2 决策)

---

## 8. 一句话总结

**WeKnora 已完成 genoffice 引擎 vendor + 适配层 35+ 函数 + Yjs 协作自研 + 9 项 P0 后端能力(v0.7.30 → v0.7.38);飞书级差距仅剩 12 项,按 v0.7.41 → v0.7.45 分 5 个版本 / 10 周推进可达飞书文档 2026 入门级。**

---

## 9. v0.7.46+ 路线图(genoffice 三件套 gateway copy 余量)

承接 v0.7.43.c（§13.4），继续 copy genoffice 剩余 gateway，目标是把 xlsx / docs / slides 三件套的核心 OOXML 能力 100% 落地到 WeKnora。

### 9.1 genoffice gateway 余量盘点

| 类型 | 文件 | 行数 | 状态 | 改造难度 | 优先级 |
| --- | --- | --- | --- | --- | --- |
| SHEET | `xlsx-notes.ts`（单元格批注） | 297 | ❌ 未 copy | 中（多文件 + VML） | P0 |
| SHEET | `xlsx-sheets.ts`（sheet 增删改 + rename 引用替换） | 607 | ❌ 未 copy | 中 | P0 |
| SHEET | `xlsx-page-setup.ts`（打印页面） | 487 | ❌ 未 copy | 低（纯属性写） | P0 |
| SHEET | `xlsx-package-io.ts`（包级 IO） | 366 | ❌ 未 copy | 低 | P1 |
| SHEET | `xlsx-table-add.ts`（ListObject 表对象） | 292 | ❌ 未 copy | 中（多文件 + Content_Types） | P1 |
| SHEET | `xlsx-structure.ts`（结构性操作 + 引用解析） | 1384 | ❌ 未 copy | 高（被多文件依赖） | P1 |
| SHEET | `xlsx-drawing-edit.ts`（SHEET 内绘图编辑） | 406 | ❌ 未 copy | 中 | P1 |
| SHEET | `xlsx-drawing-add.ts`（绘图新增） | 842 | ❌ 未 copy | 中 | P2 |
| SHEET | `xlsx-pivot.ts` + `pivot-add.ts` + `pivot-expand.ts` | 1795 | ❌ 未 copy | 高（透视表全功能） | P2 |
| SHEET | `csv-import.ts` | 259 | ❌ 未 copy | 低 | P3 |
| DOC | `editor/equation.ts`（TipTap math 节点） | 600+ | ❌ 未 copy | 高（自定义 schema） | P1 |
| DOC | `editor/convert.ts`（TipTap ↔ docx-engine） | 500+ | ❌ 未 copy | 高（pmDocToSavePlan） | P1 |
| DOC | `editor/table-handle.ts` + `table-properties.ts` + `table-sizing.ts` | 800+ | ❌ 未 copy | 中 | P1 |
| DOC | `editor/revisions.ts`（修订历史） | 400+ | ❌ 未 copy | 高 | P2 |
| DOC | `editor/shape-draw.ts` + `shape-svg.ts` | 500+ | ❌ 未 copy | 中 | P2 |
| DOC | `editor/hf-dom.ts` + `hf-text.ts` + `cover-pages.ts` | 700+ | ❌ 未 copy | 中 | P2 |
| DOC | `editor/toc-refresh.ts` + `pagination-gaps.ts` + `page-break.ts` | 600+ | ❌ 未 copy | 中 | P2 |
| DOC | `editor/numbering.ts`（编号） | 400+ | ❌ 未 copy | 中 | P2 |
| DOC | `editor/comments.ts`（DOC 批注） | 300+ | 部分已接 | 中 | P1 |
| DOC | `editor/para-border-merge.ts` | 200+ | ❌ 未 copy | 低 | P3 |
| PPT | `renderer/animation-actions.ts` + `animation-play.ts` | 800+ | 部分已接 | 低 | P1 |
| PPT | `renderer/table-actions.ts` + `table-hit.ts` | 500+ | ❌ 未 copy | 中 | P1 |
| PPT | `renderer/draw-shape.ts`（PPT 形状绘制） | 300+ | ❌ 未 copy | 中 | P1 |
| PPT | `renderer/format-brush.ts` | 400+ | ❌ 未 copy | 中 | P2 |
| PPT | `renderer/picture-edit-actions.ts` | 400+ | ❌ 未 copy | 中 | P2 |
| PPT | `renderer/ruler-ticks.ts` + `adjust-handles.ts` | 300+ | ❌ 未 copy | 中 | P2 |
| PPT | `renderer/clipboard-actions.ts` | 400+ | ❌ 未 copy | 低 | P2 |
| PPT | `renderer/arrange-actions.ts` | 300+ | ❌ 未 copy | 中 | P2 |
| PPT | SmartArt (dgm-hier) | - | ❌ 未 copy | 高（OOXML schema 自写） | P2 |

总计约 **13000+ 行**未 copy。其中 P0 / P1 高价值低改造约 **4500 行**。

### 9.2 v0.7.44 — SHEET 余量 copy 第 1 批（1 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 44.1 | copy `xlsx-page-setup.ts` → `xlsxPageSetup.ts` | `frontend/src/editor/adapters/` | 7 tests pass: orientation / paperSize / fitToPage / margins / printGridlines / printHeadings / headers |
| 44.2 | 接入 `CollabSheetEditor.vue` 页面设置 modal | `CollabSheetEditor.vue` | 「页面」按钮 → 弹 modal → 应用 |
| 44.3 | copy `xlsx-sheets.ts` → `xlsxSheets.ts` | 同上 | 5 tests pass: addSheet / renameSheet / removeSheet / moveSheet / hideSheet |
| 44.4 | 接入 `CollabSheetEditor.vue` sheet 管理 UI | `CollabSheetEditor.vue` | 右键 tab → 「重命名/删除/移动/隐藏」 |
| 44.5 | 扩展 `transformWorkbook` 支持多 sheet XML 串行编辑 | `xlsxWorksheetIo.ts` | 多 sheet round-trip 不串扰 |
| 44.6 | STATUS.md §14 + PLAN_V3.md 更新 | `docs/collab-docs/` | — |

### 9.3 v0.7.45 — SHEET 余量 copy 第 2 批（1 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 45.1 | copy `xlsx-notes.ts` → `xlsxNotes.ts` | `frontend/src/editor/adapters/` | 6 tests pass: addNote / removeNote / listNotes / persist VML + rels |
| 45.2 | 扩展 `transformWorkbook` 支持 worksheet.xml + `<sheetN>.xml.rels` + `[Content_Types].xml` 三文件联动 | `xlsxWorksheetIo.ts` | hyperlink 已对齐（v0.7.44 集成 UI 时一并完成） |
| 45.3 | 接入 SHEET 单元格批注 UI | `CollabSheetEditor.vue` | 右键 cell → 「批注」→ 输入 → 显示红角标 |
| 45.4 | copy `xlsx-table-add.ts` → `xlsxTable.ts` | `frontend/src/editor/adapters/` | 4 tests pass: addTable / styledTable / withTotalsRow |
| 45.5 | 接入 SHEET 表对象 UI（飞书 SHEET 「插入表格」） | `CollabSheetEditor.vue` | 「插入」→ 「表格」→ 选区 + 样式 → 写入 ListObject |
| 45.6 | copy `xlsx-package-io.ts` → `xlsxPackageIo.ts` | 同上 | 4 tests pass: 整包读 + 整包写 + Content_Types 合并 |

### 9.4 v0.7.46 — SHEET 结构操作 + 透视表基础（1.5 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 46.1 | copy `xlsx-structure.ts` → `xlsxStructure.ts` | `frontend/src/editor/adapters/` | 12 tests pass: 公式 / 跨表引用 / 命名区域 / chart series / hyperlink anchor 解析 |
| 46.2 | copy `xlsx-drawing-edit.ts` → `xlsxDrawingEdit.ts` | 同上 | 6 tests pass: 嵌入图片 / 形状 / 调整位置 + 大小 |
| 46.3 | copy `xlsx-pivot.ts` + `xlsx-pivot-expand.ts` → `xlsxPivot.ts` | 同上 | 8 tests pass: 创建透视表 / 字段映射 / 行/列/值区域 |
| 46.4 | 接入 SHEET 透视表 UI（飞书 SHEET 「插入透视表」） | `CollabSheetEditor.vue` | 数据源 + 字段配置 → 实时渲染 |
| 46.5 | adapter 测试 +20（structure + drawing + pivot） | `__tests__/` | pass |
| 46.6 | Y.Map `sheet:features` 同步 feature 规则（freeze/filter/cf/dv/spark/notes/tables/pivots） | `useYjsCollabDoc.ts` | 多客户端可见 |

### 9.5 v0.7.47 — DOC 飞书级补完（2 周）

承接 v0.7.43 的 DOC 路线，重点 copy docs/editor/：

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 47.1 | copy `editor/convert.ts`（pmDocToSavePlan + blocksToPmDoc） | `frontend/src/editor/adapters/docConvert.ts` | 8 tests pass: TipTap ↔ docx-engine Block JSON |
| 47.2 | 接入 `CollabDocProEditor.vue` convert 桥接 | `CollabDocProEditor.vue` | 上传 .docx → 编辑 → 下载 .docx 闭环 |
| 47.3 | copy `editor/table-handle.ts` + `table-properties.ts` + `table-sizing.ts` | `frontend/src/editor/adapters/docTable*.ts` | 6 tests pass: 表格 UI + 颜色 + 合并 + 行高 |
| 47.4 | copy `editor/equation.ts` → 自建 `docInlineMath` / `docProtected` schema | `frontend/src/components/collab/extensions/` | 输入 `/math` → 实时公式预览 |
| 47.5 | copy `editor/comments.ts`（DOC 段落级批注） | `frontend/src/editor/adapters/docComments.ts` | 选段 → 「批注」→ 红角标 + 侧边面板 |
| 47.6 | copy `editor/toc-refresh.ts` + `pagination-gaps.ts` + `page-break.ts` | `frontend/src/editor/adapters/doc*.ts` | 大纲视图 + 分页符 |

### 9.6 v0.7.48 — PPT 飞书级补完（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 48.1 | copy `renderer/draw-shape.ts` + `shape-svg.ts` | `frontend/src/editor/adapters/pptShape*.ts` | 6 tests pass: 矩形 / 圆形 / 箭头 / 流程图 |
| 48.2 | copy `renderer/table-actions.ts` + `table-hit.ts` | `frontend/src/editor/adapters/pptTable*.ts` | 4 tests pass: 表格 CRUD + 命中测试 |
| 48.3 | copy `renderer/animation-actions.ts` 补全 11 类动画 | `frontend/src/components/collab/CollabAnim*.vue` | 全部动画生效 + 时间轴 |
| 48.4 | copy `renderer/format-brush.ts` | `frontend/src/editor/adapters/pptFormatBrush.ts` | 选 → 拖 → 应用 |
| 48.5 | 自写 PPT SmartArt（OOXML dgm schema） | `frontend/src/editor/adapters/pptSmartArt.ts` | 4 tests pass: hierarchy / list / process / cycle |
| 48.6 | 接入 `CollabSlideKonvaEditor.vue` 形状工具栏 + SmartArt 面板 | 同上 | 飞书级 PPT UI |

### 9.7 v0.7.49 — 生产化 + 跨端对齐（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 49.1 | 三件套 gateway 全量 100% 测试覆盖（target: 250+ tests） | `frontend/src/editor/adapters/__tests__/` | pass |
| 49.2 | Y.Map 粒度细化：DOC per-paragraph Y.Text；SLIDE per-shape Y.Map | `useYjsCollabDoc.ts` | 多客户端并发无冲突 |
| 49.3 | 冲突解决策略（Yjs UndoManager + auto-merge + UI 提示） | 同上 | — |
| 49.4 | 后端 export 真实字节流（接 docparser / python-pptx / SheetJS save） | `internal/handler/collaborative_doc.go` | `/export` 返回真 .docx/.pptx/.xlsx |
| 49.5 | 离线编辑（IndexedDB Yjs provider + Service Worker） | `frontend/src/composables/useYjsIndexedDB.ts` | 弱网/离线可编辑 |
| 49.6 | E2E Playwright 冒烟（doc/sheet/slide 全跑）+ 性能基线 | `scripts/smoke-collab-docs.sh` | 通过 + <500ms P95 |

### 9.8 Token 经济预估

按每 turn 实际：
- v0.7.43 (公式引擎 + 5 modals + 公式持久化) — 本 turn 之前完成
- v0.7.43.c (cells 颜色) — 本 turn 完成（修复 1 bug + 4 tests）
- v0.7.44 (page-setup + sheets + transformWorkbook 扩展) — 预计 1 turn
- v0.7.45 (notes + tables + package-io + hyperlinks UI) — 预计 1 turn
- v0.7.46 (structure + drawing + pivot) — 预计 1.5 turn（pivot 复杂）
- v0.7.47 (DOC convert + table + equation + comments) — 预计 1.5 turn
- v0.7.48 (PPT shape + table + animation + format-brush + SmartArt) — 预计 1.5 turn
- v0.7.49 (生产化) — 预计 1 turn

**总计 ~9 turn（每 turn 约 30-60 分钟），可完成飞书 / 腾讯文档级三件套全能力。**

### 9.9 关键决策记录

- **transformWorkbook 多文件扩展**：notes / hyperlinks / drawing 需要同时改 worksheet.xml + rels.xml + [Content_Types].xml。需把 `transformWorkbook` 升级为 `transformPackage`，支持 per-file 串行 pipeline。
- **公式自动重算 vs 持久化**：v0.7.43.b 已实现持久化（写入 `<f>` + seed `v=0`），但 SheetJS 不重算。下一步：前端评估引擎（sheetFormula.ts）覆盖写时 + 读时，提供预览值，避免 Excel 打开时看到 stale value。
- **Y.Map 同步 vs 本地 ref**：当前 freeze/filter/cf/dv/spark/notes 仍是本地 ref，Y.Map 同步在 v0.7.46 统一做，避免每加一个 feature 都改 Y.Map schema。
- **SheetJS + 自管 styles.xml 双轨**：v0.7.43.c 已落地——SheetJS 写 cell.s index，我们维护 styles.xml + cellXfs。后续 styles（border / numFmt / protection）都走这套机制。

