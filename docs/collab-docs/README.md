# WeKnora 飞书文档协作（v0.7.26）

> 类飞书 / 腾讯文档的在线协作编辑能力：DOC / SHEET / SLIDE 三种类型，每类都有
> 实时协作（Yjs y-websocket）+ 真实 .docx / .pptx / .xlsx 字节双向闭环 + KB
> 同步 + AI 段落润色 + 公开分享链接。

## 一、目标与定位

| 项 | 说明 |
| --- | --- |
| **目标** | 把 `/Users/louloulin/appx/genoffice` 的能力移植进 WeKnora，提供类飞书 / 腾讯文档的 DOC/SHEET/SLIDE 三类多人实时协作 |
| **当前代号** | v0.7.26 `collaborative_docs` |
| **核心 CRDT** | Yjs（同 wiki 实时分支共用 y-websocket wire protocol） |
| **编辑器栈** | DOC → TipTap + docx-engine；SHEET → SheetJS；SLIDE → pptxgenjs |
| **底层引擎** | `frontend/src/editor/engines/{docx-engine,pptx-engine,pptx-render,file-parse}/`（从 genoffice 移植） |
| **浏览器适配** | `frontend/src/editor/adapters/{docxAdapter,pptxAdapter,xlsxAdapter}.ts` |

## 二、架构总览

```
┌────────────────┐  POST/GET  ┌──────────────────────┐
│ Vue 3 前端      │ ─────────▶ │ Go REST + WS handler │
│                │             │ /collaborative-docs/* │
│                │             └──────────┬───────────┘
│                │ multipart upload      │ collab_doc_files (bytea)
│                │ Yjs WebSocket         │ collab_doc_snapshots (Yjs state)
│                │                       │
│                │ docx/pptx/xlsx bytes  ▼
│                │             ┌──────────────────────┐
│                │ ──────────▶ │ docparser /chunk     │
└────────────────┘             │ (Python anydoc)      │
                               └──────────┬───────────┘
                                          ▼
                               ┌──────────────────────┐
                               │ WeKnora KB ingestion │
                               └──────────────────────┘
```

## 三、后端实现（Go）

### 数据表（`migrations/sqlite/000040_collab_doc_files.up.sql` + `migrations/mysql/...`）

```sql
CREATE TABLE collab_doc_files (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    format          VARCHAR(16)  NOT NULL,
    content         BLOB         NOT NULL,
    size_bytes      INTEGER      NOT NULL,
    sha256          VARCHAR(64)  NOT NULL DEFAULT '',
    version         INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (doc_id, version)
);
```

每保存一次 .docx/.pptx/.xlsx 写一行；版本号单调递增；`docparser /chunk` 通过 `multipart/form-data` 重新摄入。

### REST 端点（v0.7.26 新增）

```
POST   /api/v1/collaborative-docs                    # 创建
GET    /api/v1/collaborative-docs                    # 列表
GET    /api/v1/collaborative-docs/:id                # 元数据
PATCH  /api/v1/collaborative-docs/:id                # 重命名 / 可见性 / share_token
POST   /api/v1/collaborative-docs/:id/archive        # 软删除
DELETE /api/v1/collaborative-docs/:id                # 硬删除
GET    /api/v1/collaborative-docs/:id/presence       # 在线协作列表
GET    /api/v1/collaborative-docs/:id/export         # 静态 markdown 占位
POST   /api/v1/collaborative-docs/:id/sync-to-kb     # ← v0.7.26 KB 同步
POST   /api/v1/collaborative-docs/:id/upload         # ← v0.7.26 multipart 字节上传
GET    /api/v1/collaborative-docs/:id/download       # ← v0.7.26 最新字节下载
GET    /api/v1/collaborative-docs/:id/download/:v    # ← v0.7.26 历史版本下载
GET    /api/v1/collaborative-docs/share/:token/dl    # ← v0.7.26 公开只读下载
GET    /api/v1/collaborative-docs/:id/realtime       # Yjs WS 升级（y-websocket）
```

