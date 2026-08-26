# Build #14 — wiki 批量操作审计日志

> 承接 Build #13 落地的 `wiki_batch_jobs` 表,补齐"谁在何时对哪些页面做了什么"
> 的可审计行。聚焦 KB 管理员需要回溯误操作、合规审计、追责的场景,
> 不展开为通用 audit trail(后者留给横向扩展 P1 的"全平台审计")。

Build #14 在 Build #13 的"作业状态机"基础上,记录 **每一次状态转换 + undo 操作**
作为不可变审计行,让 KB 管理员 / 平台审计员能:

- 看最近 7 天谁对哪些 slug 做了哪种 batch 操作(谁 + 何时 + 类型 + 结果)
- 追溯误操作的 undo 链路(原始 job + 后续 undo job)
- 导出 CSV 给合规

## 背景

Build #13 的 `wiki_batch_jobs` 表已经记录了:
- 入队时刻 + 类型 + 参数 + 结果
- 终态(`succeeded` / `failed` / `partial`) + undo 状态

但仍有两个缺口:

1. **过程态不可见** — `queued → running` 的过渡、`running → partial` 的中间态都没单独审计行,
   KB 管理员只能看到最终结果,看不到"这个 job 中途被中断过吗"。
2. **undo 操作无独立行** — undo 是 update `wiki_batch_jobs.expires_at = NULL` + 软恢复,
   审计员看不到"是谁在何时按了撤销"。Build #13 brief 已确认 undo 不另存副本
   (见 brief §"撤销后 slug 加后缀"),所以 undo 行本身就是审计事实。

参考 Build #7(ACL)在 `wiki_page_acl_audit` 表的设计模式(Build #7 已落地,
见 `migrations/versioned/000088_wiki_page_acl_audit.sql`):事件型 INSERT-only 表,
带 `actor_id` / `tenant_id` / `kb_id` / `action` / `target_slug` / `occurred_at`
/ `metadata JSONB`。

## 范围

### 1. 后端 — 审计事件表 + 触发点

#### 1.1 新表 `wiki_batch_job_audit`

迁移 `000093_wiki_batch_job_audit.sql`:

```sql
CREATE TABLE IF NOT EXISTS wiki_batch_job_audit (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    batch_job_id UUID NOT NULL,         -- 关联 wiki_batch_jobs.id
    action TEXT NOT NULL,                -- 'enqueue' | 'start' | 'finish' | 'undo_request' | 'undo_done' | 'expire' | 'cancel'
    actor_id TEXT NOT NULL,              -- 触发操作的用户, 'system' 表示 worker / 后台
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB                       -- 终态结果摘要 / undo 影响行数 / 错误码等
);
CREATE INDEX IF NOT EXISTS idx_wbb_audit_kb_occurred ON wiki_batch_job_audit (knowledge_base_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_wbb_audit_job ON wiki_batch_job_audit (batch_job_id);
CREATE INDEX IF NOT EXISTS idx_wbb_audit_actor ON wiki_batch_job_audit (actor_id, occurred_at DESC);
```

- `id BIGSERIAL` 而非 UUID:审计行数量大(每秒可能数十行),BIGSERIAL 写入更快、占用更小。
- `metadata` 用 JSONB 存"事件型补充字段",不强 schema:终态行写 `{succeeded: N, failed: M, codes: [...]}`,undo 行写 `{restored_count: N, skipped: M}`,cancel 行写 `{reason: 'queue_full'}`。

#### 1.2 触发点 — 在 `wikiBatchJobService` 关键路径落审计行

新增 `WikiBatchAuditRepository`(`internal/application/repository/wiki_batch_job_audit.go`),
提供 `Insert(ctx, event *WikiBatchJobAuditEvent) error`。

触发位置:

| 事件 | 触发点 |
|---|---|
| `enqueue` | `EnqueueJob` 写入 `wiki_batch_jobs` 后立刻追加 |
| `start` | worker 从 channel 取出并 advance 到 `running` 后 |
| `finish` | `executeJob` 终态落 `result` 后 |
| `undo_request` | `UndoJob` handler 入口(记录"用户何时点了 undo") |
| `undo_done` | `UndoJob` 完成(成功 / 失败两种 metadata) |
| `expire` | 周期 cleanup(每日跑一次)扫到 `expires_at < NOW() AND expires_at_was_set` |
| `cancel` | `EnqueueJob` channel 满时降级同步执行的分支 |

`enqueue` 和 `start` 之间可能跨秒级,保留两行便于"卡在 queued 多久"分析。

#### 1.3 Handler — 新增查询 / 导出端点

