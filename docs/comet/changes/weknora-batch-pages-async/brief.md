# Build #13 — wiki 批量操作:异步队列 + Undo

> 承接 Build #12 留下的两个明确 not-in-scope 项("后台异步队列" 与 "Undo 撤销栈"),
> 把批量操作从"小批量同步 + 大批量卡死前端"升级为"批量按规模自适应 + 出错可撤"。

## 背景

Build #12 上线了 3 个批量端点(`batch-move` / `batch-delete` / `batch-status`),通过 100 条上限
(`MaxWikiBatchSize`)做了 DB 锁保护,但仍有两个 UX 痛点:

1. **大批量阻塞** — 100 页同步执行要花数秒到十几秒,前端 fetch + render 阻塞,
   用户看到"已发送 100 条"然后浏览器 spinner 一直转。生产 KB 经常有 > 100 页的运营动作
   (季度归档、清空草稿、整目录搬迁)。
2. **没有撤销** — 批量删除是软删除,理论上可恢复,但前端没有"撤销"按钮,误操作后只能去 DB 里
   `UPDATE wiki_pages SET deleted_at = NULL WHERE ...`,运维成本极高。

Build #12 brief 显式 not-in-scope:
> **后台异步队列**:本 Build 是同步请求,前端直接等响应;长任务走队列是 Build #14+ 的话题。
> **Undo 撤销栈**:批量删除没 undo,显式由用户确认弹窗兜底(参 D7)。

Build #13 把这两块补齐,**沿用 Build #12 的 partial-success 语义 + 公共 `WikiBatchResult` 类型**。

## 范围

### 1. 后端 — 异步任务表 + job 调度

#### 1.1 新表 `wiki_batch_jobs`

