# WeKnora 飞书文档协作 — V2 全量端口计划（v0.7.30 → v0.9.0）

> 承接 `STATUS.md` / `ANALYSIS_V1.md` / `PORT_PLAN_V2.md`，把
> `/Users/louloulin/appx/genoffice` 的能力按 **可执行颗粒度** 拆解，并同步
> 最近的引擎更新。

---

## 一、当前能力快照（2026-09-01）

### 1.1 引擎层（vendor 自 genoffice，已对齐主线）

| 引擎 | 文件数 | 行数 | 与 genoffice 差异 |
| --- | --- | --- | --- |
| `frontend/src/editor/engines/docx-engine` | 25 | 20,054 | 仅 `parse.ts` 略有浏览器适配（已同步） |
| `frontend/src/editor/engines/pptx-engine` | 41 | 20,833 | `polyfills.ts` + `pako.d.ts` 浏览器适配；`zip.ts` / `media-insert.ts` / `sections.ts` 用 polyfills 替换 Node 内置 |
| `frontend/src/editor/engines/pptx-render` | 12 | 9,163 | 全部 6 个文件为 vendored，import 路径改为相对 |
| `frontend/src/editor/engines/file-parse` | 4 | 252 | 仅 wrapper |

### 1.2 前端编辑器层（vue-konva + TipTap + 自研 SHEET）

| 组件 | 行数 | CRDT | 引擎 | 状态 |
| --- | --- | --- | --- | --- |
| `CollabDocProEditor.vue` | ~700 | ProseMirror+Yjs | docx-engine | ✅ 11 扩展 + 图片 + 表格 + 评论 |
| `CollabSheetEditor.vue` | ~490 | Y.Array×Y.Map | xlsx via SheetJS | ✅ 多 sheet + 公式 + 颜色 + 数字格式 |
| `CollabSlideKonvaEditor.vue` | ~820 | per-shape Y.Map | pptx-engine | ✅ 11 形状 + 表格 + **备注 (v0.7.30)** + 评论 |
| `CollabCommentsPanel.vue` | ~330 | REST + 5s poll | n/a | ✅ DOC/PPT 共用 |

### 1.3 后端层

| 表 | 文件 | 状态 |
| --- | --- | --- |
| `collaborative_docs` | migration 000035 | ✅ |
| `collab_doc_snapshots` | migration 000036 | ✅ Yjs 状态 |
| `collab_doc_sessions` | migration 000037 | ✅ 在线协作者 |
| `collab_doc_files` | migration 000038 | ✅ 字节存储 + 自动 version |
| `collab_doc_comments` | migration 000041 | ✅ 评论（v0.7.29） |
| `collab_doc_audit_log` | **migration 000042 (v0.7.31)** | ❌ 待建 |

REST 端点清单：13 端点 + WS 升级（详见 STATUS.md）。

---

## 二、核心差距 vs Feishu / Tencent 文档

| 维度 | WeKnora | Feishu | Tencent | 备注 |
| --- | --- | --- | --- | --- |
| 实时多人 | ✅ Yjs | ✅ | ✅ | — |
| 评论 / 讨论 | ✅ DOC + PPT | ✅ | ✅ | SHEET 未接入 |
| 选区广播 | ⚠️ 部分 | ✅ | ✅ | 全部缺失 |
| 操作历史 / 审计 | ❌ | ✅ | ✅ | v0.7.31 目标 |
| 飞书 / 腾讯特有 UI | ❌ | ✅ 高级 | ✅ | — |
| PPT 动画 | ❌ | ✅ | ✅ | genoffice 有 |
| PPT 母版 / 主题 | ❌ | ✅ | ✅ | genoffice 有 |
| PPT 图表 / SmartArt | ❌ | ✅ | ✅ | genoffice 有 |
| DOC 公式 | ❌ | ✅ | ✅ | genoffice 有 |
| SHEET 图表 | ❌ | ✅ | ✅ | 需 Univer 或自建 |
| 限流 / Webhook | ❌ | ✅ | ✅ | — |

---

## 三、Phase 实施计划

### v0.7.30 — PPT 演讲者备注 + PPT 选区广播 + 后端审计日志（当前）

