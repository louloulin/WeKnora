# Build #16 brief — wiki 批量操作预览 / Dry-run

## 目标

让用户在点"确认执行"前先看一份**只读结果**:哪些 slug 会成功、哪些会因为 `not_found` / `folder_conflict` / `kb_mismatch` 等原因失败、整体摘要。无 DB 改动,纯校验。

沿用 Build #13/15 已有的 `MovePage` / `DeletePage` / `GetBySlug+UpdatePageMeta` 路径,只是包在一个**事务回滚 + 只读 flag**的 wrapper 里,避免重复实现校验逻辑。

## 范围

| 层 | 内容 |
|---|---|
| **后端** | 1 个新接口 `POST /wiki/pages/batch-preview`(支持 move/delete/status),返回 `WikiBatchPreviewResponse`(success/error per-slug + summary 计数),handler 层调 service 的 `PreviewBatch(type, params)`,事务内回滚 → 真实模拟,但不写 |
| **前端** | `WikiBulkActionBar` 新增"预览"按钮(在"确认"按钮前):调 preview,展示 `WikiBatchPreviewDialog`(summary 计数 + per-slug 表格:slug + status icon + code/error/will-succeed 标识),底部"取消"+"确认执行"(用户确认后才走真实 batch-move/-delete/-status) |
| **i18n** | 8 keys × 4 locale(`wiki.batchPreview.{title, summary, willSucceed, willFail, unknown, confirm, cancel, empty}`) |

## 核心设计

### 后端

1. **新增 endpoint** `POST /api/v1/knowledgebase/{kb_id}/wiki/pages/batch-preview`,body 复用 `WikiBatchMoveBody` / `WikiBatchDeleteBody` / `WikiBatchStatusBody` 形状,handler 按 path 选 type:`POST /batch-preview-move` / `/batch-preview-delete` / `/batch-preview-status`(避免一个端点靠 body 字段猜 type,Swagger 也清晰)

   - 选用 3 个独立端点而不是 1 个通用端点,理由:`batch-move` 等3 个 endpoint 已经按 verb 拆开了,preview 是同一逻辑的只读兄弟,跟随现有 verb 拆法保持 API 表层一致

2. **service 新方法** `WikiBatchJobService.PreviewBatch(ctx, kbID, type, params)`:
   - 启 Tx
   - 对每个 slug 调对应的单条方法(`MovePage` / `DeletePage` / `applyStatusOne`),捕获 error 并 classify 到 code
   - Tx rollback(永远回滚,无副作用)
   - 返回 `{success: [{slug}], failed: [{slug, code, error}], summary: {total, willSucceed, willFail}}`

3. **不写 wiki_batch_jobs / audit / failures** — dry-run 是只读,不占用异步队列,不记审计

4. **不挂 progress 字段** — 同步返回结果,无轮询

### 前端

1. **`WikiBulkActionBar.vue` 改造**:
   - 当前 "确认" → 点击直接 POST batch-* 端点
   - 新流程 "预览 → 确认" 两步:第一按钮 "预览",第二按钮(在 dialog 中)"确认执行"
   - 当 selected slugs 数 < WikiBatchAsyncThreshold(20)时,sync 路径无需预览(立即出结果);≥ 20 时,预览更值钱(避免 100 条异步任务跑了一半才看到冲突)

2. **`WikiBatchPreviewDialog.vue`** 新组件:
   - 顶部 summary:`5 条会成功 / 3 条会失败`(counts from response.summary)
   - 主体 per-slug 表格:slug + icon(✓/✗)+ code badge(error 时)+ error message(failed 时)+ 当前状态(status job 时显示 "draft → published" diff)
   - 底部 "取消" / "确认执行" 两按钮;确认 → 关闭 dialog,触发原本的 POST batch-* 流

3. **状态机衔接**:
   - PreviewDialog 关闭后,WikiBulkActionBar 不变(slug 仍选中,可重新点预览 / 点取消)
   - 确认执行后走原有 batch-move/delete/status POST(sync or async by threshold),toast + progress 沿用 Build #13/#15

## 关键决定(D1–D6)

- **D1 三端点 vs 一端点** — 三端点,跟随现有 verb 拆分;Swagger 更清晰
- **D2 Tx rollback 而非 mock validator** — 真实走单条方法,意味着 dry-run 也会触发 `MovePage` 内部的"如果目标 folder 是父级就报错"等所有真实校验,准确度最高;代价是稍慢(100 slugs < 100ms)
- **D3 同步返回** — 不挂异步队列(预览用户正盯着 UI,等几秒就够;preview 本身是 IO+Tx<500ms)
- **D4 仅对 ≥ WikiBatchAsyncThreshold 的 batch 显示预览按钮** — 小批量(≤ 19 条)直接走原 sync 路径,无需多这一步;但为了 API 表面统一,后端接口始终可用
- **D5 不写 audit / failures** — dry-run 是只读,不入审计流
- **D6 dialog 是 modal 不能关闭后误触发** — 用户必须主动点"确认执行"才会 POST,避免误操作

## 范围之外(not-in-scope)

- 跨 KB 预览过滤(同 batch-* 行为,KB 不匹配返回 400,不进 dry-run)
- 实时 preview(预览是 click-triggered,不订阅)
- "dry-run 模式"开关(预览就是独立 endpoint,无需开关)
- 把 preview 结果存表(预览不持久化)
- preview 触发 audit 事件

## 验收矩阵(14 项)

- 后端 A1–A5:migration 无(只复用现有表)/ `PreviewBatch` service / `BatchPreview*` 3 endpoints + Swagger / Tx rollback 验证 / harness
- 前端 A6–A10:`WikiBatchPreviewDialog` 组件 / `WikiBulkActionBar` 改造(预览按钮 + 确认流)/ API client `batchPreview*` / per-slug 表格 + summary / "确认执行" 走原 POST 流
- 通用 A11–A14:i18n 8 keys × 4 locale / smoke script / vue-tsc + build + check-i18n / 本地 verify 全绿

## 待你拍板的 1 件事

**D7 Preview UI 触发条件**:

- **A 仅 ≥ 20 slugs 时显示预览按钮**(推荐)— 小批量跳过预览,减少点击;大批量才值得
- **B 总是显示预览按钮** — 一致 UI,用户每次都先看预览

回我 "D7 用 A" / "D7 用 B" / "按 brief 走(A)" 就开始。

调整其他范围也直说。