迁移 `000092_wiki_batch_jobs.sql`:
- `id` UUID
- `tenant_id` / `knowledge_base_id`(防跨 KB 访问)
- `type` enum: `move` / `delete` / `status` / `tag`(预留 tag 给 Build #15)
- `slugs` JSONB(`{"slugs": [...], "folder_id": "..."}` 等按 type 序列化参数)
- `state` enum: `queued` / `running` / `succeeded` / `failed` / `partial`
- `result` JSONB(`{"succeeded": [...], "failed": [{slug, code, error}]}`)
- `created_by` user_id
- `created_at` / `started_at` / `finished_at`
- `expires_at`(`finished_at + 7 天`,过期前可触发 undo)
- 索引: `(knowledge_base_id, state)`, `(expires_at)` for cleanup job

#### 1.2 Service — `wikiPageService.BatchMovePages` 等保持同步不变,新增异步包装

- 保留 Build #12 的 `BatchMovePages(ctx, kbID, slugs, folderID) -> (*WikiBatchResult, error)` 接口不动
- 新增 `BatchMovePagesAsync(ctx, kbID, slugs, folderID) -> (jobID string, error)`
- 阈值:`len(slugs) > 20` 走 async(20 条以下同步,因为同步路径前端等待 < 1s 是 OK 的)
- 异步执行器:简单 goroutine pool(全局 4 worker)+ channel,先不接 asynq/asynq-cron
  - 在 `cmd/server/main.go` 启动时初始化 worker pool
  - job 入库后 `chan <- jobID`,worker 取出后读 slugs + 调同步方法 + 写 result
  - `BatchUndoJob(ctx, kbID, jobID) -> error` 撤销指定 job

#### 1.3 新 `UndoJob` 类型

- 仅 `move` 和 `delete` 可撤销(`status` 撤销 = 再改一次,不是真的"撤回")
- `move undo`:把页面的 folder_id 改回原值。需要保存原值 → `wiki_batch_jobs.undo_state` JSONB
- `delete undo`:把 `deleted_at` 置 NULL。GORM 软删除天然支持 `Unscoped().Update(...)`,
  但需要保留页面原 slug(因为恢复时 slug 唯一索引可能冲突)。
  策略:把待恢复页面的 slug 加 `__restored_<short-id>` 后缀,恢复成功后由后续 batch 重命名回原 slug
  (此简化版先接受后缀,后续 Build 可优化)。

#### 1.4 路由

- `POST /api/v1/knowledgebase/:kb_id/wiki/pages/batch-move` — 现有签名保持,**自动**根据 slugs 数量选同步/异步
- `GET  /api/v1/knowledgebase/:kb_id/wiki/batch-jobs/:job_id` — 查 job 状态 + 结果
- `POST /api/v1/knowledgebase/:kb_id/wiki/batch-jobs/:job_id/undo` — 撤销

### 2. 前端 — 进度轮询 + Undo 按钮

#### 2.1 `api/wiki/batch.ts` 新增

- `getWikiBatchJob(kbId, jobId) -> Promise<WikiBatchJob>`(轮询,默认 2s 间隔,指数退避封顶 10s)
- `undoWikiBatchJob(kbId, jobId) -> Promise<WikiBatchJob>`

#### 2.2 `WikiBulkActionBar.vue` 改造

- 同步路径(< 20 条):保留当前 toast 行为(`X 成功 / Y 失败`)
- 异步路径(≥ 20 条):
  - 提交后立即弹一个 **持久化 toast**(不自动关闭),显示 "Job #abc 已入队,处理中…(5/100 已完成)"
  - 轮询 `getWikiBatchJob`,完成后 toast 更新成 "X 成功 / Y 失败 / [撤销] 按钮"
  - **撤销按钮 60s 内可点**,过期后隐藏(因为 `expires_at = finished_at + 7 天` 是后端永久撤销窗口,
    前端 60s 是 UX 上的"我点错了"窗口;点过后端真撤销)
- 已有 `WikiBulkActionBar` 的 4 个按钮(Move / Status / Delete / Clear)逻辑不变,
  只需在 emit 后判断返回类型(sync result vs async job)

#### 2.3 i18n — 加 14 条

- `bulkJobQueued`:`批量任务已入队(#abc123),处理中…`
- `bulkJobProgress`:`{processed}/{total} 已处理`
- `bulkJobSucceeded`:`批量完成:{succeeded} 成功 / {failed} 失败`
- `bulkJobFailed`:`批量失败:{error}`
- `bulkJobUndoButton`:`撤销`
- `bulkJobUndoHint`:`60 秒内可点`
- `bulkJobUndoing`:`正在撤销…`
- `bulkJobUndoSucceeded`:`已撤销`
- `bulkJobUndoFailed`:`撤销失败:{error}`
- `bulkJobUndoExpired`:`已超过撤销时限`
- `bulkJobPollError`:`查询进度失败`
- `bulkConfirmAsyncTitle`:`确认批量操作(后台执行)?`
- `bulkConfirmAsyncHint`:`所选页数超过 20 条,操作将在后台进行;完成后 toast 会显示进度,可点撤销按钮撤回(60s 内)。`
- `bulkConfirmAsyncConfirm`:`开始`

### 3. 测试

- 后端 harness(`internal/application/service/wiki_page_batch_async_test.go`):
  - 同步路径(< 20)走原 `BatchMovePages`,result 直接返回
  - 异步路径(> 20)写入 `wiki_batch_jobs` 后立即返回 job_id
  - worker 取出后调同步路径,state 推进,result 落库
  - `BatchUndoJob` 对 `move` 和 `delete` 各自跑通;`status` 返回 `not_undoable` 错误
  - `expires_at` 过期后 `Undo` 返回 410 Gone
- 前端 vitest:
  - `WikiBulkActionBar` 异步路径:submit → toast 出现 → 模拟轮询完成 → 撤销按钮显示
  - 撤销按钮 click → 调 `undoWikiBatchJob` → toast 更新为"已撤销"
  - 60s 后撤销按钮自动隐藏(用 fake timers)

### 4. smoke 脚本

`scripts/smoke-wiki-batch-async.sh`:
- 创建 30 页(超阈值)→ 批量 delete → 验证返回 job_id → 轮询直到 succeeded → 撤销 → 验证页面重新可见

## 决定(D1–D8)

| # | 决定 | 取舍 |
|---|---|---|
| **D1** | 同步/异步阈值 = **20 条** | 小于 20 同步(响应 < 1s 用户无感);大于等于 20 异步 |
| **D2** | 异步 worker 用 **进程内 goroutine pool**,不上 asynq | 减少基础设施依赖,4 worker 是当前 KB 数 4×30 = 120 页/批 的吞吐上限; |
| | | 上 asynq 是 P2 范围(多实例部署时再做)。单实例够用 |
| **D3** | 撤销窗口(后端永久 / 前端 60s) | 后端 `expires_at = finished_at + 7d` 真撤销;前端 60s 是"我刚点错"窗口,过期仅隐藏按钮 |
| **D4** | `status` 不可撤销 | status 撤销 = 再调一次 BatchUpdatePageStatus,语义奇怪;返回 `not_undoable` 让前端不显示按钮 |
| **D5** | 异步执行**复用同步 Batch* 方法**,不重写逻辑 | worker 取出后调 `BatchMovePages(ctx, kbID, slugs, folderID)`,拿到 `WikiBatchResult` 直接落库;partial-success 语义不变 |
| **D6** | Undo 不重新走 ACL/守卫检查 | job 创建时已经过守卫;Undo 是 admin/owner 信任操作,前端按钮所在 context 已限 owner;服务端 Undo 端点仍走 `OwnedWikiKBOrAdmin` |
| **D7** | 撤销后 slug 加 `__restored_<id>` 后缀 | 简单恢复;唯一索引冲突解决方案,后续 Build 可优化为"先改老 slug 再恢复" |
| **D8** | 不接 asynq / 不用 Redis queue | 进程内 channel 够用,简单可控;真要分布式再说 |

## 范围之外(显式 not-in-scope,留给后续)

- 跨 KB 异步批量 / 异步批量加 tag / 批量改 title/content(留给 Build #15)
- Undo 撤销 Undo(防止撤销栈溢出,Undo 操作本身不产生新 job)
- 多实例部署下 job 抢跑(asynq 接管时才需要)
- 实时进度推送(WebSocket)— Build #8.5 不在本月排期
- 撤销后 slug 重命名(去 `__restored_` 后缀)— 简单加 slug 后缀够用,优化留给后续

## 验收口径

详见 `specs/wiki-batch-async/spec.md`(A1–A16)。

## 实施路径

1. 分支 `lumos0826-batch-async`(off `lumos0826`,沿用 #7/#10/#11/#12 的直分支模式)
2. 后端:迁移 `000092` + `wikiBatchJobs` 表 + `WikiBatchJobService` + worker pool + 同步端点自动路由
3. 前端:`getWikiBatchJob` / `undoWikiBatchJob` API + `WikiBulkActionBar` 持久化 toast + 进度轮询 + 撤销按钮
4. 4 locale × 14 条 i18n
5. 后端 harness 单测 + 前端 vitest + smoke 脚本
6. 本地验证:`vue-tsc` / `npm run build` / `npm test` / `check-i18n`
7. commit → fast-forward merge `lumos0826` → push origin → LUM-20 reply(commit hash)

后端 Go 沙箱不可跑,沿用 #12 口径(代码遵循 service/handler/route 模式、smoke 兜底、用户本地验证)。

## 待你拍板的 1 件事

**async worker 容量**(D2 留的口子):如果你的 wiki KB 平均 4 倍于当前负载(每个 KB 4 个并发 batch
job),那 4 worker 是上限;如果 KB 数量还要涨,可以提到 8(代码改一个常量就行)。
是否就用 4?