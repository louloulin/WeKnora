# WeKnora 飞书文档协作 — 整体分析与 v0.7.28+ 计划

> 本文是 v0.7.27（已完工）之后的整盘盘点。把 `/Users/louloulin/appx/genoffice`
> 已移植的内容、WeKnora 当前实现、剩余 gap 与未来路线一次性说清。

---

## 1. 总览：v0.7.27 已完成的全部能力

### 1.1 后端（Go）— 全部 v0.7.25 + v0.7.26 落地

| 层 | 文件 | 行数 | 状态 |
| --- | --- | --- | --- |
| 类型 | `internal/types/collaborative_doc.go` | 264 | ✅ 三个 DocKind + Validate 接受 Version=0 |
| Repo | `internal/application/repository/collab_doc.go` | 411 | ✅ 文件/Snapshot/Session 三表 CRUD + auto-version |
| Service | `internal/application/service/collaborative_doc.go` | 416 | ✅ 业务层（含 RBAC、share token） |
| Handler | `internal/handler/collaborative_doc.go` (+ bytes / sync / ws) | 196 | ✅ 7 个 REST + 字节下载 + WS 升级 |
| 路由 | `internal/router/routes_collaborative_doc.go` | — | ✅ 全部挂载 `/api/v1/collaborative-docs/*` |
| 测试 | `collaborative_doc_test.go` + `collab_doc_file_test.go` | — | ✅ pass |

数据表：
- `collaborative_docs` — 元数据（UUID + tenant + KB + doc_kind + share_token）
- `collab_doc_snapshots` — Yjs 二进制压缩状态
- `collab_doc_sessions` — 在线协作者
- `collab_doc_files` — 每次保存的 .docx/.pptx/.xlsx 字节 + 版本号

REST 端点（v0.7.25 + v0.7.26）：
```
POST   /collaborative-docs                     # 创建
GET    /collaborative-docs                     # 列表
GET    /collaborative-docs/:id                 # 元数据
PATCH  /collaborative-docs/:id                 # 重命名 / visibility / share_token
POST   /collaborative-docs/:id/archive         # 软删除
DELETE /collaborative-docs/:id                 # 硬删除
GET    /collaborative-docs/:id/presence        # 在线协作者
GET    /collaborative-docs/:id/export          # markdown 占位
POST   /collaborative-docs/:id/sync-to-kb      # 推 docparser
POST   /collaborative-docs/:id/upload          # multipart 上传
GET    /collaborative-docs/:id/download        # 最新字节
GET    /collaborative-docs/:id/download/:v     # 历史版本
GET    /collaborative-docs/share/:token/dl     # 公开只读
GET    /collaborative-docs/:id/realtime        # Yjs WS
```

### 1.2 前端引擎（genoffice 移植）

| 引擎 | 文件数 | 行数 | 状态 |
| --- | --- | --- | --- |
| `docx-engine/` | 25 | 9k | ✅ 完整移植，浏览器可用 |
| `pptx-engine/` | 41 | 17k | ✅ 完整移植 + node-polyfill（117 行） |
| `pptx-render/` | 12 | 15k | ✅ 完整移植但**未接入** Vue 渲染层 |
| `file-parse/` | — | — | ✅ 浏览器子集 |

`pptx-engine/polyfills.ts`：纯 TS SHA-256（FIPS 180-4 vectors 通过）+ `pako.deflateRaw` + `crypto.randomUUID`，已替换 `zip.ts` / `media-insert.ts` / `sections.ts` 的 Node 引用。

### 1.3 前端适配器（adapters）

| 文件 | 行数 | 状态 | 能力 |
| --- | --- | --- | --- |
| `docxAdapter.ts` | 666 | ✅ v0.7.27 | `pmDocToSavePlan` + patchParagraphText + table/image shim |
| `xlsxAdapter.ts` | 139 | ✅ v0.7.27 | 公式 `cell.f` + 数字格式 `cell.z` + 颜色/字体填充 |
| `pptxAdapter.ts` | 122 | ⚠️ MVP | pptxgenjs 文本列表，**已被 pptxShapeAdapter 取代** |
| `pptxShapeAdapter.ts` | 244 | ✅ v0.7.27 新建 | text/rect/ellipse/line/picture 五种形状字节级闭环 |

### 1.4 前端编辑器

