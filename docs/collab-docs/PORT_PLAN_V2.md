# WeKnora 飞书文档协作 — 移植计划 v2（genoffice 全面对接版）

> 本计划承接 `STATUS.md` 的现状盘点，把 `/Users/louloulin/appx/genoffice` 的能力按
> **引擎层 → 编辑器层 → CRDT 层 → 服务层** 自下而上移植进 WeKnora。每个 phase 都给出
> 具体修改文件、验收点、可执行命令与回滚策略。
>
> **版本约定**：v0.7.25 = 当前 MVP；v0.7.26 = Phase 0+1；v0.7.27 = Phase 2+3；v0.7.28 = Phase 4+5。

---

## Phase 概览

```
┌──────────────┬───────────────┬──────────────────────────────────────────────────────┐
│ Phase        │ 目标          │ 交付物                                                │
├──────────────┼───────────────┼──────────────────────────────────────────────────────┤
│ P0 修脚手架  │ 类型/构建绿    │ vue-tsc 干净；i18n 干净；auth store 接线正确            │
│ P1 DOC 真编辑│ .docx 双向闭环 │ 上传→编辑→下载，单测覆盖 5 种 block                    │
│ P2 PPT 真渲染│ Konva 渲染     │ pptx-engine polyfill；vue-konva 形状层               │
│ P3 SHEET 真编│ Univer 替换    │ Univer 0.25.1 + Yjs provider 接入                   │
│ P4 协作粒度  │ 段落/形状粒度  │ DOC per-paragraph Y.Text；SLIDE per-shape Y.Map      │
│ P5 KB/AI 接入│ 选段 ↔ AI     │ docparser /chunk 接 collab；chat 选段润色            │
│ P6 离线+分享│ Service Worker│ IndexedDB Yjs persistence；share-token 路由          │
└──────────────┴───────────────┴──────────────────────────────────────────────────────┘
```

每个 Phase 都假设前一个 Phase 已完成且 `go build ./...` 与 `vue-tsc --build` 均绿。

---

## Phase 0 — 修脚手架（半天）

**目的**：把当前残缺的类型错误一次清零，确保后续编辑可以基于绿的 vue-tsc。

### 0.1 npm 依赖补齐

```bash
cd frontend && npm install --save \
  yjs@^13.6.0 y-websocket@^2.0.4 \
  @tiptap/extension-collaboration@^2.10.3 \
  @tiptap/extension-collaboration-cursor@^2.10.3 \
  @univerjs/presets @univerjs/ui @univerjs/sheets @univerjs/yjs \
  vue-konva konva
npm install --save-dev @types/konva
```

修后 `package.json` 应包含上述包。**坑**：`@univerjs/presets` 是 monorepo 别名；如未发布确认版本，改用 `0.25.x` 显式版本。

### 0.2 docxAdapter 签名修复

**文件**：`frontend/src/editor/adapters/docxAdapter.ts:65-72`

把
```ts
return saveDocx(doc.parsed, { blocks, ...opts })
```
改成
```ts
const opts: SaveOptions = doc.parsed.removePersonalInfo ? { removePersonalInfo: true } : {}
return saveDocx(doc.parsed, blocks, opts)
```

**理由**：`saveDocx(parsed, finalBlocks, options)` 第二参是 `SaveBlock[]`（见 `packages/docx-engine/src/patch.ts:380`）。

### 0.3 i18n 重复 key 修复

**文件**：`frontend/src/i18n/locales/en-US.ts:2918, 2942`

`google_workspace: 'Google Workspace',` 后少了一个 `,`，导致相邻两个对象字面量被解释成同一对象的重复属性。在两个被报告的位置各加一个 `,`。

### 0.4 auth store 接线

**文件**：`frontend/src/views/collab/CollabDocEditorView.vue:57-58`

```ts
// 前
token.value = authStore.accessToken || ''
displayName.value = authStore.user?.name || authStore.user?.username || '匿名用户'
// 后
token.value = authStore.token || ''
displayName.value = authStore.user?.username || '匿名用户'
```

