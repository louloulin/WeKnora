# WeKnora 飞书文档协作能力 — v2 增量分析（v0.7.37 完结后）

> 本报告是 `STATUS.md` / `PORT_PLAN_V2.md` / `ROADMAP_V2.md` 之后第 3 轮全面盘点。
> 覆盖 `v0.7.30 → v0.7.37` 的真实代码改动，并把 `/Users/louloulin/appx/genoffice`
> 的能力按 **可移植颗粒度** 重新评估，明确「哪些直接复用 / 哪些需要适配层 / 哪些不移植」。

## 0. TL;DR

| 维度 | 当前状态 | 飞书/腾讯目标 | 主要差距 |
| --- | --- | --- | --- |
| DOC 文本/段落 | ✅ TipTap + docx-engine + 11 扩展 + 11 个 pmDocToSavePlan | ✅ 完整 | 表格 / 图片 round-trip 仅 URL；缺 docx 批注（comment mark） |
| DOC 公式 | ✅ OMML → LaTeX + mathDisplayParagraph | ✅ MathType 级 | docx-engine `math.ts` 1100 行只用了 ~30%，需要 rich display |
| DOC 图表 | ✅ buildDocChartPart 3 类 | ✅ 11 类 | 只 bar/line/pie，缺 surface/radar/area/doughnut/scatter/bubble |
| SHEET 公式 | ✅ SheetJS + f/z | ✅ 飞书级 | 缺条件格式 / 数据验证 / 冻结 / 跨表引用 |
| SHEET 图表 | ❌ 自绘 SVG 缺 | ✅ | 全部缺 |
| SHEET 选区广播 | ✅ v0.7.31 | ✅ | 已完 |
| SHEET 评论 | ❌ 接入未做 | ✅ | 后端表已有，前端 panel 未挂 SHEET |
| PPT 形状 | ✅ 11 形状 + 表格 + 备注 + 评论 + 选区广播 + 选区 awareness | ✅ | 母版/主题/版式 v0.7.32 已接，但前端 UI 缺面板 |
| PPT 图表 | ✅ 11 图表 adapter + 动画 | ✅ SmartArt 缺 | 缺 dgm-hier SmartArt 渲染（genoffice 已实现） |
| PPT 动画 | ✅ 11 动画 adapter | ✅ | UI 触发面板缺；播放预览缺 |
| 实时协作 | ✅ Yjs WS + 选区广播 | ✅ | DOC 选区 awareness 仍 only cursor，缺 range |
| 评论/批注 | ✅ v0.7.35 thread + share | ✅ | DOC/SHEET 未挂 panel；DOC 文档级批注缺 |
| 操作审计 | ✅ v0.7.30 表 + service + handler + UI | ✅ 飞书级 | 差 WebHook / 跨 KB 同步审计 |
| 分享 | ✅ v0.7.35 share token | ✅ | 权限矩阵仅 owner/anyone；缺租户内 / 链接密码 / 过期 |
| AI 润色 | ✅ 段落级 | ✅ 选段 | DOC 段落级已有；SHEET / PPT 缺 |
| Slides 后端 | ✅ v0.7.37 deck + auto-generate | n/a | genoffice 私有功能，已直接端口 |
| Slides 前端 | ❌ 无 view | n/a | 仅后端 + JSON 输出，缺可视化编辑器 |
| 限流 | ❌ | ✅ | middleware 缺 |
| WebHook | ❌ | ✅ | KB 同步 / 分享访问事件缺 |

---

## 1. 当前能力矩阵（v0.7.37 完结后实测）

### 1.1 后端 — Go

| 模块 | 行数 | 表 | 端点数 | 状态 |
| --- | --- | --- | --- | --- |
| `collaborative_docs` | 482 types + 712 service + 260+338+164 handler | 4 | 14 REST + 1 WS + 4 audit + 5 comments | ✅ 完成 |
| `slides` | 273 types + 418 service + 273 test + handler | 1 (`slides` deck) | 11 REST | ✅ 完成（v0.7.37） |
| `wiki` | — | n/a | — | 历史功能，不在本审计范围 |
| `audit_log` (governance) | — | n/a | — | 接线 SlideService.audit hook，**未实现 hook producer** |
| `connector` | — | n/a | — | v0.7.36 接 M365/Google，但**未接飞书/腾讯** |