### 主要文件

```
internal/types/collaborative_doc.go              # 类型 + 三种 DocKind + CollabDocFile
internal/types/collaborative_doc_test.go        # 单元测试
internal/types/interfaces/collab_doc.go          # 三个仓库接口（+ FindByShareToken）
internal/application/repository/collab_doc.go    # Postgres/SQLite 双 repo 实现
internal/application/service/collaborative_doc.go          # 业务层
internal/application/service/collaborative_doc_authz.go    # ACL
internal/handler/collaborative_doc.go            # 元数据 REST（7 端点）
internal/handler/collaborative_doc_bytes.go      # ← v0.7.26 字节 + sync-to-kb + share
internal/handler/collaborative_doc_ws.go         # Yjs y-websocket 升级
internal/handler/collaborative_doc_sync.go       # KB 同步（multipart）
internal/router/routes_collaborative_doc.go      # 路由注册
internal/container/container.go                  # DI 注入
```

## 四、前端实现（Vue 3）

### 关键组件

| 文件 | 功能 |
| --- | --- |
| `src/api/collabDoc/index.ts` | REST + WS URL helper |
| `src/composables/useYjsCollabDoc.ts` | Yjs y-websocket composable |
| `src/components/collab/CollabDocProEditor.vue` | **v0.7.26** TipTap + docx-engine 双向闭环 + AI 润色入口 |
| `src/components/collab/CollabSheetEditor.vue` | **v0.7.26** Y.Map 网格 + SheetJS 上传/下载 |
| `src/components/collab/CollabSlideEditor.vue` | **v0.7.26** Y.Array<slide> + pptxgenjs 上传/下载 |
| `src/components/collab/CollabAiPolishDialog.vue` | **v0.7.26** 段落级 AI 润色弹窗（SSE 流式） |
| `src/editor/adapters/docxAdapter.ts` | docx-engine 浏览器侧 wrapper |
| `src/editor/adapters/pptxAdapter.ts` | **v0.7.26** pptxgenjs 包装器 |
| `src/editor/adapters/xlsxAdapter.ts` | **v0.7.26** SheetJS 包装器 |
| `src/views/collab/CollabDocListView.vue` | 飞书式列表（创建/筛选/删除/分享） |
| `src/views/collab/CollabDocEditorView.vue` | 按 doc_kind 装载三种编辑器 |
| `src/views/collab/CollabDocShareView.vue` | **v0.7.26** 公开只读分享视图 |
| `src/router/index.ts` | `/collab-documents` + `/collab-documents/:id` + `/collab-documents/share/:token` |

### 实时协作 CRDT 模型

- **DOC**：TipTap `Collaboration` 扩展 + `field: 'docx-body'`（每段是 Y.Text 子文档）
- **SHEET**：`Y.Map<rowKey, Y.Map<colKey, Y.Text>>`（每格一个 Y.Text）
- **SLIDE**：`Y.Array<Y.Map<{ title, bullets }>>`（每张幻灯片一个 Y.Map）

所有客户端共用一个 WebSocket fan-out，由 `useYjsCollabDoc` 自动管理 awareness / presence。

### AI 段落润色（v0.7.26）

```
[CollabDocProEditor.vue]
  ↓ 选中段落 → 点击"问 AI"
[CollabAiPolishDialog.vue]
  ↓ POST /api/v1/chat/agent-chat/stream + "请润色以下段落..." prompt
[后端 chat handler]
  ↓ 流式 SSE
[CollabAiPolishDialog.vue 显示原文 vs AI 建议]
  ↓ 用户"接受并替换"
[CollabDocProEditor.vue]
  ↓ patchParagraphText(doc, docxIndex, replacement)
[docxAdapter.saveDocxBytes]
  ↓ debounced 1.5s
[POST /api/v1/collaborative-docs/:id/upload]
```

## 五、本地运行与烟雾测试