（auth store 实际导出 `token` + `user.username`，见 `frontend/src/stores/auth.ts` 与 `frontend/src/api/auth/index.ts:107` 的 `UserInfo`。）

### 0.5 类型合并（boolean/peers/error）

**文件**：`CollabDocEditor.vue:55-57`、`CollabSheetEditor.vue:93-95`、`CollabSlideEditor.vue:108-110`

watch 回调里用 `!!v` 强转 + 默认值，确保赋给 `ref<boolean>` / `ref<Peer[]>` / `ref<string | null>`。
对 `CollabSlideEditor.vue:122` 把 `.map(m => objToSlide(m.toJSON() as Record<string, unknown>))` 改为
```ts
.map((m: Y.Map<unknown>) => objToSlide(m.toJSON() as Record<string, unknown>))
```

### 0.6 teardown 时 editor 赋值

**文件**：`CollabDocEditor.vue:74` 把 `editor.value = null` 改为 `editor.value = undefined`（与 `shallowRef<Editor | undefined>` 一致）。

### 0.7 验收

```bash
cd frontend && ./node_modules/.bin/vue-tsc --build 2>&1 | grep -E "error TS" \
  | grep -iE "collab|docxAdapter|engines|file-parse"
# 期望：无
cd .. && go build ./...
# 期望：无报错
```

回滚：`git checkout -- frontend/src/editor frontend/src/components/collab frontend/src/views/collab frontend/src/i18n frontend/src/composables/useYjsCollabDoc.ts`

---

## Phase 1 — DOC 真编辑（核心，1-1.5 周）

**目的**：跑通「上传 .docx → 浏览器内 TipTap 编辑 → 导出 .docx」闭环，复用 genoffice 的 `parseDocx` / `saveDocx` / `patchParagraphTexts`。

### 1.1 数据模型（前端）

**文件**：`frontend/src/components/collab/CollabDocProEditor.vue`（新建）

**关键设计**：
- **双 Y.Doc**：docA 是 TipTap `Collaboration` 用的 fragment（`default` key），docB 是 byte-patch 用的 JSON 镜像（`docx:meta` key）。两者通过 `Y.Map.observe` 互相同步。
- **段落锚定**：每个 TipTap paragraph 节点带 `attrs.docxIndex`，对应 docx-engine `Block.docxIndex`。
- **PM doc → SaveBlock**：从 TipTap doc JSON 提取 paragraphs，调 `pmDocToSavePlan`（移植自 genoffice `apps/docs/src/renderer/editor/convert.ts`）。
- **脏标记**：debounced 1.5s 触发 `saveDocxBytes(parsed, blocks, opts)` → 上传。

### 1.2 后端：upload/download 字节端点

**新增**：`internal/handler/collaborative_doc_bytes.go`

```go
POST   /collaborative-docs/:id/upload    multipart/form-data; file=*.docx
GET    /collaborative-docs/:id/download  application/vnd.openxmlformats-officedocument.wordprocessingml.document
```

- 存到 `collab_doc_files` 表（新增）：`(doc_id, format, content bytea, size_bytes, sha256, version, created_at)`
- `version` 每次 upload 自增，配合 `ETag` 防丢更
- `GetDocState` 把 `content` 也写进 Yjs `ydoc_state`（**用 Y.encodeStateAsUpdateV2 + content hash 作为 snapshot key**）
- 不覆盖 `collab_doc_snapshots`（Yjs CRDT 状态仍独立）

**新增**：`migrations/sqlite/000040_collab_doc_files.up.sql`
```sql
CREATE TABLE collab_doc_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id INTEGER NOT NULL,
  doc_id TEXT NOT NULL,
  format TEXT NOT NULL CHECK(format IN ('docx','pptx','xlsx')),
  content BLOB NOT NULL,
  size_bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  version INTEGER NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(doc_id, version)
);
CREATE INDEX idx_collab_doc_files_doc ON collab_doc_files(doc_id, version DESC);
```