### 1.2 前端 — Vue 3

| 组件 | 行数 | CRDT | 引擎 | 适配层入口 |
| --- | --- | --- | --- | --- |
| `CollabDocProEditor.vue` | 31,529 byte | ProseMirror + Yjs | docx-engine | `docxAdapter.ts` |
| `CollabSheetEditor.vue` | 26,750 byte | Y.Array×Y.Map | xlsx (SheetJS) | `xlsxAdapter.ts` |
| `CollabSlideKonvaEditor.vue` | 58,935 byte | per-shape Y.Map | pptx-engine + pptx-render | `pptxShapeAdapter.ts` + `pptxAdapter.ts` |
| `CollabCommentsPanel.vue` | 12,767 byte | REST + 5s poll | n/a | 直接调 `api/collabDoc/comments` |
| `CollabAuditTimeline.vue` | 11,592 byte | REST + 30s poll | n/a | 直接调 `api/collabDoc/audit` |

### 1.3 引擎层（已 vendor）

| 引擎 | vendor 文件数 | 行数 | 与 genoffice 差异 |
| --- | --- | --- | --- |
| `docx-engine/` | 27 | ~20k | 仅 `parse.ts` 略改（同步主线） |
| `pptx-engine/` | 42 | ~21k | `polyfills.ts` + `pako.d.ts`；`zip.ts/media-insert.ts/sections.ts` 用 polyfill |
| `pptx-render/` | 12 | ~9k | 全部 6 文件 vendor，import 改相对 |
| `file-parse/` | 4 | 252 | 仅 wrapper |

### 1.4 适配层（adapter-first 架构）

| Adapter | 导出数 | 测试 | 覆盖 |
| --- | --- | --- | --- |
| `docxAdapter.ts` | 36 | `docxMath.test.ts` 6/6 + `docxChart.test.ts` 5/5 + `imageEmbed.test.ts` + `adapters.test.ts` | open/save/patch/math/chart/image + 11 pmDocToSavePlan |
| `pptxShapeAdapter.ts` | 31 | `pptxNotes.test.ts` 2/2 + `pptxAnimations.test.ts` 3/3 + `pptxChart.test.ts` 3/3 + `pptxMasterThemeLayouts.test.ts` 9/9 + `pptxSlideComments.test.ts` 4/4 + `pptxTable.test.ts` | 11 形状 + 表格 + 备注 + 评论 + 11 图表 + 11 动画 + 母版/主题/版式 |
| `xlsxAdapter.ts` | 6 | `adapters.test.ts` 中 | open/save + cellExtras |
| `pptxAdapter.ts` | — | n/a | simple wrapper |

**总测试**：10 个 test 文件，覆盖 ≥35 个 adapter 函数。

---

## 2. 与 genoffice 差距（按文件级别）

### 2.1 已经直接复用 ✅

| genoffice 文件 | WeKnora 落地位置 | 说明 |
| --- | --- | --- |
| `packages/docx-engine/src/*` (27) | `frontend/src/editor/engines/docx-engine/` | 全量 vendor，仅 import 路径调整 |
| `packages/pptx-engine/src/*` (42) | `frontend/src/editor/engines/pptx-engine/` | 全量 vendor + browser polyfills |
| `packages/pptx-render/src/*` (12) | `frontend/src/editor/engines/pptx-render/` | 全量 vendor |
| `packages/file-parse/src/docx.ts/pptx.ts/xlsx.ts` | `frontend/src/editor/engines/file-parse/` | 仅 wrapper，删 Node-only 文件 |
| `packages/docx-engine/src/math.ts` | 已通过 `latexToDocxMath/mathDisplayParagraph` adapter 暴露 | 11 行 adapter 包装 |
| `packages/docx-engine/src/chart.ts` | 已通过 `buildDocChartPart/patchDocChartPart/parseDocChartPart` adapter 暴露 | 5 行 adapter 包装 |
| `packages/pptx-engine/src/notes.ts` | 已通过 `setSlideNotesOnDeck` adapter | 签名已对齐 |
| `packages/pptx-engine/src/comments.ts` | 已通过 `get/add/deleteSlideCommentOnDeck` adapter | 4 个测试通过 |
| `packages/pptx-engine/src/animation.ts` | 已通过 `get/setSlideAnimationsOnDeck` adapter | 11 动画类型 |
| `packages/pptx-engine/src/chart-insert.ts` | 已通过 `addChartToSlide` adapter | 11 图表类型 |
| `packages/pptx-engine/src/master-edit.ts` + `theme-apply.ts` + `builtin-layouts.ts` | 已通过 `listMasterParts/applyTheme/recolorDeck/listBuiltinLayouts/ensureBuiltinLayout` adapter | 9 个测试通过 |

