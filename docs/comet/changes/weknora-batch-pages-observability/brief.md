# Build #15 — wiki 批量操作可观察性(progress + 错误聚合)

承接 Build #13(async + undo)+ Build #14(audit),Build #15 解决用户最痛的剩余体验问题:

> "我点了一次批量移动 100 条页面,等了 30 秒,什么也没发生 — 我不知道它跑到第几条了。"

## 范围

| 层 | 内容 |
|---|---|
| **后端** | 1 个新字段 `progress JSONB` 在 `wiki_batch_jobs` + 1 个新表 `wiki_batch_job_failures`(per-slug 失败明细) + worker 内逐条迭代 + 节流更新 + `GET /batch-jobs/:id/failures` 错误聚合接口 |
| **前端** | Toast 升级:实时 `{processed}/{total} 已处理` 文本 + 完成态"查看错误"链接 → 打开 `WikiBatchFailureDrawer`(按 code 分组的错误明细 + slug 列表) |
| **i18n** | 8 keys × 4 locale(progress / failure drawer / 错误分组标题) |

## 后端 schema delta

### 字段新增

```sql
-- 000094_wiki_batch_job_progress.up.sql
ALTER TABLE wiki_batch_jobs
  ADD COLUMN IF NOT EXISTS progress JSONB;

-- progress JSONB 形态:
-- {
--   "total": 100,
--   "processed": 23,
--   "succeeded": 21,
--   "failed": 2,
--   "updated_at": "2026-08-26T05:00:00Z"
-- }
```

字段选 JSONB 而非 4 个 INT,理由:
- Build #17 批量加 tag 时,progress 可能需要加 `{tag_added, tag_skipped}` 等扩展字段
- 单列更新 vs 4 列原子写,JSONB 减少 future migration 数量
- 写入频率 ~每秒 1 次,JSONB 写入开销可忽略

### 新表

```sql
-- 000094_wiki_batch_job_failures.up.sql
CREATE TABLE IF NOT EXISTS wiki_batch_job_failures (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    batch_job_id UUID NOT NULL,
    slug TEXT NOT NULL,
    code TEXT NOT NULL,                  -- not_found / folder_not_found / ...
    error TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wbbf_job ON wiki_batch_job_failures (batch_job_id);
CREATE INDEX idx_wbbf_kb_code ON wiki_batch_job_failures (knowledge_base_id, code);
```

| 设计点 | 选择 |
|---|---|
| 为什么不直接用 `wiki_batch_jobs.result.failed[]` | 大批量(>1000)时 result blob 膨胀,handler 一次吐出;failures 表支持分页查询 + 按 code 分组聚合 |
| 为什么不用 `wiki_batch_job_audit.metadata` | audit 表面向"何时谁做了啥"事件流,failures 表面向"哪条 slug 因为什么挂了"诊断数据;两表职责分离 |
| 为什么不写完整 result blob 到 failures | failures 只记录失败行,成功行不进表(成功率高时不浪费空间) |
| BIGSERIAL 主键 | 与 Build #14 audit 表风格一致;写多读少,UUID 开销不划算 |

## Worker 改造

```
executeJob (Build #13 现状):
  dispatchBatch(ctx, job)  →  WikiBatchResult{succeeded[], failed[]} 一次性返回

executeJob (Build #15 后):
  for slug in job.params.slugs:
      按 type 调单条方法 (MovePage / DeletePage / UpdatePageStatus)
      result = ...
      processed++, succeeded++ / failed++
      if failed → insert wiki_batch_job_failures
      if processed % 5 == 0 or processed == total:
          UPDATE wiki_batch_jobs SET progress = ... (节流)
```

### 关键决定

- **D1 节流 5 条** — 100 条批量 ≈ 20 次 UPDATE,DB 压力可控;UI 实时感够用
- **D2 复用单条方法** — `MovePage` / `DeletePage` / `UpdatePageStatus` 已存在,避免重构 Batch* sync 方法
- **D3 failures 写库在事务内** — 与 worker Update 同事务,保证 progress 计数与 failures 表一致(失败的不丢、成功的不少)
- **D4 sync 路径不节流** — <20 走同步,执行 < 1s,前端 toast 不轮询,无需 progress

### 范围之外(not-in-scope)

