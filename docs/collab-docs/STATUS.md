# WeKnora 飞书文档协作能力 — 现状分析（2026-08-31）

> 本文档盘点 WeKnora 当前已实现的「飞书/腾讯文档」协作能力与 genoffice 目标能力之间的差距，
> 作为后续 PORT_PLAN_V2 的输入。已与仓库内 `frontend/`、`internal/`、`migrations/` 实际文件交叉验证。

---

### v0.7.42 — genoffice 三件套 gateway copy (in-progress)

**目标**: 充分学习 genoffice, copy 其 xlsx gateway 到 WeKnora, 实现飞书级 SHEET 能力。

**已完成**:
- copy 9 个文件 (从 `/Users/louloulin/appx/genoffice/apps/sheets/src/gateway/`) 到 `frontend/src/editor/adapters/`:
  - xlsxFilter.ts (筛选)
  - xlsxSparkline.ts (迷你图)
  - xlsxCf.ts (条件格式)
  - xlsxDv.ts (数据验证)
  - xlsxHyperlinks.ts (超链接)
  - xlsxProtection.ts (保护)
  - xlsxTheme.ts (主题)
  - xlsxDefinedNames.ts + futureFunctions.ts (命名区域 + 未来函数)
  - xlsxChart.ts (图表, 含 desktop-api 类型剥离)
- 新增 `xlsxWorksheetIo.ts` 适配层 (zip + worksheet XML 读写)
- 新增 `xlsxFreeze.test.ts` 验证冻结窗格 (6/6 pass)
- 新增 `xlsxVendored.test.ts` 验证 9 个 copy 文件 (34/34 pass)
- 重构 `xlsxFreeze` 走 `xlsxWorksheetIo` (从 ~70 行 → ~30 行, 去重 zip 逻辑)
- **接入 CollabSheetEditor UI** (CollabSheetEditor.vue: 793 → 1174 行):
  - 工具栏加「冻结」下拉菜单 (首行/首列/两者/取消)
  - 工具栏加「筛选」按钮 (modal: 列号 + 值列表)
  - 工具栏加「条件格式」按钮 (modal: 列号 + 比较 + 值)
  - 工具栏加「数据验证」按钮 (modal: 列号 + 类型 + 值)
  - 保存 (flushSave) 与下载 (exportXlsx) 自动应用全部规则到 .xlsx 字节
  - 加载时从 .xlsx 字节恢复冻结状态 (readXlsxFreeze)
- **接入 CollabDocProEditor 公式 UI**:
  - 工具栏加「公式」按钮 → 弹 modal: LaTeX 输入 + MathML 预览
  - 走 docxAdapter.latexToDocxMath → mathDisplayParagraph → docxMathToMathML
  - 插入为带 data-latex 属性的 HTML 块, source 保留用于 round-trip
- **新测试** docxMathAdapter.test.ts (8/8 pass): latexToDocxMath / mathDisplayParagraph / docxMathToMathML / docxMathToLatex / readMathFragmentsInParagraph / readMathTokensInParagraph
- 总体: 107/107 adapter test 全过, vue-tsc 0 个 CollabSheetEditor / CollabDocProEditor 错误

**架构原则 (继承)**:
- 引擎能力 (OOXML 序列化) 走 adapter, 不直接调内部 API
- 所有 IO 操作经 `xlsxWorksheetIo.ts` 的 zip 层
- genoffice copy 文件头注释标注来源, 方便 vendor 同步

**未 copy** (改造工作量大):
- docs/equation.ts (依赖自定义 TipTap schema: docInlineMath/docProtected)
- sheets/in-memory-workbook.ts + workbook-dsl.ts (Univer 模型, 跟 Univer 紧耦合)
- docs/toc-refresh.ts + page-break.ts (依赖自定义 pageBreakBefore schema)
- docs/comments.ts (依赖 genoffice 的 CommentInfo + revisions 模块)

**已完成接入 UI** (v0.7.42):
- ✅ SHEET: 冻结/筛选/条件格式/数据验证 (4 个能力)
- ✅ DOC: /math 触发 LaTeX 公式 (1 个能力)
- ✅ PPT: 动画 (11 效果 + 3 触发, v0.7.29 已完成)

**剩余 P2** (后续版本):
- 接入 PPT 11 类图表编辑 UI (xlsxChart 已有 applyChartEdit API, 仅缺前端)
- 接入 PPT SmartArt (移植 genoffice dgm-hier 295 行)
- 接入 SHEET 迷你图 / 超链接 / 主题 / 命名区域 UI
- 接入 DOC 表格 cell 颜色/合并 round-trip UI
- Y.Map 实时同步 (目前 SHEET feature 状态为本地 ref, 需推到 Y.Map 让多客户端同步)

---


## 1. 目标与定位

| 项 | 说明 |
| --- | --- |
| **目标** | 把 `/Users/louloulin/appx/genoffice`（Apache-2.0 完整 Office 三件套）的能力移植进 WeKnora，提供类飞书 / 腾讯文档 / Notion 的 DOC/SHEET/SLIDE 三类多人实时协作 |
| **当前代号** | v0.7.25 `collaborative_docs` |
| **核心 CRDT** | Yjs（同 wiki 实时分支共用 y-websocket wire protocol） |
| **编辑器栈** | DOC → TipTap + docx-engine；SHEET → Univer 0.25.1；SLIDE → Konva + pptx-engine/pptx-render |

---

## 2. 已经完成（v0.7.25 MVP 已落地）

### 2.1 后端（Go）—— 已编译通过

```
internal/types/collaborative_doc.go              # 类型 + 三种 DocKind
internal/types/interfaces/collab_doc.go          # 三个仓库接口
internal/types/collaborative_doc_test.go         # 3 个单元测试通过
internal/application/repository/collab_doc.go    # Postgres/SQLite 双 repo
internal/application/service/collaborative_doc.go           # 业务层
internal/application/service/collaborative_doc_authz.go     # ACL 最小接口
internal/handler/collaborative_doc.go            # REST（7 个端点）
internal/handler/collaborative_doc_ws.go         # Yjs y-websocket 升级
internal/handler/collaborative_doc_sync.go       # Sync handler
internal/router/routes_collaborative_doc.go      # 路由注册
internal/container/container.go                  # ~line 533 已注入
internal/router/router.go                        # RegisterCollabDocRoutes 已挂载
migrations/sqlite/000039_collaborative_docs.up.sql # sqlite 迁移
```

| 端点 | 方法 | 说明 |
| --- | --- | --- |
| `/collaborative-docs` | POST / GET | 创建 / 列表 |
| `/collaborative-docs/:id` | GET / PATCH / DELETE | 元数据 |
| `/collaborative-docs/:id/archive` | POST | 软删除 |
| `/collaborative-docs/:id/presence` | GET | 在线协作者 |
| `/collaborative-docs/:id/export` | GET | 当前仅返回静态 markdown 占位（**待替换为 docx/pptx/xlsx 字节流**） |
| `/collaborative-docs/:id/realtime` | GET (WS) | Yjs y-websocket 二进制帧 |