### 2.2 部分移植（能力有，UI/接线未完）⚠️

| genoffice 能力 | WeKnora 现状 | 缺口 |
| --- | --- | --- |
| `apps/slides/src/renderer/SlideCanvas.tsx` + `konva-adapter.ts` | ✅ vue-konva 替代 | 缺少 PPT **格式栏**（字体 / 颜色 / 对齐 / 边框）的 property panel |
| `apps/slides/src/renderer/AnimationPlay.ts` | ❌ | 适配层 OK 但前端未触发播放 |
| `apps/slides/src/renderer/SmartArt` (dgm-hier) | ❌ | adapter 缺 `addSmartArtToSlide` |
| `apps/slides/src/renderer/insert-presets.ts` | ⚠️ 部分 | 11 形状已可插入，缺图片 / 视频 / 嵌入对象 |
| `apps/slides/src/renderer/ink.ts` (墨迹) | ❌ | 完全未移植 |
| `apps/docs/src/renderer/editor/convert.ts` (pmDoc ↔ SaveBlock) | ✅ | `pmDocToSavePlan` 已实现但只覆盖 paragraph/heading/list/image/table；缺 comment / footnote / TOC |
| `apps/docs/src/renderer/editor/equation.ts` | ⚠️ math 通过 | 缺 UI 公式输入面板 |
| `apps/docs/src/renderer/editor/comments.ts` (文档级批注) | ❌ | 后端表已有，前端 mark 缺 |
| `apps/docs/src/renderer/editor/margin-annotations.ts` (页边批注) | ❌ | |
| `apps/docs/src/renderer/editor/cover-pages.ts` | ❌ | |
| `apps/docs/src/renderer/editor/page-break.ts` | ⚠️ 分页符已有 TipTap 内置 | 缺分节符 / 分栏 |
| `apps/docs/src/renderer/editor/direction.ts` (RTL) | ❌ | |
| `apps/docs/src/renderer/editor/case-transform.ts` | ❌ | |
| `apps/sheets/src/renderer/*` (Univer 替代) | ❌ 自研 SheetJS + Y.Map | 公式 / 条件格式 / 数据验证 / 冻结 / 图表全部缺 |
| `packages/ai-search` + `packages/ai-provider` | ✅ ChatPolisher 段落级 | 缺「选段 → AI 指令 → 替换」与「全文改写」 |
| `packages/agent-core` (Slide autosave + conflict resolution) | ⚠️ 仅后端 audit hook | 前端冲突解决策略未实现 |

### 2.3 不移植 ❌（明确排除）

| 项 | 理由 |
| --- | --- |
| Electron 主进程 / preload / native | WeKnora 是 SaaS，不需要桌面应用；`desktop/` 329MB 二进制应 .gitignore |
| `apps/pdf/*` | PDF 编辑不在飞书文档范畴 |
| `apps/markdown/*` | Markdown app 不在计划内 |
| `packages/pdf2docx/*` | 可作为 v0.7.40+ 的「PDF → DOC」导入工具；**本期不做** |
| `packages/electron-utils/*` | 同 Electron 排除 |
| `tools/*` (genoffice dev tooling) | 无移植价值 |

---

## 3. v0.7.38 → v0.8.0 路线图（建议）