### 后端编译

```bash
cd /Users/louloulin/appx/WeKnora
go build ./cmd/server/
go test ./internal/types/...
```

### 前端类型检查 + 测试

```bash
cd /Users/louloulin/appx/WeKnora/frontend
npm install
./node_modules/.bin/vue-tsc --build                  # 0 errors
./node_modules/.bin/tsx --test src/editor/adapters/__tests__/adapters.test.ts
# 3 tests pass: docx/pptx/xlsx round-trip
```

### 烟雾测试（需要 WeKnora server + docparser）

```bash
WEKNORA_TOKEN=<jwt> WEKNORA_KB_ID=<existing-kb> \
  /Users/louloulin/appx/WeKnora/scripts/smoke-collab-docs.sh
```

脚本会：
1. 创建一个 `doc_kind=doc` 的协作文档
2. 内联生成最小 .docx 并通过 `/upload` 上传
3. 通过 `/download` 取回并比对字节大小
4. 把 visibility 设为 public 并通过 `/share/:token/download` 公开访问
5. 触发 `/sync-to-kb`（不论 docparser 是否可达都返回 202）

## 六、与 genoffice 的对应关系

| genoffice 资源 | WeKnora 中的角色 |
| --- | --- |
| `packages/docx-engine/src/patch.ts` (saveDocx, patchParagraphTexts) | 浏览器侧 `docxAdapter.saveDocxBytes` + `patchSingleParagraphXml`（自实现，因为 patchParagraphTexts 是为 footnote 设计的） |
| `packages/docx-engine/src/parse.ts` (parseDocx, ParsedDocFull) | `docxAdapter.openDocx` |
| `packages/docx-engine/src/blank.ts` (buildBlankDocx) | `docxAdapter.buildBlankDocxDoc`（首次保存无字节场景） |
| `packages/pptx-engine/src/index.ts` (openPptx, savePptx) | 暂未直接使用；浏览器侧走 pptxgenjs（PPT 场景对协同要求低，genoffice 引擎的 node:crypto 依赖需要 polyfill） |
| `apps/docs/src/renderer/editor/convert.ts` (pmDocToSavePlan) | v0.7.27 待移植（目前段落级协作够用，全文 PM ↔ SaveBlock 转换下个版本） |
| `apps/sheets/` (Univer 0.25.1 + Rust sidecar) | SheetJS（数据 round-trip，公式/图表 v0.7.27+ Univer） |

## 七、v0.7.27+ 待办

- [x] 移植 genoffice `pmDocToSavePlan`（PM doc ↔ SaveBlock 双向序列化），支持富文本格式全保真
- [ ] Univer 0.25.1 替换 SheetJS（公式 + 单元格格式 + 跨表引用）
- [x] Polyfill pptx-engine 的 `node:crypto` / `node:zlib`，让 genoffice 的 pptx-engine 真正可用
- [ ] vue-konva 接入，做 PPT 形状/主题/母版编辑
- [x] IndexedDB Yjs persistence（CRDT 离线缓冲，重连自动 merge）
- [ ] Per-shape Y.Map（PPT）和 per-paragraph Y.Text 拆分（DOC 大文档优化）

## 八、风险登记

| 风险 | 当前缓解 |
| --- | --- |
| docx-engine 自带 parse.ts 用 `node:fs/promises` | 浏览器侧仅使用纯 TS 函数（parseDocx 走 JSZip，不需要 fs） |
| pptx-engine 含 `node:crypto` / `node:zlib` | 浏览器侧走 pptxgenjs（避免触发 node-only 路径） |
| SheetJS 类型缺失 | 已 `npm install ./packages/xlsx-0.20.2.tgz` 装上 0.20.2 |
| AI 段落润色 prompt 注入 | 后端 chat handler 自带输入脱敏（沿用既有 chat 管线） |
| 大文档 OOM | 单 docx 上限 64 MiB（`collaborative_doc_bytes.go` 内 hard cap） |