**目标**：完善 PPT 演讲者备注（已落代码 + 已修 TS 错误），新增 PPT 选区广播，启动审计日志表创建。

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.30.1 ✅ | `setSlideNotesOnDeck` 修 slideIndex / 类型错误 | `frontend/src/editor/adapters/pptxShapeAdapter.ts`、`frontend/src/editor/engines/pptx-engine/zip.ts` | vue-tsc 零错误 |
| 0.7.30.2 ✅ | 写 PPT 备注 round-trip 测试 | `frontend/src/editor/adapters/__tests__/pptxNotes.test.ts` | 2/2 pass |
| 0.7.30.3 | PPT 选区广播（awareness `selection: {slide, shapeId}`） | `CollabSlideKonvaEditor.vue` + `useYjsCollabDoc.ts` | 远端选区高亮 |
| 0.7.30.4 | 后端 `collab_doc_audit_log` 表 + repo + service + middleware（草案） | migration 000042 + `internal/types/...` + `application/service/...` | 写入日志 + 列表查询 |
| 0.7.30.5 | 写 `pptxSelection.test.ts` 验证 awareness 字段 | `frontend/src/composables/__tests__/...` | pass |

### v0.7.31 — DOC 选区广播 + SHEET 选区广播 + 评论到 SHEET（1 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.31.1 | TipTap `CollaborationCursor` selection 范围扩展 | `CollabDocProEditor.vue` + `useYjsCollabDoc.ts` | 远端选区高亮 |
| 0.7.31.2 | SHEET awareness selection 字段（sheet + range） | `CollabSheetEditor.vue` | 远端选区高亮 |
| 0.7.31.3 | SHEET 评论接入 | `CollabCommentsPanel.vue` + `CollabSheetEditor.vue` | 单元格评论可用 |
| 0.7.31.4 | 写 DOC selection 测试 + SHEET selection 测试 | — | pass |

### v0.7.32 — PPT 母版 / 主题 / 版式 / 动画（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.32.1 | 接入 `master-edit.ts` | `pptxShapeAdapter.ts` + `CollabSlideKonvaEditor.vue` | 编辑后下载 .pptx 在 PowerPoint 中识别 |
| 0.7.32.2 | 接入 `theme-apply.ts` + `builtin-layouts.ts` | 同上 | 主题切换 + 套用版式 |
| 0.7.32.3 | 接入 `animation.ts`（动画定义 + 播放） | 新组件 + adapter | 添加进入动画可播放 |
| 0.7.32.4 | PPT 写测试覆盖 | — | pass |

### v0.7.33 — PPT 图表 / SmartArt（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.33.1 | 接入 `pptx-render/build-chart.ts` | `CollabSlideKonvaEditor.vue` + 新 `pptxChartAdapter.ts` | 插入柱状/折线/饼图 |
| 0.7.33.2 | `chart-insert.ts` 数据绑定 | — | 编辑图表数据 |
| 0.7.33.3 | SmartArt（`smartart.ts` + `smartart-layout.ts` + `dgm-hier.ts`） | — | 插入组织结构图 |

### v0.7.34 — DOC 公式 / 图表 / 批注（3 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.34.1 | DOC OMML 数学公式 | `docx-engine/math.ts` (1087) + `docxAdapter.ts` | 写 `E=mc^2` → OOXML omml |
| 0.7.34.2 | DOC 图表（`docx-engine/chart.ts` 951 行） | `docxAdapter.ts` + 新 `docxChartAdapter.ts` | 在文档中插图 |
| 0.7.34.3 | DOC 批注（Docx 文档级 + 行级） | `collab_doc_comments` 扩展 | DOC 内批注 ↔ 讨论 |

### v0.7.35 — SHEET 高级（3 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.35.1 | 评估 Univer 集成 vs 自建（已有公式 + 多 sheet） | ADR | 决策记录 |
| 0.7.35.2 | 自建方向：条件格式 / 数据验证 / 冻结窗格 / 隔行染色 | `CollabSheetEditor.vue` | — |
| 0.7.35.3 | 图表 + 透视表概念 | — | — |

### v0.7.36 — 后端生产化（持续）

| # | 任务 | 文件 |
| --- | --- | --- |
| 0.7.36.1 | 限流（per-tenant / per-IP / per-doc） | `internal/middleware/ratelimit.go` |
| 0.7.36.2 | Webhook（KB 同步 / 分享 / 评论） | `internal/handler/webhooks.go` |
| 0.7.36.3 | 审计日志查询 API + UI | `internal/handler/collaborative_doc_audit.go` + 前端列表页 |