> 时间盒：6 周 / 1 人月 / 3 个版本号。
> 策略：**先补完「飞书级最小可用」** → 再做「高级渲染」 → 最后做「生产化」。

### v0.7.38 — 飞书级最小可用（2 周）

**目标**：DOC / SHEET / SLIDE 三类型达到飞书文档 2026 入门级，能替换企业内 80% 的 Notion 使用场景。

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.38.1 | PPT 格式面板（字体 / 颜色 / 对齐 / 边框） | `CollabSlideKonvaEditor.vue` + 新 `CollabFormatToolbar.vue` | 选中 shape 后可改 fill/stroke/font/family |
| 0.7.38.2 | DOC 文档级批注（mark `+` panel 接入） | `CollabDocProEditor.vue` + `CollabCommentsPanel.vue` 扩 kind=doc | 选段 → 「批注」→ 显示在右侧 |
| 0.7.38.3 | SHEET 评论接入 | `CollabSheetEditor.vue` + `CollabCommentsPanel.vue` 扩 kind=sheet | 单元格右键 → 批注 |
| 0.7.38.4 | PPT 动画播放面板 | `CollabSlideKonvaEditor.vue` + `pptxShapeAdapter.ts` 已有 | 选中动画 → 预览 |
| 0.7.38.5 | DOC 选区 range awareness | `CollabDocProEditor.vue` + `useYjsCollabDoc.ts` | 远端选区高亮 |
| 0.7.38.6 | SHEET 公式栏 UI | `CollabSheetEditor.vue` | `=A1+B2` 输入 + 结果 |
| 0.7.38.7 | 后端 Slides audit hook 接通 governance | `container.go` + `service/slides/service.go` | 操作落 `audit_log` |
| 0.7.38.8 | 修 `desktop/` 329MB 二进制 .gitignore | `.gitignore` | git status 干净 |
| 0.7.38.9 | 适配层测试加 pptxFormatBrush / pptxThemesPanel 各 2 个 | `__tests__/` | pass |

### v0.7.39 — 高级渲染 + 飞书级特性（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.39.1 | SHEET 条件格式（cellRules JSON in Y.Map） | `CollabSheetEditor.vue` + `xlsxAdapter.ts` | 选中区段 → 条件规则 → 实时渲染 |
| 0.7.39.2 | SHEET 数据验证（dropdown / number range） | 同上 | |
| 0.7.39.3 | SHEET 冻结窗格（rows/cols 锁定 scroll） | 同上 | |
| 0.7.39.4 | SHEET 图表（自绘 SVG，bar/line/pie） | 新 `xlsxChartAdapter.ts` | 选区 → 「插入图表」→ SVG |
| 0.7.39.5 | DOC 公式 UI 输入（`/math` 触发 → mathDisplayParagraph） | `CollabDocProEditor.vue` | |
| 0.7.39.6 | DOC 图表 11 类扩（surface/radar/area/doughnut/scatter/bubble） | `docxAdapter.ts` | |
| 0.7.39.7 | DOC 表格 round-trip 完整（cell 颜色 / 合并） | `docxAdapter.ts` + pmDocToSavePlan | |
| 0.7.39.8 | PPT SmartArt（`addSmartArtToSlide` + `dgm-hier`） | `pptxShapeAdapter.ts` | 插入组织结构图 |

### v0.7.40 — 生产化（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.40.1 | 限流中间件（per-tenant / per-doc / per-IP） | `internal/middleware/ratelimit.go` | 429 响应正确 |
| 0.7.40.2 | WebHook（KB 同步 / 分享访问 / 评论事件） | `internal/handler/webhooks.go` + 客户端 `useWebhookDelivery.ts` | outbound POST 重试 3 次 |
| 0.7.40.3 | Audit 查询 UI 完善（filter panel + export CSV） | `CollabAuditTimeline.vue` | |
| 0.7.40.4 | Slides 前端可视化编辑器（接 v0.7.37 后端） | 新 `CollabSlideDeckEditor.vue` | 创建 deck → 编辑 slide → 导出 |
| 0.7.40.5 | Slides 后端：export .pptx（服务端用 pptxgenjs） | `service/slides/service.go` | export .pptx 字节 |
| 0.7.40.6 | 冲突解决策略（Yjs undo manager + auto-merge） | `useYjsCollabDoc.ts` | 双客户端冲突提示 |
| 0.7.40.7 | 公开分享密码 + 过期 | `internal/handler/collaborative_doc_bytes.go` | |
| 0.7.40.8 | E2E Playwright 冒烟 | `scripts/smoke-collab-docs.sh` | doc/sheet/slide/slides 全通过 |