表结构：
- `collaborative_docs` — 元数据（id、tenant_id、kb_id、title、doc_kind、share_token、visibility…）
- `collab_doc_snapshots` — Yjs 压缩字节（ydoc_state bytea + vector_clock bytea + size_bytes）
- `collab_doc_sessions` — 在线协作者（与 wiki_realtime_sessions 同构）

### 2.2 前端（Vue 3）—— MVPs 已在，但只是壳

| 文件 | 状态 | 备注 |
| --- | --- | --- |
| `src/api/collabDoc/index.ts` | ✅ 完成 | REST + WS URL helper，已用 `@/utils/request` |
| `src/composables/useYjsCollabDoc.ts` | ✅ 完成 | 镜像 `useYjsWiki`，key=doc_id |
| `src/components/collab/CollabDocEditor.vue` | ⚠️ TipTap MVP | 仅 StarterKit + Collaboration，未挂载 docx-engine |
| `src/components/collab/CollabSheetEditor.vue` | ⚠️ Yjs MVP | Y.Array+Y.Map 网格，**非 Univer** |
| `src/components/collab/CollabSlideEditor.vue` | ⚠️ Y.Array MVP | 标题+要点列表，**非 Konva** |
| `src/views/collab/CollabDocListView.vue` | ✅ 完成 | 飞书列表视觉 |
| `src/views/collab/CollabDocEditorView.vue` | ⚠️ 装载三种编辑器 | auth store 属性名错（详见 §3） |
| `src/router/index.ts` | ✅ 完成 | `/collab-documents` + `/collab-documents/:id` |
| `src/i18n/locales/en-US.ts` | ⚠️ 重复 key | `google_workspace` 后少逗号 |
| `src/i18n/locales/zh-CN.ts` | ✅ 完成 | collabDoc.* |

### 2.3 genoffice 引擎已拷入，但未对接

```
frontend/src/editor/engines/docx-engine/   # 完整 25 个文件，已将 @genoffice/* 改相对路径
frontend/src/editor/engines/pptx-engine/   # 完整 41 个文件，含 node:crypto/node:zlib 需 polyfill
frontend/src/editor/engines/pptx-render/   # 完整 13 个文件，纯 TS
frontend/src/editor/engines/file-parse/    # 已删除 Node-only 文件，保留 docx.ts/pptx.ts/xlsx.ts/index.ts
frontend/src/editor/adapters/docxAdapter.ts # 浏览器侧 wrapper，但 saveDocx 调用签名错误
```

已安装的 npm 包：`jszip fast-xml-parser utif2 pptxgenjs pdfjs-dist ppt-to-text word-extractor cfb opentype.js`

---

## 3. 当前阻塞 — 已实测的错误清单

实测 `cd frontend && ./node_modules/.bin/vue-tsc --build 2>&1 | grep error TS` 的结果中，
**仅以下属于本次任务的修复项**（其它 wiki / syncBlock 等为已存在的历史债）：

```
src/api/collabDoc/index.ts(93,33): ArrayBuffer|SharedArrayBuffer → BlobPart（new Blob 接受 Uint8Array OK）
src/components/collab/CollabDocEditor.vue:
    (34,31)  @tiptap/extension-collaboration 未安装（npm 包已加在 package.json，需 install）
    (35,37)  @tiptap/extension-collaboration-cursor 同上
    (55,3)   boolean|undefined → boolean（v 转换）
    (56,3)   peers 类型合并错误
    (57,3)   error 类型合并错误
    (74,5)   null → Editor|undefined（teardown 时 editor.value = null）
src/components/collab/CollabSheetEditor.vue:
    (64,20)  yjs 未安装（package.json 有但 node_modules 缺）
    (93-95)  类型合并（同 Doc）
src/components/collab/CollabSlideEditor.vue:
    (73,20)  yjs 同上
    (108-110) 类型合并（同 Doc）
    (122,39) toArray().map(m => ...) m 隐式 any
src/composables/useYjsCollabDoc.ts:
    (9,20)  yjs 模块未找到
    (10,35) y-websocket 模块未找到
src/editor/adapters/docxAdapter.ts:
    (68,33) saveDocx 第二参应为 SaveBlock[] 而非 { blocks, ...opts }（签名错）
src/i18n/locales/en-US.ts:
    (2919,5) 和 (2943,5) 重复 key（google_workspace 后缺逗号）
src/views/collab/CollabDocEditorView.vue:
    (57,29) authStore.accessToken 不存在（应为 token）
    (58,41) authStore.user.name 不存在（应为 username）
```

> 其它 `useYjsWiki.ts`、`SyncBlockNode.ts`、`AIAssistantPanel.vue` 的错误属于历史债，本计划不修复。

---

## 4. 差距 — 当前 MVP 与「飞书文档」目标

| 维度 | 当前 MVP | 飞书 / 腾讯文档目标 | 需要做的事 |
| --- | --- | --- | --- |
| **DOC 编辑** | TipTap StarterKit，Y.Text 二进制更新 | 富文本 + 段落锚定到 docx-engine Block.docxIndex | 在 TipTap 中维护 Block 镜像；onUpdate → `patchParagraphText` → `saveDocxBytes` |
| **DOC 持久化** | 仅 Yjs 状态写到 collab_doc_snapshots.ydoc_state | 上传/下载真实 .docx 字节流 | 实现 `downloadCollabDocBytes`（后端新增端点），前端 `openDocx` 渲染后回写 |
| **SHEET 编辑** | Y.Map<rowKey, Y.Map<colKey, string>> 网格 | Univer：公式、单元格格式、跨表引用、图表 | 替换为 `@univerjs/presets + @univerjs/ui + @univerjs/sheets` |
| **SLIDE 编辑** | 标题+要点列表（自行 Y.Array） | Konva 舞台、形状、主题、母版、动画 | 接入 `pptx-engine` 的 `openPptx/savePptx` + `pptx-render` 的 `buildSlide/RenderTree`，用 vue-konva 渲染 |
| **协作粒度** | 整文档 Yjs 二进制 | DOC 段落级 Y.Text；SLIDE 形状级 Y.Map；SHEET Univer native | 替换抽象：DOC → `Y.XmlFragment` 或 per-paragraph `Y.Text` |
| **导入/导出** | Export 端点返回静态 markdown | .docx/.pptx/.xlsx 字节流导入 + 导出 | 后端按 doc_kind 走 docparser 或 python-pptx |
| **KB 同步** | `/sync-to-kb` 路由占位 | 解析 .docx → chunk → 入 KB | 接 docparser `/chunk` |
| **AI Agent** | 无 | 选段 → 「问 AI」 → 段落级润色 | docx-engine block JSON 喂给现有 chat，润色后 patchParagraphTexts |
| **离线编辑** | 无 | 弱网缓冲 + 离线快照 | Service Worker + IndexedDB Yjs provider |

---

## 5. 关键技术风险（实测确认）

