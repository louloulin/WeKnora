# Build #13 — wiki 批量操作:异步队列 + Undo(spec)

## 验收矩阵

### 后端

#### A1 — 异步 job 表 + 迁移
- `internal/migrations/000092_wiki_batch_jobs.sql` 创建 `wiki_batch_jobs` 表
- 所有字段按 brief 1.1 节落库
- 索引:`(knowledge_base_id, state)` 和 `(expires_at)`

#### A2 — 同步/异步自动路由
- `BatchMovePages(ctx, kbID, slugs, folderID)` 内部:`len(slugs) >= 20` 时入队,返回 `(jobID, nil)`
- `len(slugs) < 20` 时同步执行,返回 `(*WikiBatchResult, nil)`(原行为不变)
- `BatchDeletePages` / `BatchUpdatePageStatus` 同样策略

#### A3 — Job 状态机
- `queued → running → (succeeded | failed | partial)`
- worker 取出后立即写 `running` + `started_at`
- 完成后写终态 + `finished_at` + `result`
- 失败时 `result.error` 字段填 error message

#### A4 — Worker pool
- 全局 4 worker,`cmd/server/main.go` 启动时初始化
- channel buffer = 32;超出时同步路径自动降级(避免无限增长)
- graceful shutdown:close channel + WaitGroup

#### A5 — Job 查询
- `GET /api/v1/knowledgebase/:kb_id/wiki/batch-jobs/:job_id`
- 返回 `WikiBatchJob` JSON(state + result + timestamps + undoable 标记)
- 权限:`OwnedWikiKBOrAdmin` + `KBAccessWrite("kb_id")`
- 跨 KB / 不存在的 job → 404

#### A6 — Undo 端点
- `POST /api/v1/knowledgebase/:kb_id/wiki/batch-jobs/:job_id/undo`
- `type = move`:`undo_state.folder_id` 写回每页
- `type = delete`:每页 `deleted_at = NULL` + slug 加 `__restored_<short-id>` 后缀
- `type = status`:返回 422 `not_undoable`
- `expires_at` 已过期:返回 410 Gone
- 不存在 / 跨 KB:404

#### A7 — Harness 单测(`wiki_page_batch_async_test.go`)
- `TestBatchMovePages_SyncPath_Lt20`:< 20 条同步返回,result 立即填充
- `TestBatchMovePages_AsyncPath_Gte20`:≥ 20 条入队,返回 job_id,result 留空
- `TestWorkerPool_PicksUpQueuedJob`:worker 取出 job 后 state → running → succeeded
- `TestUndoJob_MoveRestoresFolder`:undo 后所有页 folder_id 回到原值
- `TestUndoJob_DeleteRestoresPages`:undo 后页面可见,slug 带 `__restored_` 后缀
- `TestUndoJob_StatusReturnsNotUndoable`:status 类型返回 422
- `TestUndoJob_AfterExpiryReturns410`:模拟过期时间,返回 Gone
- `TestCrossKBJobReturns404`:跨 KB 访问 job_id 返回 404

### 前端

#### A8 — API client 扩展
- `getWikiBatchJob(kbId, jobId): Promise<WikiBatchJob>`
- `undoWikiBatchJob(kbId, jobId): Promise<WikiBatchJob>`
- 类型 `WikiBatchJob` 包含 state / result / timestamps / `undoable: boolean`

#### A9 — 持久化 toast + 进度轮询
- `WikiBulkActionBar` 在收到 `{ kind: 'job', jobId }` 时(对比 `{ kind: 'sync', result }`)
  - 创建持久化 toast(用 `MessagePlugin.loading` 然后 `MessagePlugin.replace`)
  - 启动 `setInterval` 2s 轮询 `getWikiBatchJob`
  - state ∈ {succeeded, failed, partial} → 停止轮询 + 更新 toast 文案 + 显示撤销按钮

#### A10 — 撤销按钮 + 60s 窗口
- 撤销按钮 click → 调 `undoWikiBatchJob` → 成功后 toast 文案变 "已撤销"
- 60s 后按钮自动隐藏(用 `setTimeout` 配合 timestamp 比较)
- `undoable === false`(status 类型 job)→ 不显示撤销按钮

#### A11 — i18n 14 条
- 4 locale × 14 keys,见 brief 1.4 节
- `check-i18n` 通过

#### A12 — 异步确认弹窗
- ≥ 20 条提交时先弹 `t-dialog` 解释"将后台执行 + 可撤销"
- 用户确认后才发请求,避免误触发大批量

#### A13 — Frontend vitest
- `WikiBulkActionBar.test.ts` 新增 3 个用例:
  - 异步提交 → 模拟 `getWikiBatchJob` 返回 succeeded → toast 显示成功 + 撤销按钮出现
  - 撤销按钮 click → 调 `undoWikiBatchJob` → toast 文案变 "已撤销"
  - 60s 后撤销按钮自动隐藏(fake timers)

### 通用

#### A14 — Smoke 脚本 `scripts/smoke-wiki-batch-async.sh`
- 创建 30 页 → 批量 delete → 验证返回 job_id → 轮询直到 succeeded → 撤销 → 验证页面 slug 带 `__restored_` 前缀

#### A15 — 本地验证
- `vue-tsc --noEmit` 0 errors
- `npm run build` clean
- `npm test` 通过(允许遗留的 ACL workspaceTerminology 字符串 fail)
- `npm run check-i18n` 11/11 pass

#### A16 — Commit + push + reply
- 单一 commit 信息 `feat(wiki): Build #13 async batch jobs + undo support`
- 合并到 `lumos0826`,推送 origin
- LUM-20 reply 挂载 commit hash + 测试结果