> Postgres 版同等 DDL（bigserial + bytea）。

### 1.3 端到端单测

**文件**：`frontend/src/editor/adapters/docxAdapter.test.ts`（新建，vitest）

```ts
import { openDocx, saveDocxBytes, patchParagraphText, docxToMarkdown } from './docxAdapter'
import { readFileSync } from 'node:fs'

it('round-trips a simple .docx', async () => {
  const bytes = new Uint8Array(readFileSync('fixtures/simple.docx'))
  const doc = await openDocx(bytes)
  const original = doc.parsed.blocks.length
  patchParagraphText(doc, 0, 'Hello from WeKnora!')
  const out = await saveDocxBytes(doc)
  expect(out.byteLength).toBeGreaterThan(0)
  // Re-open and confirm change persisted
  const doc2 = await openDocx(out)
  expect(doc2.paragraphs[0].text).toBe('Hello from WeKnora!')
  expect(doc2.parsed.blocks.length).toBe(original)
})
```

**fixture**：`frontend/src/editor/adapters/__fixtures__/simple.docx` —— 从 `genoffice/packages/docx-engine/tests/fixtures/` 复制最小 .docx。

### 1.4 验收

- `npm run test src/editor/adapters/docxAdapter.test.ts` 绿
- 浏览器手动：
  1. 进入 `/collab-documents` → 新建 doc
  2. 上传 .docx → 看到段落被渲染
  3. 编辑 → 自动 1.5s 后下载 .docx 字节
  4. 用 Word/Pages 打开 → 文本变更保留、表格/图片保留

回滚：删除 `CollabDocProEditor.vue` + `collab_doc_files` 表，路由回退到 `CollabDocEditor.vue`。

---

## Phase 2 — PPT 真渲染（1 周）

**目的**：用 Konva 把 pptx-engine 解析出的 `SlideDeck` 渲染成 vue-konva 组件，支持只读视图 + 选段文字编辑。

### 2.1 polyfill node-only 依赖

**文件**：`frontend/src/editor/engines/pptx-engine/polyfills.ts`（新建）

```ts
// Browser-safe replacements for pptx-engine's Node-only imports.
// pptx-engine/src/zip.ts uses createHash; we expose a global sha256 via SubtleCrypto.
if (typeof globalThis.crypto?.subtle === 'undefined') {
  throw new Error('SubtleCrypto required; ensure HTTPS or localhost.')
}
// pptx-engine/src/media-insert.ts uses deflateSync; provide CompressionStream.
export const deflateSync: (data: Uint8Array) => Uint8Array = async (data) => {
  const cs = new CompressionStream('deflate')
  const writer = cs.writable.getWriter()
  writer.write(data); writer.close()
  const chunks: Uint8Array[] = []
  const reader = cs.readable.getReader()
  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    chunks.push(value)
  }
  // concat chunks
  const out = new Uint8Array(chunks.reduce((s, c) => s + c.length, 0))
  let o = 0
  for (const c of chunks) { out.set(c, o); o += c.length }
  return out
}
// deflateSync is referenced sync in pptx-engine; we re-export as sync via
// precomputed dictionary + fall back to async wrapper in callers.
```

**修改**：`pptx-engine/src/zip.ts:10` 把 `createHash` 改为读取 `globalThis.crypto.subtle.digest('SHA-256', bytes)` 的 sync 包装（用 `node:crypto` 仅在 Electron 走）。

**修改**：`pptx-engine/src/media-insert.ts:14` 同理替换。

### 2.2 vue-konva adapter

**文件**：`frontend/src/editor/adapters/pptxKonvaAdapter.ts`（新建）

把 genoffice `apps/slides/src/renderer/konva-adapter.ts` 的 4 个核心函数移植：