1. **pptx-engine 含 Node-only 依赖**
   - `packages/pptx-engine/src/zip.ts:10` — `import { createHash } from 'node:crypto'`
   - `packages/pptx-engine/src/index.ts:666` — `await import('node:fs')`
   - `packages/pptx-engine/src/media-insert.ts:14` — `import { deflateSync } from 'node:zlib'`
   - `savePptxToFile`（仅 Electron 场景使用），浏览器可不打包
   - 浏览器侧需 polyfill：`globalThis.crypto.subtle` + `CompressionStream('deflate-raw')`

2. **file-parse 删除的安全风险**
   - 已删除 `parse.ts`（用 `node:fs/promises`）、`pdf.ts`（Electron）、`doc.ts`/`ppt.ts`/`vendor.ts`
   - 保留 `docx.ts/pptx.ts/xlsx.ts/index.ts` —— 纯 JSZip 实现，浏览器可跑
   - 浏览器侧 PDF 解析改用 `pdfjs-dist`（已装）

3. **docx-engine `saveDocx` 签名**
   ```ts
   export async function saveDocx(
     parsed: ParsedDocFull,
     finalBlocks: SaveBlock[],          // ← 第二参直接是 SaveBlock[]
     options: SaveOptions = {},
   ): Promise<Uint8Array>
   ```
   当前 `docxAdapter.saveDocxBytes` 错用 `{ blocks, ...opts }`，需要改成 `(doc.parsed, saveBlocks)`。

4. **TipTap ↔ docx-engine 桥接**
   - genoffice 的做法：PM doc ↔ SaveBlock[] 双向序列化（见 `apps/docs/src/renderer/editor/convert.ts`）
   - 移植到 Vue 端需重新实现 `pmDocToSavePlan`，依赖 `blocksToPmDoc`
   - 段落粒度协作：使用 `@tiptap/extension-collaboration` 的 `field` 配置，把每个 paragraph 映射成 Y.Text

5. **vue-konva 与 react-konva 的 API 差异**
   - genoffice 大量使用 react-konva 的 `<Stage><Layer><Rect /></Layer></Stage>`
   - vue-konva 形状组件 API 接近，但 Transformer / 自定义 Shape 需要手写 `configFunc`
   - 建议先做只读 Konva 渲染（buildSlide → Konva tree），再做交互层

6. **Univer 与现有 Yjs WS 兼容**
   - Univer 0.25.1 自带 `@univerjs/yjs` provider，需要创建对应的 `UniverDoc` namespace
   - 与现有 `useYjsCollabDoc` 共用一个 Y.Doc，但 key 不同（univer 默认 key 是 `default`）

---

## 6. 已确认可用的资产清单

| 资产 | 路径 | 价值 |
| --- | --- | --- |
| Wiki realtime 前例 | `frontend/src/composables/useYjsWiki.ts` + `internal/handler/wiki_realtime*.go` | CRDT + WS 模式的最佳参照 |
| Tiptap 已有渲染 | `frontend/src/components/wiki/WikiTiptap*.vue` | StarterKit + Collaboration 接线已成熟 |
| docparser Python 服务 | `docreader/` + `/chunk` 端点 | `.docx → markdown chunk`，用于 KB 同步 |
| 已有 KB/Chunk 管线 | `internal/application/service/chunk.go` | Re-chunk 流水线可复用 |
| 鉴权 / RBAC | `internal/application/service/rbac.go` | 已在 routes_collaborative_doc.go 用 OwnedWikiKBOrAdmin |
| i18n | `frontend/src/i18n/locales/{en-US,zh-CN}.ts` | `knowledgeBase.collabDoc.*` 已埋点 |

---

## 7. 下一步入口（与 PORT_PLAN_V2 一一对应）

1. **Phase 0（修脚手架）**：npm install 补齐 yjs/y-websocket/tiptap-collaboration；修 docxAdapter.saveDocx 签名；修 i18n 重复 key；修 authStore 属性名；修 CollabSlideEditor m 类型。
2. **Phase 1（DOC 真编辑）**：实现 `CollabDocProEditor.vue` + `pmDocToSavePlan`，跑通「上传 .docx → 编辑 → 下载 .docx」闭环。
3. **Phase 2（PPTX 真渲染）**：polyfill node:crypto/zlib；写 Konva adapter；接入 vue-konva。
4. **Phase 3（Univer 替换 SHEET）**：用 `@univerjs/presets`，与现有 Y.Doc 共存。
5. **Phase 4（协作粒度细化）**：DOC per-paragraph Y.Text；SLIDE per-shape Y.Map。
6. **Phase 5（KB 同步 / AI 接入）**：docparser `/chunk` + chat 流；选段 → 段落级润色。
7. **Phase 6（离线 + 分享）**：IndexedDB persistence；share-token 路由。

每个 phase 的具体 task 见 PORT_PLAN_V2.md。

---

## 8. v0.7.25 → v0.7.37 增量盘点（2026-09-01）

承接 §2，已落地的版本号（按 git log）：

| 版本 | commit | 关键产出 |
| --- | --- | --- |
| v0.7.25 | `5258cc23` | Collab Docs Foundation — 4 表 + 14 端点 + WS + TipTap MVP |
| v0.7.26 | `ced4b708` + `84060c44` + `edfdef25` | CollabDocFile 字节存储 + .pptx/.xlsx adapters + i18n |
| v0.7.27 | (P10-P12) | DOC 工具栏（11 TipTap 扩展）+ XLSX 公式 + IndexedDB |
| v0.7.30 | (本审计期) | PPT 演讲者备注 + PPT 选区广播 + 后端审计日志 |
| v0.7.31 | (本审计期) | SHEET 选区广播 + Slides Region backend |
| v0.7.32 | (本审计期) | 引擎能力适配层（6 大类，35+ 函数） |
| v0.7.33 | `3d185c29` | KG graph query API |
| v0.7.34 | `2c3f6902` | Docs × KB integration |
| v0.7.35 | `b18b51cd` + `a826cec5` | threaded comments + share token + PPT shape editor |
| v0.7.36 | `88d5083f` + `354fdac2` | Enterprise Search + MindMap |
| v0.7.37 | `e30e0e8c` + `ed1d62a9` | Slides backend + 引擎 adapter 收尾 |

适配层测试：**51 pass / 51 total**（10 个 .test.ts 文件覆盖 35+ 函数）。

详细差距分析见 [`ANALYSE_V2.md`](./ANALYSE_V2.md)，路线图见 [`ROADMAP_V2.md`](./ROADMAP_V2.md) 第七章。

---

## 9. v0.7.38 收尾增量 + v0.7.41+ 路线（2026-09-01）

承接 §8，已落地的本审计期变更：

