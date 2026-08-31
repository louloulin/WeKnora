# WeKnora 飞书文档协作能力 — 现状分析（2026-08-31）

> 本文档盘点 WeKnora 当前已实现的「飞书/腾讯文档」协作能力与 genoffice 目标能力之间的差距，
> 作为后续 PORT_PLAN_V2 的输入。已与仓库内 `frontend/`、`internal/`、`migrations/` 实际文件交叉验证。

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