| 组件 | 行数 | 状态 | 编辑器 |
| --- | --- | --- | --- |
| `CollabDocProEditor.vue` | 704 | ✅ v0.7.27 | TipTap + 11 个扩展（表格/图片/任务列表/对齐/高亮/颜色/下划线） + 工具栏 |
| `CollabSheetEditor.vue` | 490 | ✅ v0.7.27 | Y.Map+Y.Array 网格 + `cellFormula` / `cellPercent` / `buildCell` |
| `CollabSlideKonvaEditor.vue` | 815 | ✅ **v0.7.27 本轮完工** | vue-konva + per-shape Y.Map + 1.5s debounce save |
| `CollabSlideEditor.vue` | 434 | ⚠️ 旧 MVP | pptxgenjs 文本列表，**路由已切换走 Konva** |
| `CollabAiPolishDialog.vue` | 200 | ✅ | AI 段落润色 |

### 1.5 协作层

| 文件 | 行数 | 状态 |
| --- | --- | --- |
| `useYjsCollabDoc.ts` | 105 | ✅ Yjs + y-websocket + awareness |
| `useYjsCollabDocPersistence.ts` | 97 | ✅ IndexedDB persistence，多租户 key |

---

## 2. 本轮（v0.7.27 PPT Konva 收尾）实际修改

| 文件 | 修改 |
| --- | --- |
| `pptxShapeAdapter.ts` | `emuTransformToRect` 改签名为 `{ offset?: EmuRect }`，直接读 `.offset` |
| `CollabSlideKonvaEditor.vue` | (a) 引入 `Slide` 类型；(b) `syncFromY` 的 `remote` mapper 补 `raw`；(c) 引入 `localSlidesByIndex` 用来按 index 找到 raw；(d) `savePptxShapeBytes(deck.value)` 两处加 `as unknown as PptxShapeDeck` 软断言；(e) `downloadCollabDocBytes(docId, filename)` → `(docId)`，filename 由前端构造 blob 时设定 |
| `CollabDocEditorView.vue` | `kind==='slide'` 路由切换到 `CollabSlideKonvaEditor`（带 `tenantId`） |

**vue-tsc**：collab 相关 6 → **0**（剩余 14 个均为 wiki/i18n 既有债）
**npm run build-only**：✅ built in 18.41s
**tsx --test adapters**：14/14 pass
**go test types/repo**：pass
（`internal/handler/collaborative_doc_bytes_test.go` 有 pre-existing shim 类型不匹配，与本任务无关）

---

## 3. 与 genoffice 的功能差距（已审计）

### 3.1 DOC（覆盖度约 35%）

| genoffice 能力 | genoffice 文件 | WeKnora 现状 | 备注 |
| --- | --- | --- | --- |
| 段落 patch | `docx-engine/patch.ts` | ✅ `patchParagraphText` | — |
| 段落粒度 round-trip | `apps/docs/convert.ts:1295 pmDocToSavePlan` | ✅ `pmDocToSavePlan` | 含 table/image 顶层节点 shim |
| **图片二进制嵌入** | `docx-engine/generate.ts:49 patchImageParagraphXml` | ⚠️ `pmImageToDrawingXml` 只生成 `<w:drawing>` 骨架，**未写真图片字节** | **v0.7.28 P0** |
| 表格 cell 文本 | `docx-engine/generate.ts:541 patchTableCellTexts` | ⚠️ 通过 docx-engine 走 `xml` 路径，但 tip→docx 的段落级 patch 未覆盖表格 | v0.7.28 P1 |
| **公式 / OMML** | `ommlToLatex`, `inlineMathML`, `patchMathTokens` | ❌ 未移植 | v0.7.28 P2 |
| **图表** | `docx-engine/chart.ts` (618 lines) | ❌ 未移植 | v0.7.29+ |
| **批注 / track changes** | `docx-engine/comments.ts` (未直接 grep) | ❌ 未移植 | v0.7.29+ |
| **节 / 页眉页脚** | `docx-engine/section.ts` | ⚠️ 解析支持，写不支持 | v0.7.29+ |
| **SDT（内容控件）** | `SdtShell` | ❌ 未处理 | v0.7.30+ |
| **Field patch** | `patchFieldParagraphXml` | ❌ 未暴露 | v0.7.30+ |
| **样式刷 / 高级排版** | `convert.ts:2670` 完整链路 | ⚠️ 仅 bold/italic | v0.7.30+ |

### 3.2 PPT（覆盖度约 25%）