- 实时 WS 推送 progress(留 Build #23 之后)
- 进度条 / 百分比 UI(只显示文本计数;进度条留给横向 P2 UI 升级)
- CancelJob 时回滚已成功的子操作(与 Build #14 一致:best-effort)
- progress 推送到 audit 表(避免审计膨胀;failures 表已记录原因)

## 前端 UI 改造

### Toast 升级(在 WikiBrowser.vue watchBatchJob 中)

```ts
// Build #13 现状:content: '已入队 (id: abc12345)' 持续不变
// Build #15 后:每次 poll 拿新 progress 更新文本
content: t('knowledgeEditor.wikiBrowser.bulkJobProgress', {
  processed: job.progress?.processed ?? 0,
  total: job.progress?.total ?? 0,
})
// "23/100 已处理"
```

由于 tdesign MessagePlugin 没暴露 `replace`,沿用 Build #13 的 close-then-emit 模式:每次 poll 拿到新 progress 就 close loading toast + emit 新 toast(短 duration=2000ms 让它自动消失)。每 2 秒一次轮询,用户感觉是"实时刷新"。

### 错误聚合 drawer

完成态时若 `state === 'partial' || failed > 0`,在最终 toast 加"查看错误"按钮(`MessagePlugin` 的 `onClick` 钩子),点击打开 `WikiBatchFailureDrawer`:

- 标题:`批量错误 (id: {shortID})`
- 顶部:按 code 分组的 tab / 折叠面板(`not_found` × 12 / `folder_conflict` × 3)
- 表格:`slug` + `error` + `code` 列
- 分页(>50 条)

## 关键决定(D1–D6)

- **D1 节流 5 条** — 写 DB 频率可接受 + UI 实时感
- **D2 复用单条方法** — 不动 Batch* sync 公共方法签名
- **D3 failures 写库同事务** — 与 progress Update 同 Tx
- **D4 sync 路径不节流** — <20 同步 < 1s,无 progress 需求
- **D5 progress JSONB 而非 4 列** — 未来扩展 + 减少 ALTER
- **D6 错误 drawer 仅 `partial`/`failed` 时显示** — `succeeded` 不打扰

## 范围之外(not-in-scope)

- 跨 KB 全平台审计(横向 P1)
- 90 天以上历史归档(横向 P2)
- 实时审计 WS 推送(留 Build #23 之后)
- 进度条 UI(横向 P2 UI 升级)
- per-slug 审计行(失败明细在 `wiki_batch_job_failures`,成功不进表)

## 验收矩阵(15 项)

### 后端 A1–A8
- A1 migration 000094(progress 字段 + failures 表 + 2 索引)
- A2 worker 逐条迭代 + 节流 5 条 update progress
- A3 failures 写库与 progress update 同事务
- A4 progress 字段正确反映 processed/succeeded/failed
- A5 `GET /batch-jobs/:id/failures` 分页 + 按 code 分组聚合
- A6 harness 测试覆盖:全成功 / 全失败 / 部分失败 / 节流 5 条触发 1 次 update
- A7 7 审计事件仍正确触发(Build #14 不回归)
- A8 smoke `scripts/smoke-wiki-batch-observability.sh` 干运行

### 前端 A9–A13
- A9 WikiBatchJob 类型加 `progress` 字段
- A10 API client 加 `getWikiBatchJobFailures`
- A11 `WikiBatchFailureDrawer`(分组 + 分页 + slug 列表)
- A12 `watchBatchJob` 每次 poll 更新 toast 文本(节流 2s 一致)
- A13 finalizeToast partial 时加"查看错误"按钮

### 通用 A14–A15
- A14 i18n 8 keys × 4 locale
- A15 vue-tsc + build + i18n-check + npm test 全部干净

## 待你拍板的 1 件事

**D7 单条方法 vs 整批方法**:

- **A 拆单条** (推荐) — 复用 `MovePage` / `DeletePage` / `UpdatePageStatus`,每个 slug 单独跑,worker 控制节流。DB 写入 ~Nx,但 progress 真实。
- **B 一次性 + 后台拉** — Batch* 整批一次跑,worker 跑完后只回填最终 counts。progress 只在终态更新,前端看不到中间过程。

回我"D7 用 A"或"D7 用 B"或"按 brief 走(A)"就开始。调整其他范围也直说。