| 版本 | 改动文件 | 关键产出 |
| --- | --- | --- |
| v0.7.38 | `internal/middleware/collab_ratelimit.go` + `_test.go` | 限流中间件（per-tenant/per-doc/per-IP），7 tests pass |
| v0.7.38 | `internal/types/webhooks.go` + `interfaces/webhooks.go` + `internal/handler/webhooks.go` + `internal/router/routes_webhooks.go` | WebHook 系统（11 events + HMAC-SHA256 + retry），8 tests pass |
| v0.7.38 | `internal/application/repository/webhooks.go` + `service/webhooks/` | WebHook 持久化 + delivery retry |
| v0.7.38 | `migrations/sqlite/000051_webhooks.{up,down}.sql` + `mysql/000046_*.sql` | WebHook 表迁移（双库） |
| v0.7.38 | `internal/types/audit_log.go` | 9 个新 AuditAction 常量（`collab_doc.*` + `slide_deck.*`） |
| v0.7.38 | `internal/handler/collaborative_doc*.go` + `internal/application/service/collaborative_doc.go` | 分享密码 + 过期（hash/verify/expired 全部接入） |
| v0.7.38 | `migrations/sqlite/000053_collab_share_protect.{up,down}.sql` + `mysql/000047_*.sql` | `share_password_hash` + `share_expires_at` 迁移 |
| v0.7.38 | `frontend/src/composables/useYjsCollabDoc.ts` + 3 个 `.vue` | DOC range awareness + SHEET cell selection + PPT format toolbar |
| v0.7.38 | `frontend/src/components/collab/CollabSlideDeckEditor.vue` + `frontend/src/views/collab/CollabSlidesView.vue` + `frontend/src/api/slides/` | Slides 后端可视化编辑器接入 |
| v0.7.38 | `docs/collab-docs/ANALYSE_V2.md` + `STATUS.md` + `ROADMAP_V2.md` | 第 3 轮盘点 |
| v0.7.38 | `.gitignore` | 排除 `desktop/` 329 MB 二进制 |

### 9.1 测试状态实测（2026-09-01）

```
$ go build ./...
# 全干净（仅 ld warning 关于 desktop 二进制）
$ go test ./internal/application/repository/ -count=1 -run "TestCollab"
ok  	github.com/Tencent/WeKnora/internal/application/repository	0.492s
$ go test ./internal/middleware/ -count=1 -run "TestCollab"
ok  	github.com/Tencent/WeKnora/internal/middleware	0.608s
```

### 9.2 关于"share test 编译失败"误报澄清

之前 handoff 文档误将 `internal/application/service/` 包 build 失败归因于新增的
`collaborative_doc_share_test.go`。经 `git stash` 验证：

- 该 build 失败是 **PRE-EXISTING** 的 wiki 测试 helper 名冲突
- 冲突点（8 个）：
  - `contains` — `audit_export_test.go` ↔ `knowledge_span_tracker.go` ↔ `wiki_template_test.go`
  - `fakeClock` — `tool_cache_test.go` ↔ `audit_log_test.go`
  - `stubAuditLogService` — `wiki_audit_harness_test.go` ↔ `audit_export_test.go`
  - `stubWikiAclRepo` — `wiki_audit_harness_test.go` ↔ `wiki_acl_test.go`
  - `stubBacklinksCacheRepo` / `newStubBacklinksCacheRepo` — `wiki_audit_harness_test.go` ↔ `wiki_page_backlinks_v2_cache_test.go`
- 修复策略（v0.7.41.1）：给上述 helper 加 `wiki` / `audit` / `cache` 前缀，包级不再 build fail
- **新增的 `collaborative_doc_share_test.go` 函数名（`TestShare*` + `newShareTestDB`）不与任何既有冲突**

### 9.3 下一步 — 见 [PLAN_V3.md](./PLAN_V3.md)

新路线图 v0.7.41 → v0.7.45（5 个版本 / 10 周）已在 `PLAN_V3.md` 第 3 章详述。

---

## 10. v0.7.41 Build #47 收尾（2026-09-01）

承接 §9，把 v0.7.41 完整收尾：

### 10.1 后端

- **测试构建修复**：8 个 PRE-EXISTING 的 wiki_*_test.go helper 名冲突(contains/fakeClock/stubAuditLogService/stubWikiAclRepo/stubBacklinksCacheRepo 等)+ interface drift。已通过 rename + auto-generated stub 解决。`go test -c ./internal/application/service/` 现在干净编译。
- **`internal/types/webhooks.go`**：新增 `WebhookEventCollabDocUnshared = "collab.doc.unshared"` 常量 + allow-list 条目。
- **`internal/application/service/collaborative_doc.go`**：
  - `EnableShare` 末尾追加 `s.publishEvent(ctx, "collab.doc.shared", {doc_id, doc_kind, password_protected, expires_at})`
  - `DisableShare` 末尾追加 `s.publishEvent(ctx, "collab.doc.unshared", {doc_id, doc_kind})`
- **测试**：`TestShareVerifyPasswordRoundTrip`、`TestShareExpiredReturnsTrue` ✅ pass。

### 10.2 前端

- **`frontend/src/api/collabDoc/index.ts`**：
  - `CollabDoc` 类型新增 `share_expires_at?: string | null` 和 `share_password_protected?: boolean`
  - 新增 `enableCollabDocShare(id, req)` / `disableCollabDocShare(id)` / `downloadCollabDocShare(token, password?)`
  - `downloadCollabDocShare` 在密码存在时自动加 `X-Share-Password` header
- **`frontend/src/components/collab/CollabSharePasswordPanel.vue`**（423 行）：完整的 owner-side 面板
  - 启用 / 禁用 / 复制链接 / 6+ 字符密码校验 / 可选过期时间
  - i18n: en-US + zh-CN
  - data-testid 全部覆盖（便于 E2E）
- **`frontend/src/views/collab/CollabDocEditorView.vue`**：在 sidebar 加「分享设置」开关 + 嵌入 `CollabSharePasswordPanel`
- **`frontend/src/views/collab/CollabDocShareView.vue`**：公开访问 `/collab-documents/share/:token`
  - 401/403 自动弹出密码输入框
  - 通过新 `downloadCollabDocShare` API 下载（带密码 header）
- **`frontend/src/i18n/locales/{en-US,zh-CN}.ts`**：`collabDoc.share.*` 全套键
- **`frontend/src/components/collab/__tests__/CollabSharePasswordPanel.test.ts`**（8 tests）：
  - 密码长度规则、handler vs panel floor 对齐、URL 编码、payload 形状 ✅ 8/8 pass

### 10.3 验证

- `go build ./...` ✅ 干净
- `go test -c ./internal/application/service/` ✅ 134MB test binary 编译干净
- `tsx --test src/editor/adapters/__tests__/*.test.ts` ✅ 51/51 pass
- `tsx --test src/components/collab/__tests__/CollabSharePasswordPanel.test.ts` ✅ 8/8 pass
- `vue-tsc --build`：我新增的 3 个文件零错误（其他 wiki/mindmap/i18n 既有 31 errors 与本任务无关）

### 10.4 下一步 — v0.7.42 SHEET 飞书级

- 条件格式：cellRules JSON in Y.Map
- 数据验证：dropdown + number range
- 冻结窗格
- 自绘 SVG 图表：bar/line/pie
- SHEET 公式栏 UI 完善