| genoffice 能力 | genoffice 文件 | WeKnora 现状 |
| --- | --- | --- |
| 形状 round-trip | `pptx-engine/index.ts:605 openPptx` + `647 savePptx` | ✅ |
| 5 种形状（text/rect/ellipse/line/picture） | — | ✅ |
| **箭头/标注/自由形/custom geometry** | `insert.ts:143 buildSpXml` + `custgeom.ts` | ❌ |
| **形状 preset 库** | `pptx-render/preset-geometry.ts` (1548 lines) | ❌ |
| **Group 组合** | `insert.ts:538 buildGrpSpXml` | ❌ |
| **Slide masters / layouts / themes** | `master-edit.ts`, `theme-apply.ts`, `builtin-layouts.ts` | ❌ |
| **Animations** | `animation.ts`, `animation-play.ts` | ❌ |
| **Tables on slides** | `table-edit.ts`, `table-grid.ts`, `table-style.ts` | ❌ |
| **Charts on slides** | `chart-insert.ts`, `pptx-render/build-chart.ts` (3059 lines) | ❌ |
| **SmartArt** | `smartart.ts`, `smartart-layout.ts`, `dgm-hier.ts` | ❌ |
| **Notes 页** | `notes.ts` | ❌ |
| **Comments on slides** | `comments.ts:160 addSlideComment` | ❌ |
| **Format brush / arrange / align** | `align.ts`, `arrange-actions.ts` | ❌ |
| **Konva 渲染层完整移植** | `apps/slides/SlideCanvas.tsx` (1951) + `konva-adapter.ts` (1544) | ❌ |
| **文字富排版**（上下标、RTL、复杂脚本） | `pptx-render/text-layout.ts` (1617) + `metrics.ts` | ❌ |

### 3.3 SHEET（覆盖度约 10%）

| genoffice 能力 | WeKnora 现状 |
| --- | --- |
| Univer 0.25.1 + Yjs provider | ❌ **未安装**（`@univerjs/*` 不在 package.json） |
| 多 sheet tab + 跨表引用 | ❌ |
| 公式栏 + 自动重算 | ⚠️ 仅显示公式文本，未真重算 |
| 图表 | ❌ |
| 条件格式 / 数据验证 / 过滤 | ❌ |
| 透视表 / 命名区域 | ❌ |
| Rust xlsx sidecar (EE) | ❌（改 SheetJS 解析 + Univer 渲染） |

当前 SHEET 编辑器是手写 Y.Map 网格 + xlsxAdapter 字节回写，**不是 Univer**。

### 3.4 协作层（覆盖度约 50%）

| genoffice 能力 | WeKnora 现状 |
| --- | --- |
| 整文档 Yjs 二进制 | ✅ |
| DOC per-paragraph Y.Text | ⚠️ 整 ProseMirror Y.XmlFragment，未做 per-paragraph |
| PPT per-shape Y.Map | ✅ |
| SHEET per-cell Y.Map | ⚠️ 整 Y.Array<rowKey, Y.Map<colKey, cell>>，不是 Univer 原生 |
| 用户光标 / 选区广播 | ⚠️ awareness 只发了 user，**未发 cursor/selection** |
| 离线 / 重连 merge | ✅ IndexedDB persistence |
| 撤回 / 重做 UI | ❌ Yjs 自带 undoManager 未接到 UI |
| 批注 / 讨论 | ❌ |
| 操作历史 / 审计 | ❌ |

---

## 4. v0.7.28 → v0.9.0 路线图

### v0.7.28 — 文档稳定性 + PPT 进阶（1 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 1 | DOC 图片二进制嵌入（真 `<w:drawing>` + image bytes） | `docxAdapter.ts` + `pmImageToDrawingXml` | 上传含图片 .docx → 编辑图片 alt → 下载 → 解压见正确 PNG |
| 2 | DOC 表格 cell 级 patch | `docxAdapter.ts` + TipTap table cell | 在表格内改字 → 单元格 XML 局部 patch |
| 3 | PPT 形状 preset 库（arrow / callout / triangle / star） | `pptxShapeAdapter.ts` + 工具栏按钮 | 添加箭头，保存后 Microsoft PowerPoint 识别 |
| 4 | PPT 字体/字号/对齐富文本 | `CollabSlideKonvaEditor.vue` 右侧 inspector + `setElementTextBodyProps` | 编辑富文本不丢其他属性 |
| 5 | Konva 渲染层接入 `pptx-render/buildRenderSlide` | 新建 `pptxRenderAdapter.ts` | 只读预览与编辑器共用 |
| 6 | PPT 形状 z-order / 锁定 / 复制粘贴 | 工具栏 + `reorderElement` | — |
| 7 | 路由 + 工具栏 + i18n 完善 | — | — |

### v0.7.29 — 协作体验（1 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 1 | DOC per-paragraph Y.Text | `pmDocToSavePlan` 改造 + TipTap `Collaboration` field | 两人同时改同段不同位置不丢字 |
| 2 | awareness cursor / selection 广播（DOC + PPT） | `useYjsCollabDoc.ts` | 远端光标位置可见 |
| 3 | Yjs undoManager 接 UI | 新增按钮 + 快捷键 | Ctrl+Z 撤销自己最近一次操作 |
| 4 | 批注 (DOC/PPT 通用) | 新建 `commentsAdapter.ts` + DB `collab_doc_comments` 表 | 选中段落 → 添加评论 → 多人可见 |
| 5 | SHEET cursor + 选区广播 | `CollabSheetEditor.vue` | — |