| genoffice 函数 | 移植目标 |
| --- | --- |
| `boxPivotProps(node)` | Konva Group x/y/rotation 计算 |
| `fillToKonva(fill)` | Rect/Path fill props |
| `isEditableText(node)` | TextEditOverlay 触发判断 |
| `renderNodeToKonva(node)` | 递归渲染成 vue-konva 组件树 |

vue-konva 的组件声明式特性其实比 react-konva 更适合 Vue 模板；用 `<v-stage :config="..."><v-layer><v-rect /></v-layer></v-stage>`。

### 2.3 组件

**文件**：`frontend/src/components/collab/CollabSlideProEditor.vue`（新建）

- 左侧 100px 宽 slide 缩略图条（用 buildSlide 缩放渲染到 off-screen canvas）
- 中间 Konva 舞台（视口缩放、平移、键盘快捷键）
- 右侧属性面板（genoffice 的 `adjust-handles.ts` Vue 化）
- 顶部工具栏：插入文字框、形状、图片、表格

**先做只读 + 文字编辑**，复杂交互（母版/动画/主题）留到 P4。

### 2.4 验收

- 上传 .pptx → Konva 舞台正确渲染所有 slide
- 双击文字框进入编辑 → 修改后保存到 Yjs + 后端 .pptx 字节
- 与 Phase 1 DOC 同源的 byte-save 路径，但走 `savePptx`

---

## Phase 3 — SHEET 真编辑（1 周）

**目的**：用 Univer 替换当前 Y.Map 网格 MVP，获得完整电子表格能力（公式、格式、跨表引用）。

### 3.1 Univer 安装与初始化

**新增**：`frontend/src/components/collab/CollabSheetProEditor.vue`

```ts
import { createUniver, defaultTheme, LocaleType } from '@univerjs/presets'
import { UniverSheetsCorePreset } from '@univerjs/presets/preset-sheets'
import '@univerjs/presets/lib/styles/preset-sheets.css'
```

**Yjs provider**：用 Univer 内置的 `@univerjs/yjs` + `WebsocketProvider`，指向同一 `wsUrl` 但 namespace 不同：
```ts
const ydoc = new Y.Doc()
const provider = new WebsocketProvider(wsUrl, `collab-sheet-${docId}`, ydoc)
const univer = createUniver({
  theme: defaultTheme,
  locale: LocaleType.EN_US,
  presets: [{
    name: UniverSheetsCorePreset,
    payload: { /* ... */ },
  }],
})
```

### 3.2 xlsx 字节端点

复用 Phase 1 的 `collab_doc_files` 表 + `format='xlsx'`。
后端导出走 docparser `/render/xlsx`（如果存在）或新增 `/render/sheet` 端点（python `openpyxl`）。

### 3.3 验收

- 新建 sheet → 默认 26×100 网格
- 编辑单元格 → Yjs 同步 → Univer native 渲染
- 上传 .xlsx → 解析 → Univer 接管（公式只读提示）
- 下载 .xlsx → docparser 还原

---

## Phase 4 — 协作粒度细化（1 周）

**目的**：把 Yjs 二进制粗粒度拆成段落/形状/单元格粒度，提升大文档响应。

### 4.1 DOC per-paragraph Y.Text

genoffice 的做法：每个 paragraph 是一个独立 `Y.Text` 节点，挂到 `Y.XmlFragment` 上。TipTap `Collaboration` 的 `field` 配置支持这种 sub-document 模式。

**文件**：`frontend/src/components/collab/CollabDocProEditor.vue`
- 把 `Collaboration` 的 `field` 从默认 'default' 改为自定义 `docx-paragraph`
- paragraph 节点 schema 加 `collaboration: true`
- Yjs schema：
  ```ts
  ydoc.getXmlFragment('docx-paragraph')   // 段落结构
  ydoc.getMap('docx-meta')                // 每段元数据（block 类型、level、runs 格式）
  ```

### 4.2 SLIDE per-shape Y.Map

```ts
ydoc.getArray<Y.Map>('slide:shapes')  // 每 shape 一个 Y.Map: {x,y,w,h,fill,text,...}
```