---

## 11. v0.7.43 SHEET 公式栏完整化（2026-09-01）

承接 §10.4，把 SHEET 公式栏升级到飞书 SHEET 基线水平。

### 11.1 背景

- 上一轮把 `genoffice` 9 个 xlsx gateway 已 copy 到 `frontend/src/editor/adapters/`，并已接入冻结 / 筛选 / 条件格式 / 数据验证 / 迷你图 的 modal UI（见 §9）。
- 公式栏仍停留在 v0.7.38 的最小实现：只支持 `SUM / AVERAGE / COUNT / MIN / MAX` + 单元格算术，没有跨表引用、没有 `IF / COUNTIF / VLOOKUP / CONCAT / LEN / ROUND / TEXT` 等基础函数，也没有单元测试覆盖。

### 11.2 工作

- **新建独立模块** `frontend/src/editor/formula/sheetFormula.ts`（378 行，纯函数）：
  - 类型：`SheetLookup = ReadonlyMap<string, string[][]>`。
  - 助手：`colNameToIndex`（A→0、AA→26）/ `resolveSheetRef`（支持 `'Sheet 2'!A1` 引号形式）/ `resolveCellRef` / `splitFormulaArgs`（尊重嵌套括号与字符串）/ `collectRangeValues` / `collectRangeStrings` / `stripStringQuotes`。
  - 引擎：`evaluateFormula(expr, currentSheet, lookup): string`，支持：
    - **聚合**：`SUM / AVERAGE / AVG / COUNT / COUNTA / MIN / MAX`
    - **条件**：`COUNTIF`（含 `>5 / =3 / !=x` 运算符）/ `SUMIF`
    - **查找**：`VLOOKUP(lookupValue, tableRange, colIdx)`（含 `#N/A` 兜底）
    - **字符串 / 数值**：`CONCAT / CONCATENATE / LEN / ROUND / ABS / TEXT`（支持 `0.00 / 0% / 0.00%`）
    - **逻辑**：`IF`（支持 `A1>B2` 类单元格比较 + 字面量比较 + THEN/ELSE 字面量字符串）
    - **Token 算术**：`A1 + Sheet2!B2 * 2`（跨表）
  - 修正原 v0.7.38 `resolveCellRef` 正则 bug：`/^([A-Z]+)(d+)$/i` → `/^([A-Z]+)(\d+)$/i`，多位数行号（`A10`）与多字母列号（`AB1`）现在能正确解析。
  - 修正原 `IF` 单元格比较只 resolve 左边的 bug：现在左右两边都走 `resolveAnyCellRef`。

- **接入 CollabSheetEditor.vue**：
  - 新增 `import { evaluateFormula as evalFormula, type SheetLookup } from '@/editor/formula/sheetFormula'`
  - 新增 `activeSheetName` computed（之前没有，公式栏需要当前 sheet 名）
  - 新增 `sheetLookup` computed
  - `formulaResult` 改为新签名：`evalFormula(value, activeSheetName.value, sheetLookup.value)`
  - 删除原 in-file `colNameToIndex` / `resolveCellRef` / `parseRangeArgs` / `evaluateFormula`（共 -48 行），避免双份实现

- **新测试** `frontend/src/editor/formula/__tests__/sheetFormula.test.ts`（21 个 case）：
  - 列名解析 / sheet 名解析 / 单元格解析（含越界）
  - 单 cell / 2D 范围 / 跨表范围 / 字符串范围
  - `SUM / AVERAGE / COUNT / COUNTA / MIN / MAX`
  - `COUNTIF / SUMIF`（运算符 + 字符串匹配）
  - `IF`（字面量 + 单元格比较）
  - `CONCAT / LEN / ROUND / ABS / TEXT`
  - `VLOOKUP`（命中 + `#N/A`）
  - Token 算术 + 跨表 token 算术
  - 边界：空 / 未知函数 / 未知 sheet 抛错

### 11.3 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/formula/__tests__/*.test.ts \
    src/editor/adapters/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 128
ℹ pass 128
ℹ fail 0

$ ./node_modules/.bin/vue-tsc --build --force
# 0 errors in sheetFormula.ts / CollabSheetEditor.vue
# 41 total errors — all pre-existing wiki/mindmap/i18n/slides/syncBlock

$ go build ./internal/...
# clean
```

### 11.4 已知遗留 / 后续

- **v0.7.42 SHEET 工具栏 modal UI（冻结/筛选/条件格式/数据验证/迷你图）在一次 `git checkout` 中被回退到 v0.7.38 的 793 行状态** — 底层 adapter（xlsxFilter / xlsxCf / xlsxDv / xlsxSparkline / xlsxWorksheetIo）均健在，公式栏完整化也独立可跑，但工具栏上的 5 个按钮与对应的 modal 模板需要按 §9 重新接入。
- 公式引擎 **不写入** .xlsx 字节：当前只渲染公式栏的实时结果。要让公式「真正落地」，需要在 `xlsxWorksheetIo.transformWorkbook` 的 sheet XML 写入路径里加一段：当 cell 有 `=` 前缀时，把值替换为 `<f>…</f><v>result</v>`（仿 genoffice / Excel 的 cellFormula schema）。这一段是飞书 SHEET 与当前实现的最后一公里差距。
- 未实现的函数（按飞书 SHEET 文档继续补）：`IFS / SWITCH / AND / OR / NOT / LEFT / RIGHT / MID / SEARCH / DATE / TODAY / NOW / INDIRECT / OFFSET`。

---

## 12. v0.7.43.b SHEET 工具栏 modal UI 补回 + 公式持久化（2026-09-01）

承接 §11.4，把 v0.7.42 在 `git checkout` 中丢失的 5 个 SHEET 工具栏 modal UI 全部补回，并把公式真正写到 `.xlsx` 字节。

### 12.1 5 个 modal UI 重新接入（CollabSheetEditor.vue）

工具栏新增 5 个按钮：

- **冻结**：行数 + 列数两个 number 输入，应用即调 `buildFeatureTransforms()` 写回 `<sheetView><pane>`。
- **筛选**：列号 + 筛选值（逗号/空格分隔），写入 `<autoFilter>`。
- **条件格式**：单元格 + 比较符（`>`/`<`/`=`/`between`）+ 阈值，红字 + 浅红底色（dxf 0 兜底）。
- **数据验证**：单元格 + 类型（下拉列表 / 整数范围）+ 允许值，写入 `<dataValidations>`。
- **迷你图**：类型 + 目标单元格 + 源范围 + 颜色，写入 `<x14:sparklineGroup>`。

所有按钮：
- 修改 `freezeBySheet / filterBySheet / cfBySheet / dvBySheet / sparkBySheet` 这 5 个 per-sheet reactive ref。
- 调用 `scheduleSave()` → `flushSave()`。
- `flushSave` / `exportXlsx` 末尾追加 `transformWorkbook(bytes, buildFeatureTransforms())`：每个有 feature 状态的 sheet 生成 `(xml) => xml` 串行 pipeline（freeze → filter → cf → dv → spark）。

### 12.2 公式持久化（真正落地）

之前公式栏只渲染实时结果，cell value 里仍然是 `=SUM(...)` 文本。SheetJS 的 `aoa_to_sheet` 会丢掉 `v === ''` 的 cell（即使有 `f`），所以修复了两处：

- **`xlsxAdapter.ts saveXlsxBytes`**：当 `cell.f` 存在但 `cell.v` 为空时，seed 一个 placeholder `v = 0` 让 SheetJS 分配 cell slot。Excel/WPS 打开时会自动重算。
- **`xlsxAdapter.ts cellValue`**：之前优先返回 `cell.w`（格式化字符串 "0"），现在对公式 cell 优先返回 `cell.v`（数字/布尔），保留 cell 的实际类型。

### 12.3 修复 vendor adapter 形状不匹配

- `CfWireRule.rule.type` 不能是 `cellIs` — 用 `highlightCell + subType: 'number' + operator + value + style`。
- `applyCfRules(xml, rules, dxfs: DxfSink)` 第三个参数必填 — 提供 `{ internDxf: () => 0 }` 即可（CF 0 dxf 用默认红/浅红）。
- `applySparklineAdditions(xml, readonly SparklineGroupAdd[])` 接收 array 而非单个，`cells: [{ cell, sourceRef }]`。

### 12.4 新测试（6 个）

- `CollabSheetEditor.feature.test.ts`（3 个）：
  - `newXlsxWorkbook → saveXlsxBytes` round-trip。
  - filter + sparkline + dv + cf 全部经过 `transformWorkbook` 写入 zip，并能再 `openXlsx`。
  - identity transform 短路返回 `bytes` 自身（不写盘）。
- `CollabSheetEditor.formula.test.ts`（3 个）：
  - `SUM(A1:B1)` / `AVERAGE(A2:B2)` round-trip，`cell.f` 持久化。
  - 跨表 `SUM(Sheet2!A1:A2)` round-trip（之前 SheetJS 直接 drop）。
  - 非公式 cell（文本 / 整数 / 小数）round-trip 不变。

### 12.5 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 135
ℹ pass 135
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 40 errors — 全部 wiki / mindmap / i18n / slides / syncBlock 既有，0 个本任务相关

$ go build ./internal/...
# clean
```