### v0.7.30 — SHEET Univer 化 + 高级能力（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 1 | 装 `@univerjs/presets` + UI + sheets + yjs | `package.json` | — |
| 2 | 用 Univer 替换手写 Y.Map 网格 | `CollabSheetEditor.vue` | 多 sheet tab + 真公式重算 |
| 3 | 保留 xlsxAdapter 作为字节导出通道 | — | 编辑 → 下载 .xlsx 字节一致 |
| 4 | SHEET charts / conditional formatting | Univer preset 内置 | — |
| 5 | DOC charts（chart.ts 移植） | `pptx-engine` 同源 | — |
| 6 | PPT tables + notes | `table-edit.ts` + `notes.ts` | — |
| 7 | DOC 节 / 页眉页脚编辑 | `section.ts` 写路径 | — |

### v0.8.0 — 飞书级别（3-4 周）

| # | 任务 | 验收 |
| --- | --- | --- |
| 1 | 完整 Konva 渲染层（搬运 `konva-adapter.ts` 1544 行） | 复杂 PPT 视觉效果与 PowerPoint 接近 |
| 2 | Slide masters / themes / layouts | 改母版 → 所有幻灯片同步 |
| 3 | PPT 动画（`animation-play.ts`） | 播放预览 |
| 4 | SmartArt + 复杂图表 | — |
| 5 | DOC 目录 / 大纲视图 / 引用 | — |
| 6 | 全文搜索（DOC + PPT 文本 + SHEET cell） | — |
| 7 | 模板库 + 一键创建 | — |
| 8 | 历史版本 diff / 回滚 | — |
| 9 | 导出 PDF（复用 docparser） | — |
| 10 | Webhook + 操作审计 | — |

---

## 5. 立即可执行（今天-明天）的下一步

1. **手测 PPT Konva 编辑器**（浏览器）：
   - 上传一份本地 .pptx
   - 添加 / 拖动 / 缩放形状
   - 1.5s 后下载确认字节保存
   - 打开两个 tab 协作，验证 Y.Map 同步

2. **后端补一个 retention 任务**（v0.7.28 内）：
   - `internal/application/service/collaborative_doc.go` 加 `PurgeFilesOlderThan(ctx, n)` 方法
   - 定时任务每天清理 > 30 天的非最新 `collab_doc_files` 行
   - 防止 `collab_doc_files` 无界增长

3. **CI 增加 vue-tsc 检查脚本**：
   ```bash
   ./node_modules/.bin/vue-tsc --build 2>&1 | grep -E "error TS" \
     | grep -iE "collab|adapters" && exit 1
   ```

4. **补前端 adapter 测试覆盖**（v0.7.28 内）：
   - `pptxShapeAdapter`：上传 .pptx → 添加形状 → 保存 → 再打开看到新形状
   - `CollabSheetEditor`：公式 `=SUM(A1:A10)` → 下载 → cellExtras 含 formula

---

## 6. 风险与权衡

1. **Univer 接入成本**：装包后要做 `useYjsCollabDoc` 与 Univer 的 namespace 桥接（Univer 默认 key 是 `default`，需要映射到 `slide:{docId}`）。预计 2-3 天。
2. **Konva 性能**：8000 个形状以上时 `v-transformer` 重绘会卡。v0.8.0 需要做 `Konva.Layer` 分层 + requestAnimationFrame 批量更新。
3. **DOC 二进制图片**：现在 `pmImageToDrawingXml` 没写真图片字节，需要 `docx-engine` 暴露的 image part API（genoffice 在 `parse.ts` 有，但路径在 `src/editor/engines/docx-engine/` 需要审计），预估 1 天。
4. **历史版本 diff**：需要把 `collab_doc_files` 与 `collab_doc_snapshots` 关联起来做 unified diff。SHEET/DOC 都用 docparser `diff` 端点。
5. **保留 OSS 兼容性**：`SaveFile` 用了 `Version=0` auto-increment，老数据可能没有 version 字段；需要在 `CurrentVersion` fallback 时返回 1 而非 0。

---

## 7. 文档归档

- `docs/collab-docs/README.md` — 用户面介绍
- `docs/collab-docs/PORT_PLAN.md` — 初始移植计划
- `docs/collab-docs/PORT_PLAN_V2.md` — 分阶段任务清单（已大半完成）
- `docs/collab-docs/STATUS.md` — v0.7.25 现状盘点
- **`docs/collab-docs/ANALYSIS_V1.md`（本文）** — v0.7.27 后整盘盘点 + 路线图