Konva 节点 change → patch 对应 Y.Map 字段。

### 4.3 SHEET

Univer native，无需额外。

### 4.4 验收

- 1000 段落文档：单客户端编辑延迟 < 50ms
- 两客户端同时编辑不同段落：Yjs 自动合并，互不干扰

---

## Phase 5 — KB 同步 / AI 接入（1 周）

### 5.1 KB 同步

**文件**：`internal/handler/collaborative_doc_sync.go`（已存在，扩展）

`/sync-to-kb` 流程：
1. 读 `collab_doc_files` 最新 .docx/.pptx/.xlsx
2. 按 doc_kind 走不同 docparser 端点：
   - docx → `/chunk`（已有）
   - pptx → `/render/pptx`（新增，python-pptx 把 slide JSON 转 .pptx；或直接 forward 已上传的 .pptx）
   - xlsx → `/chunk/xlsx`（新增）
3. 复用 `internal/application/service/chunk.go` 的 chunk → KB 摄入管线

### 5.2 选段 ↔ AI

**文件**：`frontend/src/components/collab/CollabAiPopover.vue`（新建）

- TipTap 选区 → 捕获 `state.selection` 范围
- 调 `/chat/stream?context=docx-block&block_index=...&user_text=...`
- AI 回复 → 弹窗显示 diff → 用户确认 → `patchParagraphTexts` 应用

复用现有 `frontend/src/views/chat/` 的流式输出 + TDesign 组件。

### 5.3 验收

- 选一段 → "润色" → 30s 内收到 AI diff
- 应用后段落更新，且 docx 文件字节保存
- 「同步到知识库」按钮 → docparser chunk → KB 出现新 chunk

---

## Phase 6 — 离线 + 分享（3 天）

### 6.1 IndexedDB persistence

**文件**：`frontend/src/composables/useYjsIndexedDb.ts`（新建）

```ts
import { IndexeddbPersistence } from 'y-indexeddb'
const persistence = new IndexeddbPersistence(`collab-doc-${docId}`, ydoc)
```

WS 断线 → 本地仍可编辑 → 重连后自动 merge。

### 6.2 share-token 路由

后端已存 `share_token`，前端加 `/collab-documents/share/:token` 只读视图（复用 CollabDocEditorView 但挂 `:read-only` query）。

### 6.3 验收

- 关网编辑 → 重连 → 远端自动追上
- share 链接打开新窗口 → 只读 + 实时更新

---

## 风险登记 + 缓解

| 风险 | 概率 | 影响 | 缓解 |
| --- | --- | --- | --- |
| pptx-engine polyfill 不完整 | 中 | P2 推迟 | 优先只读渲染，写操作降级为服务器端 python-pptx |
| TipTap ↔ docx-engine 段落边界不一致 | 高 | P1 文档结构破坏 | `pmDocToSavePlan` 移植后用 genoffice 89 个 fixtures 回归 |
| vue-konva Transformer 缺失 | 中 | P2 拖拽失败 | 手写 Konva Group 拖拽逻辑（genoffice 已有 ~1000 行 adjust-handles.ts） |
| Univer 包大小（~3MB） | 中 | 首屏慢 | dynamic import + 路由级 code split |
| Yjs 大文档（>5MB）初次加载 | 中 | 慢 | IndexedDB 缓存 + 后台 snapshot 增量加载 |
| docx 文件 round-trip 丢失样式 | 高 | 用户信任 | 引入 genoffice 的 89 个 fixture + snapshot test |

---

## 文件改动清单（按目录汇总）