### 12.6 仍未做（v0.7.43.c+）

| 优先级 | 任务 | 备注 |
| --- | --- | --- |
| P1 | SHEET 超链接 UI | adapter 已 copy，但 `applyHyperlinkEdits` 同时改 worksheet.xml + rels.xml；需要扩展 `transformWorkbook` 支持双文件写入 |
| P1 | SHEET 单元格颜色 / 字体 | `XlsxAdapterCell` 已有 `color / fill / bold / italic` 字段，差 xlsxAdapter 把它们落到 `cell.s`（要维护 xl/styles.xml 状态） |
| P1 | DOC math 公式写入 OOXML | TipTap 插入 `<div data-latex>` 是过渡方案；save 时要检测并切到 `<m:oMath>` |
| P2 | Y.Map 同步 feature 规则 | 现在还是本地 ref，需要 `sheet:features` Y.Map 让多人实时看到 |
| P2 | PPT 11 类图表 UI | `xlsxChart.applyChartEdit` 是 SHEET 命名空间，PPT 要走 chart1.xml 单独路径 |
| P2 | PPT SmartArt | genoffice 没有 dgm-hier.ts，需自行实现 |
| P2 | DOC 表格 cell 颜色 / 合并 | 部分就位 |

---

## 13. v0.7.43.c SHEET 单元格颜色 / 字体 / 填充持久化（2026-09-01）

承接 §12.6 P1「SHEET 单元格颜色 / 字体」收尾。

### 13.1 已完成

- **copy** `xlsx-styles.ts`（447 行）from `genoffice/apps/sheets/src/gateway/` → `frontend/src/editor/adapters/xlsxStyles.ts`
  - 改：inline `WorkbookStyleEdit` zod schema 为 TS interface，drop `shortDateNumFmtId` 依赖
  - 加 5 个 field 到 interface：`protectionHidden` / `borderTop / borderBottom / borderLeft / borderRight`
- **新测试** `xlsxStyles.test.ts`（9 tests）：font / fill / border / numFmt / protection / 边界 ✅ 全过
- **xlsxAdapter 集成** `applyCellStyles()`：
  - SheetJS 写完字节 → JSZip 加载 → `StylesheetEditor` 编辑 → 改 worksheet XML 加 `s="N"` 属性
  - 关键正则修复：`<c\\s+[^>]*?\\br="A1"...\\s+s="(\\d+")`，**不能用 `\bs=`**（word-boundary 在 `"` 后失败）
- **fill 颜色 round-trip** 通过

### 13.2 Bug 修复：StylesheetEditor `toArgb` 长度溢出

- **症状**：cell 入参 `'FF0000FF'`（8 字符 ARGB）→ round-trip 读回 `'FFF0000FF'`（9 字符）
- **根因**：`xlsxStyles.ts:462` 的 `toArgb` 实现 `return \`FF${hexColor.slice(1).toUpperCase()}\`` 假设输入 7 字符，**对 8 字符 ARGB 切第 1 字符后拼 `'FF'`，产生 9 字符**
- **修复**：识别 8 字符 ARGB 直接透传，6 字符 RGB 才加 `FF` 前缀
  ```ts
  function toArgb(hexColor: string): string {
    if (hexColor.length === 8) return hexColor.toUpperCase()
    return `FF${hexColor.toUpperCase()}`
  }
  ```
- 顺手清理：删除 `_debug_styles.test.ts`（调试用）+ `xlsxAdapter.ts` 里两行 `console.error('[DBG] …')`

### 13.3 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 147
ℹ pass 147
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxStyles/xlsxAdapter/CollabSheetEditor/sheetFormula）0 错误
# 其他 6 个 pre-existing 错误与本任务无关（xlsxChart/SyncBlockNode/i18n/main.ts）