- `GET /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/audit` — 单 job 完整审计链
- `GET /api/v1/knowledgebase/<kb>/wiki/batch-audit?since=<RFC3339>&actor=<id>&action=<enqueue|undo_done|...>&page=N` — 按 KB 维度分页查询,默认 `since=now-7d`,最大 `since=now-90d`
- `GET /api/v1/knowledgebase/<kb>/wiki/batch-audit/export?since=<...>` — 返回 `text/csv`,UTF-8 BOM,列:`occurred_at,actor_id,action,job_id,job_type,job_state,slugs_affected,error`

权限:`OwnedWikiKBOrAdmin` + `KBAccessRead`;`export` 限管理员(`OwnedWikiKBOrAdmin` 已覆盖)。

### 2. 前端 — 历史抽屉 + 列表

#### 2.1 `WikiBatchJobHistory.vue`(新组件)

抽屉式(`t-drawer`,宽度 720px),挂在 `WikiBrowser` 顶部工具栏的"🕓 历史"按钮(Build #14 新增)。

#### 2.2 内容布局

- 顶部:`t-date-picker` 范围选择 + 操作类型 `t-select` 过滤 + 操作者 `t-input` 搜索
- 主表:`t-table`,列 `时间 / 操作者 / 动作 / Job ID / 类型 / 结果摘要`,行可点击展开 → `t-table` 子表显示 `metadata`(JSON 渲染)
- 底部:"导出 CSV"按钮 → 调 `GET .../audit/export?since=...` → 浏览器下载

#### 2.3 单 job 详情嵌入

`WikiBulkActionBar` 的 toast 完成态(`succeeded` / `partial` / `failed` / `undo_succeeded`)
追加"查看历史"链接 → 跳到 `WikiBatchJobHistory` 并预过滤到该 `job_id`。

### 3. i18n — 9 keys × 4 locale

挂在 `knowledgeEditor.wikiBrowser` 命名空间下:

| key | 中文 | 英文 |
|---|---|---|
| `bulkAuditTitle` | 批量操作历史 | Batch job history |
| `bulkAuditFilterDate` | 时间范围 | Date range |
| `bulkAuditFilterActor` | 操作者 | Actor |
| `bulkAuditFilterAction` | 动作类型 | Action |
| `bulkAuditExportCsv` | 导出 CSV | Export CSV |
| `bulkAuditEmpty` | 暂无记录 | No records yet |
| `bulkAuditActionEnqueue` | 提交任务 | Enqueue |
| `bulkAuditActionUndoDone` | 撤销完成 | Undo done |
| `bulkAuditViewJob` | 查看 Job | View job |

## 关键决定(D1–D5)

- **D1 事件粒度** — 6 个事件够用,不细到 per-slug(per-slug 数据已在 `result.succeeded/failed` JSONB 里,审计行只存摘要计数)
- **D2 actor 字段保留** — 删 / undo 操作者都记,即使是 worker 自动 expire 也用 `'system'`
- **D3 BIGSERIAL 主键** — 审计行量大,UUID 写入开销不划算
- **D4 90 天上限** — 查询 `since` 最大 90 天,避免单次返回数百万行把前端卡死;更老的进归档表(留 P2)
- **D5 权限复 `KBAccessRead`** — 审计是 KB 内的可读行为,不另开 `audit_admin` 角色

## 范围之外(显式 not-in-scope)

- per-slug 审计行(只在 `metadata` 里存摘要)
- 跨 KB 全平台审计(留给横向 P1 的"全平台审计"模块)
- 90 天以上历史归档(留 P2)
- 实时审计流(WS 推送,留 Build #15 / #23 之后)
- `audit_admin` 独立角色(目前复 `KBAccessRead`)

## 验收矩阵(13 项)

### 后端 A1–A7
- A1:迁移 000093 干净执行 / 回滚
- A2:`enqueue` 事件在 `Batch*PagesRoute` 入队后立即落审计行
- A3:`start` / `finish` 在 worker 推进时落审计行
- A4:`undo_request` / `undo_done` 在 `UndoJob` 落审计行
- A5:channel 满降级路径落 `cancel` 审计行
- A6:`GET .../audit` 查询返回分页 + 过滤
- A7:`GET .../audit/export` 返回 CSV

### 前端 A8–A11
- A8:`WikiBatchJobHistory` 抽屉组件
- A9:过滤器(date / actor / action)生效
- A10:CSV 导出浏览器下载
- A11:`WikiBulkActionBar` 完成态加"查看历史"链接

### 通用 A12–A13
- A12:smoke `scripts/smoke-wiki-audit.sh`
- A13:本地 verify(vue-tsc + build + i18n + 9 keys × 4 locale + commit)

## 待你拍板的 1 件事

**D3 主键选择**:用 `BIGSERIAL`(写快)还是 `UUID`(跨实例唯一)?

- BIGSERIAL:适合单实例,Build #14 当前架构匹配
- UUID:留作多实例分布式审计的伏笔,但增加 ~30% 写入开销

回我"D3 用"或"按 brief 走"就开始。要调整范围也直说。