---

## 四、关键技术决策记录

### 4.1 PackageArchive.zip 公开化（v0.7.30）

**问题**：`private readonly zip: JSZip` 在 vue-tsc 严格模式下，class
的 structural shape 必须包含 `zip`，导致 `OpenedPptx.archive` 的
structural 类型检查失败。

**决策**：`zip` → `readonly zip`（去掉 `private`）。运行时行为完全
不变（仅类型层面）。Genoffice 的上游在该 case 下未触发是因为其
tsconfig 配置与 WeKnora 不同。

### 4.2 `setSlideNotes` 签名（v0.7.30）

**问题**：`pptx-engine/notes.ts:107` 的真实签名是
`setSlideNotes(opened: OpenedPptx, slideIndex: number, text: string): boolean`，
但 adapter 错误地传 `slide.path`（string）。

**决策**：adapter 改用 `slideIndex`（number）参数。

### 4.3 后端审计日志表设计（v0.7.31 草案）

```sql
CREATE TABLE collab_doc_audit_log (
  id            BIGINT       PRIMARY KEY AUTOINCREMENT,
  tenant_id     INTEGER      NOT NULL,
  doc_id        VARCHAR(36)  NOT NULL,
  actor_id      INTEGER      NOT NULL,
  actor_name    VARCHAR(128) NOT NULL DEFAULT '',
  action        VARCHAR(32)  NOT NULL,    -- upload|save|share|archive|delete|comment|polish|sync_to_kb
  target        VARCHAR(64),              -- file_version, comment_id, ...
  payload       TEXT,                     -- JSON 详情
  ip            VARCHAR(64),
  user_agent    VARCHAR(256),
  created_at    DATETIME     NOT NULL,
  INDEX idx_audit_doc (doc_id, created_at),
  INDEX idx_audit_tenant (tenant_id, created_at)
);
```

**验收点**：每次 `SaveFile` / `UploadBytes` / `UpdateShareToken` /
`Archive` / `Delete` / `AddComment` / `SyncToKB` 都写一条。

---

## 五、验证命令

```bash
# 前端 TS
cd /Users/louloulin/appx/WeKnora/frontend && \
  ./node_modules/.bin/vue-tsc --build 2>&1 | grep -E "error TS" | grep -iE "collab"

# 前端 build
cd /Users/louloulin/appx/WeKnora/frontend && npm run build-only

# 前端 adapter 测试
cd /Users/louloulin/appx/WeKnora/frontend && \
  ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/*.test.ts

# 后端 build（跳过 types 包）
cd /Users/louloulin/appx/WeKnora && \
  go build ./internal/application/... ./internal/handler/... ./internal/router/...

# 后端测试
cd /Users/louloulin/appx/WeKnora && \
  go test ./internal/application/repository/ -count=1 -run "TestCollabDoc"
```

---

## 六、Token 经济

- 单 turn 内 vue-tsc + build-only + 19 个 adapter 测试 ~ 30 秒
- 后端 `go test` ~ 1 秒
- 全部增量打补丁（apply_patch 风格），每个 commit 影响 1-2 个文件


---

## 七、v0.7.37 完结后增量计划（v0.7.38 → v0.7.40）

> 承接 `ANALYSE_V2.md` 的能力矩阵与差距表。共 3 个版本号 / 6 周 / 1 人月。