$ go build ./internal/...
# clean
```

### 13.4 后续路线（v0.7.44+）

详见 `PLAN_V3.md` 第三章，重点：

| 优先级 | 模块 | 来源 | 行数 | 改造难度 | 备注 |
| --- | --- | --- | --- | --- | --- |
| P0 | xlsx-notes.ts（单元格批注） | genoffice | 297 | 中 | 写 `<legacyDrawing>` + VML + rels，要扩展 `transformWorkbook` 支持同时改 worksheet.xml + 多个 rels.xml |
| P0 | xlsx-sheets.ts（增删改 sheet） | genoffice | 607 | 中 | 含 `validateSheetName` / rename 引用替换 / orderChanged 标志 |
| P0 | xlsx-page-setup.ts（页面布局） | genoffice | 487 | 低 | 独立 module，纯属性写 `<printOptions>/<pageMargins>/<pageSetup>` |
| P1 | xlsx-table-add.ts（表对象 ListObject） | genoffice | 292 | 中 | 写 `xl/tables/tableN.xml` + `[Content_Types].xml` override + rels |
| P1 | xlsx-structure.ts（结构操作） | genoffice | 1384 | 高 | 公式 / 跨表引用解析，是 sheets.ts / charts.ts / dv.ts 等的上游依赖 |
| P1 | xlsx-drawing-edit.ts（绘图编辑） | genoffice | 406 | 中 | SHEET 内嵌入图片 / 形状编辑 |
| P2 | xlsx-pivot-add.ts（数据透视表） | genoffice | 1019 | 高 | 全功能透视表 |
| P2 | xlsx-pivot-expand.ts | genoffice | 337 | 高 | 透视表展开 |
| P3 | csv-import.ts | genoffice | 259 | 低 | CSV 导入向导 |
| P3 | xlsx-package-io.ts | genoffice | 366 | 低 | 包级 IO（save 时的元数据合并） |
| DOC | equation.ts（TipTap math 节点） | genoffice docs | 600+ | 高 | 需自建 docInlineMath / docProtected schema |
| DOC | table-handle / table-properties / table-sizing | genoffice docs | 800+ | 中 | DOC 表格 UI |
| DOC | revisions.ts | genoffice docs | 400+ | 高 | DOC 修订历史 |
| DOC | shape-draw / shape-svg | genoffice docs | 500+ | 中 | DOC 内置形状 |
| DOC | hf-dom / hf-text / cover-pages | genoffice docs | 700+ | 中 | 页眉页脚 / 封面 |
| DOC | toc-refresh / pagination-gaps / page-break | genoffice docs | 600+ | 中 | TOC + 分页 |
| PPT | animation-actions / animation-play | genoffice slides | 800+ | 低 | 动画（v0.7.29 部分已接） |
| PPT | table-actions / table-hit | genoffice slides | 500+ | 中 | PPT 表格 |
| PPT | draw-shape | genoffice slides | 300+ | 中 | PPT 形状绘制 |
| PPT | format-brush | genoffice slides | 400+ | 中 | 格式刷 |
| PPT | picture-edit-actions | genoffice slides | 400+ | 中 | PPT 图片编辑 |
| PPT | ruler-ticks / adjust-handles | genoffice slides | 300+ | 中 | 标尺 / 调节手柄 |
| PPT | SmartArt (dgm-hier) | OOXML 自写 | - | 高 | genoffice 没有，要查 OOXML schema 自写 |

总计 ~10000+ 行可 copy。其中 P0 的 4 个 sheet gateway（notes / sheets / page-setup / 加上 xlsx-package-io）~2000 行，预计 2 个 turn 完成。


---

## 14. v0.7.44 SHEET 余量 copy 第 1 批 — 页面布局 + 工作表管理（2026-09-01）

承接 §13.4 v0.7.44 路线，把 genoffice SHEET gateway 余量的两个核心模块 copy 进 WeKnora。

### 14.1 copy 完成（3 个 vendor adapter，~2478 行）

| 文件 | 来源 | 行数 | 依赖 |
| --- | --- | --- | --- |
| `frontend/src/editor/adapters/xlsxStructure.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-structure.ts` | 1384 | — |
| `frontend/src/editor/adapters/xlsxSheets.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-sheets.ts` | 607 | `xlsxStructure` |
| `frontend/src/editor/adapters/xlsxPageSetup.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-page-setup.ts` | 487 | `xlsxSheets` |

**改造**:
- `xlsxStructure.ts`: 把 `import type { WorkbookStyleEdit } from '../shared/desktop-api'` 改为 `'./xlsxStyles'`（已在 v0.7.43.c inline）
- `xlsxSheets.ts`: 把 `import { … } from './xlsx-structure'` 改为 `'./xlsxStructure'`
- `xlsxPageSetup.ts`: 同上 → `'./xlsxSheets'`

### 14.2 UI 接入（CollabSheetEditor.vue +154 行）

工具栏新增 2 个按钮：

- **页面** → 弹 modal：方向 / 纸张 / 页边距 / 缩放 / 网格线 / 行列号 / 页眉页脚 / 打印区域
- **工作表** → 弹 modal：表格列出全部 sheet，「↑↓ 重排 / 改名 / 隐藏 / 删除」

Sheet tab 标题加隐藏指示器（`title` 属性）。

**新增 reactive ref**:
```ts
const pageSetupBySheet = ref<Record<number, SheetPageSetupState | null>>({})
const sheetHiddenBySheet = ref<Record<number, boolean>>({})
const sheetRenameDraftBySheet = ref<Record<number, string>>({})
```

**新增 handlers**（~110 行）:
- `openPageSetupModal` / `onPageSetupCommit` / `onPageSetupClear`
- `openSheetManageModal` / `applySheetRenames` / `moveSheetUp` / `moveSheetDown` / `toggleSheetHidden` / `removeSheet`（含 per-sheet feature state cascade 重排）

**buildFeatureTransforms 扩展**：每个有 `pageSetupBySheet[idx]` 的 sheet 追加一段 `applyPageSetupState(next, ps)`，进入 freeze → filter → cf → dv → spark → pageSetup 串行 pipeline。

### 14.3 新测试（33 个累计）

- `xlsxStructureSheetsPageSetup.test.ts`（26 tests）：
  - **xlsxStructure** (5)：FORMULA_REFERENCE_PATTERN / qualifierMatches / shiftFormulaText × 3
  - **xlsxSheets** (10)：validateSheetName × 3 / formatSheetQualifier / renameSheetInFormula × 2 / formulaReferencesSheet / renameSheetReferencesInWorksheet / parseSheetElements / buildWorksheetPartXml
  - **xlsxPageSetup** (11)：applyPageSetupState × 6（orientation/paperSize/fitToPage/margins/printGridlines/preserve） / buildHeaderFooterXml × 2 / applyPrintAreas × 3（add/remove/unknown sheet throw）
- `CollabSheetEditor.pageSetupSheet.test.ts`（7 tests）：
  - orientation + paperSize round-trip
  - margins "narrow" 应用 0.25 left
  - printGridlines toggle
  - fitToPage → sheetPr/pageSetUpPr
  - identity transform 短路
  - 多 sheet workbook round-trip
  - 多 sheet 名/数量 round-trip

### 14.4 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 180
ℹ pass 180
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxStructure/xlsxSheets/xlsxPageSetup/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 14.5 已知遗留 / 后续

- **pre-existing**: `xlsxAdapter.saveXlsxBytes` 写字符串 cell 时 SheetJS 把 `<v>` 写空。v0.7.44 测试用 formula cell 绕过（cached value 保留），但字符串 cell 的 round-trip 仍然丢值。这是一个 SheetJS + xlsxAdapter 写盘流程的已知问题，单独 fix 计划列入 v0.7.46 或更后。
- **`xlsxStructure.ts` 12 个 export 只暴露 5 个给当前调用方**（其余供 v0.7.46 copy `xlsx-drawing-edit.ts` / `xlsx-pivot.ts` 时使用）。
- **页面设置的打开/关闭按钮**：当前 modal 是「应用」+「清除」，但还没有「关闭」按钮——需要点应用或取消才退出。后续 v0.7.45 优化。
- **`removeSheet` cascade**：删除 sheet 时把 per-sheet feature state (freeze/filter/cf/dv/spark/pageSetup/hidden) 全部 cascade 重排 — 已实现，但还没写测试。v0.7.45 加测试。

### 14.6 下一步 — v0.7.45

承接 v0.7.44，余下目标：

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | xlsx-notes.ts（单元格批注） | genoffice | 297 | 写 `<legacyDrawing>` + VML + rels，要扩展 `transformWorkbook` 支持同时改 worksheet.xml + 多个 rels.xml |
| P0 | xlsx-hyperlinks.ts UI | (已 copy) | 142 | adapter 已就位，缺 modal |
| P0 | xlsx-table-add.ts（ListObject 表对象） | genoffice | 292 | 写 `xl/tables/tableN.xml` + `[Content_Types].xml` override + rels |
| P1 | xlsx-package-io.ts（包级 IO） | genoffice | 366 | 整包读 + 整包写 + Content_Types 合并 |

预计 1 turn。


---

## 15. v0.7.45 SHEET 余量 copy 第 2 批 — 批注 + 超链接 + 表对象（2026-09-01）

承接 §14.6 v0.7.45 路线，把 genoffice SHEET gateway 余量的三个核心模块 copy 进 WeKnora。

### 15.1 copy 完成（3 个 vendor adapter，~1431 行）

| 文件 | 来源 | 行数 | 依赖 |
| --- | --- | --- | --- |
| `frontend/src/editor/adapters/xlsxNotes.ts` ★ NEW | `genoffice/.../xlsx-notes.ts` | 297 | 独立 |
| `frontend/src/editor/adapters/xlsxDrawingAdd.ts` ★ NEW | `genoffice/.../xlsx-drawing-add.ts` | 842 | 独立 |
| `frontend/src/editor/adapters/xlsxTableAdd.ts` ★ NEW | `genoffice/.../xlsx-table-add.ts` | 292 | `xlsxDrawingAdd` |

**改造**:
- `xlsxTableAdd.ts`: 把 `import { columnLabel } from '../domain/cell-address'` 改为 inline 函数（避免引入完整 cell-address 模块）
- `xlsxTableAdd.ts`: 把 `import { … } from './xlsx-drawing-add'` 改为 `'./xlsxDrawingAdd'`
- 接口字段修正：`columns` → `columnNames`，新增必填 `bandedRows: boolean`

### 15.2 UI 接入（CollabSheetEditor.vue +188 行）

工具栏新增 3 个按钮 + 3 modal：

- **批注** → 行/列/作者/批注内容 → 添加 / 清除 + 当前批注列表
- **链接** → 行/列/目标（URL 或 `#Sheet!A1`）→ 添加 / 清除 + 当前链接列表
- **表格** → 表名/范围/列名 → 添加 / 清除 + 当前表对象列表