### 后端（Go）
```
internal/handler/collaborative_doc_bytes.go        # 新增
internal/handler/collaborative_doc.go              # 扩展 Mount
internal/handler/collaborative_doc_sync.go         # 扩展 doc_kind 路由
internal/router/routes_collaborative_doc.go        # 注册新端点
internal/types/interfaces/collab_doc.go            # 新增 CollabDocFileRepository
internal/application/repository/collab_doc.go          # 新增 CollabDocFileRepository 实现
internal/types/collaborative_doc.go                # 新增 CollabDocFile 类型
internal/application/service/collaborative_doc.go  # 新增 SaveFile/LoadFile 方法
internal/container/container.go                    # 注入新 repo
migrations/postgresql/000040_collab_doc_files.up.sql    # 新增
migrations/sqlite/000040_collab_doc_files.up.sql         # 新增
```

### 前端（Vue 3）
```
frontend/package.json                              # 新增依赖
frontend/src/editor/engines/docx-engine/...        # 已拷贝 25 文件，小修
frontend/src/editor/engines/pptx-engine/zip.ts     # polyfill createHash
frontend/src/editor/engines/pptx-engine/media-insert.ts  # polyfill deflate
frontend/src/editor/engines/pptx-engine/polyfills.ts # 新增
frontend/src/editor/engines/file-parse/...         # 已删 Node-only
frontend/src/editor/adapters/docxAdapter.ts       # 签名修复
frontend/src/editor/adapters/docxAdapter.test.ts  # 新增
frontend/src/editor/adapters/pptxKonvaAdapter.ts   # 新增（genoffice port）
frontend/src/editor/adapters/pmDocToSavePlan.ts    # 新增（genoffice port）
frontend/src/composables/useYjsCollabDoc.ts        # 已有，小修类型
frontend/src/composables/useYjsIndexedDb.ts        # 新增
frontend/src/components/collab/CollabDocEditor.vue          # 已有，待废弃
frontend/src/components/collab/CollabDocProEditor.vue       # 新增
frontend/src/components/collab/CollabSheetEditor.vue        # 已有，待废弃
frontend/src/components/collab/CollabSheetProEditor.vue     # 新增（Univer）
frontend/src/components/collab/CollabSlideEditor.vue        # 已有，待废弃
frontend/src/components/collab/CollabSlideProEditor.vue     # 新增（vue-konva）
frontend/src/components/collab/CollabAiPopover.vue          # 新增
frontend/src/views/collab/CollabDocEditorView.vue           # 升级装载 Pro
frontend/src/views/collab/CollabDocShareView.vue            # 新增（只读）
frontend/src/api/collabDoc/index.ts               # 已有，加 upload/download
frontend/src/router/index.ts                       # 加 share 路由
frontend/src/i18n/locales/en-US.ts                 # 修重复 key
frontend/src/i18n/locales/zh-CN.ts                 # 加 Pro 编辑器词条
```

---

## 验收总览（每 Phase 末）

| Phase | 必须通过的验收 |
| --- | --- |
| P0 | `vue-tsc --build` 干净；`go build ./...` 干净 |
| P1 | docxAdapter 单测全绿；浏览器手动闭环通过 |
| P2 | 上传 .pptx → Konva 渲染像素正确（vs LibreOffice 截图 diff < 1% 像素） |
| P3 | 上传 .xlsx → Univer 渲染 + 公式只读；下载 .xlsx 完整 |
| P4 | 1000 段落文档两客户端并发编辑，Yjs 合并无冲突 |
| P5 | 选段 ↔ AI 端到端 30s 内；KB 同步 chunk 出现 |
| P6 | 离线编辑 → 重连合并；share 链接打开新窗口只读 |

---

## 时间线（按 1 人月 18 工作日估）

| 周 | Phase | 关键交付 |
| --- | --- | --- |
| W1 | P0 + P1 (DOC) | DOC 闭环 + upload/download |
| W2 | P1 收尾 + P2 起步 | DOC 段落粒度；PPTX polyfill + 只读渲染 |
| W3 | P2 收尾 + P3 | PPTX 文字编辑；SHEET Univer |
| W4 | P4 + P5 + P6 | 协作粒度、AI、离线、share |

> **执行顺序可调整**：若 P1 段落粒度挑战大，可把 P4 推迟一周，把 P3 提前一周（Univer 自带协作粒度）。