### v0.7.38 — 飞书级最小可用（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.38.1 | PPT 格式面板（字体 / 颜色 / 对齐 / 边框） | `CollabSlideKonvaEditor.vue` + 新 `CollabFormatToolbar.vue` | 选中 shape 后可改 fill/stroke/font |
| 0.7.38.2 | DOC 文档级批注（mark + panel 接入） | `CollabDocProEditor.vue` + `CollabCommentsPanel.vue` | 选段 → 「批注」→ 显示在右侧 |
| 0.7.38.3 | SHEET 评论接入 | `CollabSheetEditor.vue` + `CollabCommentsPanel.vue` | 单元格右键 → 批注 |
| 0.7.38.4 | PPT 动画播放面板 | `CollabSlideKonvaEditor.vue` | 选中动画 → 预览 |
| 0.7.38.5 | DOC 选区 range awareness | `CollabDocProEditor.vue` + `useYjsCollabDoc.ts` | 远端选区高亮 |
| 0.7.38.6 | SHEET 公式栏 UI | `CollabSheetEditor.vue` | `=A1+B2` 输入 + 结果 |
| 0.7.38.7 | 后端 Slides audit hook 接通 governance | `container.go` + `service/slides/service.go` | 操作落 `audit_log` |
| 0.7.38.8 | 修 `desktop/` 329MB 二进制 .gitignore | `.gitignore` | git status 干净 |
| 0.7.38.9 | 适配层测试加 pptxFormatBrush / pptxThemesPanel 各 2 个 | `__tests__/` | pass |

### v0.7.39 — 高级渲染 + 飞书级特性（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.39.1 | SHEET 条件格式（cellRules JSON in Y.Map） | `CollabSheetEditor.vue` + `xlsxAdapter.ts` | 选中区段 → 规则 → 实时渲染 |
| 0.7.39.2 | SHEET 数据验证（dropdown / number range） | 同上 | — |
| 0.7.39.3 | SHEET 冻结窗格 | 同上 | — |
| 0.7.39.4 | SHEET 图表（自绘 SVG，bar/line/pie） | 新 `xlsxChartAdapter.ts` | 选区 → 「插入图表」→ SVG |
| 0.7.39.5 | DOC 公式 UI 输入 | `CollabDocProEditor.vue` | `/math` 触发 → mathDisplayParagraph |
| 0.7.39.6 | DOC 图表扩 11 类 | `docxAdapter.ts` | surface/radar/area/doughnut/scatter/bubble |
| 0.7.39.7 | DOC 表格 round-trip 完整 | `docxAdapter.ts` + pmDocToSavePlan | cell 颜色 / 合并保留 |
| 0.7.39.8 | PPT SmartArt（addSmartArtToSlide + dgm-hier） | `pptxShapeAdapter.ts` | 插入组织结构图 |

### v0.7.40 — 生产化（2 周）

| # | 任务 | 文件 | 验收 |
| --- | --- | --- | --- |
| 0.7.40.1 | 限流中间件（per-tenant / per-doc / per-IP） | `internal/middleware/ratelimit.go` | 429 响应 |
| 0.7.40.2 | WebHook（KB 同步 / 分享 / 评论） | `internal/handler/webhooks.go` + `useWebhookDelivery.ts` | 重试 3 次 |
| 0.7.40.3 | Audit 查询 UI 完善（filter panel + export CSV） | `CollabAuditTimeline.vue` | — |
| 0.7.40.4 | Slides 前端可视化编辑器（接 v0.7.37 后端） | 新 `CollabSlideDeckEditor.vue` | 创建 deck → 编辑 → 导出 |
| 0.7.40.5 | Slides 服务端 export .pptx | `service/slides/service.go` + pptxgenjs | export 字节 |
| 0.7.40.6 | 冲突解决策略（Yjs undo manager + auto-merge） | `useYjsCollabDoc.ts` | 双客户端冲突提示 |
| 0.7.40.7 | 公开分享密码 + 过期 | `internal/handler/collaborative_doc_bytes.go` | — |
| 0.7.40.8 | E2E Playwright 冒烟（doc/sheet/slide/slides） | `scripts/smoke-collab-docs.sh` | 全通过 |

### 验证命令

```bash
# 后端编译
cd /Users/louloulin/appx/WeKnora && go build ./...

# 后端测试
go test ./internal/application/repository/ -count=1 -run "TestCollabDoc|TestAuditRepo|TestSlide"

# 前端 TS
cd frontend && ./node_modules/.bin/vue-tsc --build 2>&1 | grep -E "error TS" | grep -iE "collab"

# 适配层测试
./node_modules/.bin/tsx --test src/editor/adapters/__tests__/*.test.ts

# E2E
cd .. && bash scripts/smoke-collab-docs.sh
```

### Token 经济

- 单 turn TS 类型校验 ~ 12-25s；adapter 测试 ~0.5s；backend test ~0.6s；build ~ 19-21s
- 每版本独立 commit；不要混入多个 phase 的 patch