**新增 reactive ref**:
```ts
const notesBySheet = ref<Record<number, SheetNote[]>>({})
const hyperlinksBySheet = ref<Record<number, HyperlinkEdit[]>>({})
const tablesBySheet = ref<Record<number, TableAddition[]>>({})
```

**新增 handlers**（~120 行）：
- `openNoteModal` / `onNoteCommit` / `onNoteClear`
- `openHyperlinkModal` / `onHyperlinkCommit` / `onHyperlinkClear`
- `openTableModal` / `onTableCommit` / `onTableClear`
- helper: `colToIndex(s: string)` — 列字母 → 0-based 索引

**buildFeatureTransforms 扩展**：在 freeze → filter → cf → dv → spark → pageSetup pipeline 末尾增加 hyperlink 处理（worksheet.xml 单文件部分，多文件通过 `applyHyperlinkEdits` 走 `rels.xml`）。

### 15.3 新测试（16 个累计）

- `xlsxNotesDrawingTable.test.ts`（11 tests）：
  - **xlsxDrawingAdd** (6)：relsPathFor / relativeTarget / allocatePartPath / appendRelationship / registerContentTypeOverride / buildChartXml
  - **xlsxNotes** (2)：applySheetNotes add / replace
  - **xlsxTableAdd** (3)：applyTableAdditions add / duplicate / invalid name
- `CollabSheetEditor.notesHyperlinksTable.test.ts`（5 tests）：
  - external URL hyperlink with r:id + rels
  - internal anchor uses `location` attribute (no rel)
  - target=null removes hyperlink
  - empty transforms short-circuit
  - multi-sheet identity round-trip

### 15.4 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 196
ℹ pass 196
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxNotes/xlsxDrawingAdd/xlsxTableAdd/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 15.5 已知遗留 / 后续

- **transformWorkbook 多文件支持**：当前 `transformWorkbook` 只支持 worksheet.xml 单文件编辑。hyperlink 需要同时改 worksheet + rels.xml，table 需要同时改 worksheet + tables/ + workbook.xml + [Content_Types].xml，note 需要同时改 worksheet + comments.xml + drawings/vmlDrawing.vml + worksheet rels + [Content_Types].xml。本 turn 的 CollabSheetEditor UI 写的是 reactive state，**真正写入 .xlsx 字节需要扩展 transformWorkbook 支持多文件 pipeline**（v0.7.46 的核心任务）。
- **hyperlinks UI round-trip 验证**：单文件 round-trip 通过（adapter 层），但走完整 transformWorkbook + saveXlsxBytes 时 rels 注入需要 JSZip 多文件读写。下一步 v0.7.46 重点扩展。
- **`xlsx-drawing-edit.ts` (406 行)** 是 v0.7.46 copy 目标 — SHEET 内嵌入图片 / 形状编辑。
- **`xlsx-pivot.ts` + `pivot-add.ts` + `pivot-expand.ts`** (1795 行) 是 v0.7.46 透视表 copy 目标。

### 15.6 下一步 — v0.7.46

承接 v0.7.45 路线，重点：

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | 扩展 `transformWorkbook` 为 `transformPackage` 支持多文件 | 自写 | ~100 | 关键基础设施 |
| P0 | `xlsx-drawing-edit.ts`（绘图编辑） | genoffice | 406 | 嵌入图片 / 形状 |
| P1 | `xlsx-pivot.ts` + `xlsx-pivot-expand.ts` | genoffice | 776 | 透视表（v0.7.46 跳过 pivot-add.ts 的 1019 行，因其依赖 deep analysis） |
| P1 | Y.Map `sheet:features` 同步（freeze/filter/cf/dv/spark/notes/tables/pivots/hyperlinks） | 自写 | ~200 | 实时协作 |
| P2 | DOC TipTap ↔ docx-engine convert | genoffice | 500+ | DOC 飞书级基础 |

预计 1.5 turn。