---

## 4. 关键技术决策（建议）

### 4.1 保留 adapter-first 架构

继续所有引擎能力通过 `frontend/src/editor/adapters/{docx,pptx,xlsx}Adapter.ts` 暴露；
genoffice 内部 `apps/*` 的 renderer-only 代码（如 `konva-adapter.ts` / `convert.ts`）通过
**新建 Vue 版**而不是直接 import。

### 4.2 不引入 Univer

Univer 包大小 ~3MB、Yjs provider 与现有 `useYjsCollabDoc` 命名冲突、且 Univer 自带
公式 / 条件格式 / 跨表与 genoffice 飞书级目标对不上。**自研 + SheetJS 改造**路线更省
时间，且 SHEET 用户体验在飞书级别更可控。

### 4.3 genoffice 反向归一

v0.7.30 起已与 genoffice 主线持续同步（`docs/collab-docs/SYNC_LOG.md` 应该有记录）。
继续每月 1 次同步：取 genoffice `master` 分支 → diff → apply → 跑 adapter test 51/51。

### 4.4 Slides 后端与 PPT 编辑器分离

v0.7.37 的 `slides` 是**独立产品**（类似 Notion AI Slides），与 `collaborative_docs`
下 `doc_kind=pptx` 的 PPT 编辑器并行存在。前端用两个不同 view 装载，不要强行合并。

---

## 5. 验证命令（统一入口）

```bash
# 1. 引擎 vendor 一致性
diff -r /Users/louloulin/appx/WeKnora/frontend/src/editor/engines/docx-engine/src \
        /Users/lougoulin/appx/genoffice/packages/docx-engine/src \
        --brief | head -20
# 期望：仅 polyfills / import path

# 2. 适配层测试
cd /Users/louloulin/appx/WeKnora/frontend && \
  ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/*.test.ts
# 期望：51 pass

# 3. Type 检查（collab 块）
cd /Users/louloulin/appx/WeKnora/frontend && \
  ./node_modules/.bin/vue-tsc --build 2>&1 | grep -E "error TS" | grep -iE "collab|konva|pptxShape|docxAdapter|sheet"
# 期望：0

# 4. 后端编译 + 单测
cd /Users/louloulin/appx/WeKnora && \
  go build ./... 2>&1 | tail -3 && \
  go test ./internal/application/repository/ -count=1 -run "TestCollabDoc|TestAuditRepo|TestSlide"
# 期望：build 干净；test ok

# 5. E2E 冒烟（已有 scripts/smoke-collab-docs.sh）
cd /Users/louloulin/appx/WeKnora && bash scripts/smoke-collab-docs.sh
# 期望：DOC + SHEET + SLIDE + SLIDES 全过
```

---

## 6. 不做的事（明确 out-of-scope）

- ❌ 桌面客户端（`desktop/` 二进制属于个人本地构建产物）
- ❌ 飞书 / 腾讯 / Lark / 钉钉 / Notion 的 connector 接入（v0.7.36 connector 仅 M365/Google）
- ❌ 实时音视频会议
- ❌ 邮件 / 通知中心（前端已有但不是 collab 一部分）
- ❌ 移动端原生 App

---

## 7. 总结 — 一句话

**WeKnora 已经把 genoffice 三件套引擎全部 vendor 完毕、适配层 35+ 函数封装完毕、审计/分享/评论/选区基础完成（v0.7.30 → v0.7.35）；当前缺口集中在「飞书级 UI 完善 + SHEET 高级渲染 + 生产化」三块，按 v0.7.38 → v0.7.40 顺序推进 6 周可达到飞书文档 2026 入门级。**
