# Build #16 spec — wiki 批量操作预览 / Dry-run

## 目标

实现 wiki 批量操作(move / delete / status)的**预览**流程:用户先看到一份只读结果(哪些 slug 会成功、哪些会因为 `not_found` / `folder_conflict` / `kb_mismatch` 等原因失败、整体摘要),确认无误后再走真实 batch 端点。

无 DB 改动,纯校验 / 读取。

## 范围(包含)

### 后端

1. **3 个新 endpoint**(Build #16 — 与现有 `batch-move` / `batch-delete` / `batch-status` 对称):
   - `POST /api/v1/knowledgebase/{kb_id}/wiki/pages/batch-preview-move`(body 复用 `WikiPageBatchMoveRequest`)
   - `POST /api/v1/knowledgebase/{kb_id}/wiki/pages/batch-preview-delete`(body 复用 `WikiPageBatchDeleteRequest`)
   - `POST /api/v1/knowledgebase/{kb_id}/wiki/pages/batch-preview-status`(body 复用 `WikiPageBatchStatusRequest`)

2. **响应类型 `WikiBatchPreviewResponse`**(新增于 `internal/types/wiki_page.go`):
   ```go
   type WikiBatchPreviewResponse struct {
     Success []string                 `json:"success"`        // 预计会成功的 slug
     Failed  []WikiPageBatchFailure   `json:"failed"`         // 预计会失败的 slug + code + error
     Summary WikiBatchPreviewSummary   `json:"summary"`        // 整体计数
   }

   type WikiBatchPreviewSummary struct {
     Total      int `json:"total"`
     WillSucceed int `json:"will_succeed"`
     WillFail   int `json:"will_fail"`
   }
   ```

3. **service 新方法**(均在 `wikiPageService` 上,因为校验逻辑需要 `s.repo` + `applyFolderToPage`):
   - `PreviewBatchMove(ctx, kbID, slugs, folderID) (*WikiBatchPreviewResponse, error)`
   - `PreviewBatchDelete(ctx, kbID, slugs) (*WikiBatchPreviewResponse, error)`
   - `PreviewBatchStatus(ctx, kbID, slugs, status) (*WikiBatchPreviewResponse, error)`
   - 全部加入 `WikiPageService` 接口(新建条目,不再走 `WikiBatchJobService`)

4. **handler 新方法** `BatchPreviewMove` / `BatchPreviewDelete` / `BatchPreviewStatus`(`internal/handler/wiki_page.go`),对齐现有 batch-* 写法:
   - 复用 `validateBatchSlugs`
   - 复用 `respondBatchServiceError`(对 `kb_mismatch` → 400)
   - 复用 `writeBatchRouteResult` 不可用(返回类型不同),改用 `c.JSON(200, resp)`

5. **route 注册** `internal/router/routes_knowledge.go`,与 `batch-move` 等同一条 RBAC 链(`OwnedWikiKBOrAdmin` + `KBAccessWrite`)。预览是只读,但用 write 守卫以保证与"写"路径权限一致(读权限低不能"试"写)。

### 前端

1. **API client 新增** `frontend/src/api/wiki/batchPreview.ts`(或合并到 `frontend/src/api/wiki/index.ts` 的 batch 块):
   - `batchPreviewMove(kbId, payload)` / `batchPreviewDelete(kbId, payload)` / `batchPreviewStatus(kbId, payload)`,返回 `WikiBatchPreviewResponse`。

2. **类型** `WikiBatchPreviewResponse` + `WikiBatchPreviewSummary`,导出至 `frontend/src/api/wiki/batchTypes.ts`。

3. **`WikiBatchPreviewDialog.vue`** 新组件:
   - Props: `visible` / `previewType` / `previewData` / `loading`
   - Emit: `update:visible` / `confirm`(用户点"确认执行")
   - 顶部 summary 行:`5 条会成功 / 3 条会失败`(从 `previewData.summary` 渲染)
   - 主体 per-slug 表格:slug + 状态 icon(✓ / ✗)+ code badge(failed 时)+ error message
   - 底部 "取消" / "确认执行" 两按钮;确认 → 关闭 dialog,emit confirm(由父组件触发原 batch-* POST)

4. **`WikiBulkActionBar.vue` 改造**:
   - 当 `selectedSlugs.length >= WikiBatchAsyncThreshold`(20)时,显示"预览"按钮(在"确认"前)
   - 点击"预览" → 调对应 `batchPreview*` API → 打开 `WikiBatchPreviewDialog`
   - dialog "确认执行" → 关闭 dialog,走原 `batch-*` POST 路径(sync / async 同 Build #13)
   - 当 `< 20` 时,继续走原直 POST 路径(预览不值这一步)

5. **i18n 8 keys × 4 locale**:
   - `wiki.batchPreview.title`(对话框标题)
   - `wiki.batchPreview.summary`(顶部摘要)
   - `wiki.batchPreview.willSucceed`(成功行标签)
   - `wiki.batchPreview.willFail`(失败行标签)
   - `wiki.batchPreview.unknownCode`(未知错误码兜底)
   - `wiki.batchPreview.confirm`(确认执行按钮)
   - `wiki.batchPreview.cancel`(取消按钮)
   - `wiki.batchPreview.empty`(没有结果时的占位)

## 关键决定(D1–D8)

- **D1 三端点 vs 一端点** — 三端点,跟随现有 verb 拆分;Swagger 更清晰。
- **D2 纯只读校验,不调 `MovePage` / `DeletePage` / `UpdatePageMeta`** — 调真实方法会出现 Tx 外副作用(`DeletePage` 调 `removeInLinks` + `deleteChunkForPage` + `DeleteRevisionsByPage`,`MovePage` 调 `repo.UpdateMeta`),且 `wikiBatchJobService` 没有 GORM handle,Tx-rollback 无法干净实施。改用纯只读路径:`s.repo.GetBySlug` + `applyFolderToPage` + `IsValidWikiPageStatus` + `s.repo.GetFolderByID`,所有调用都是 read,无副作用。错误分类复用 `classifyBatchError`,与真实调用 1:1 对齐。
- **D3 同步返回** — 不挂异步队列;预览用户正盯着 UI,等几百毫秒就够。
- **D4 仅对 ≥ `WikiBatchAsyncThreshold`(20)slugs 显示预览按钮** — 小批量跳过预览,减少点击;大批量才值得。后端接口始终可用。
- **D5 不写 audit / failures** — 预览是只读,不入审计流,不写 `wiki_batch_job_failures`。
- **D6 dialog 是 modal 不能关闭后误触发** — 用户必须主动点"确认执行"才会 POST,避免误操作。
- **D7 Preview UI 触发条件: A**(≥ 20 slugs 才显示预览按钮)— 用户已确认。
- **D8 复用 `WikiPageBatchFailure`** — 已有 `{Slug, Code, Error}` 字段,直接复用,前端不用新 schema。

## D2 设计细节 — PreviewBatchXxx 实现

### 公共 helper:`buildPreviewResponse`

```go
func (s *wikiPageService) buildPreviewResponse(
  ctx context.Context,
  kbID string,
  slugs []string,
  perSlug func(ctx context.Context, slug string) error,
) (*types.WikiBatchPreviewResponse, error)
```

- 在 service 内提供,因为 `applyFolderToPage` 是 unexported
- 输入 `clean := normalizeBatchSlugs(slugs)`
- `assertBatchKBOwnership(ctx, kbID, clean)`(失败 → 返回 kb_mismatch 错误,handler 映射 400)
- 对每个 slug 调 `perSlug`,append to Success/Failed,最后 `classifyBatchError(err)`
- 返回 `{Success, Failed, Summary}`

### PreviewBatchMove

```go
func (s *wikiPageService) PreviewBatchMove(ctx, kbID, slugs, folderID) (*WikiBatchPreviewResponse, error) {
  return s.buildPreviewResponse(ctx, kbID, slugs, func(ctx, slug) error {
    page, err := s.repo.GetBySlug(ctx, kbID, slug)
    if err != nil { return err }  // not_found
    // check ACL? skip — same as MovePage does NOT gate, BatchMovePages
    // doesn't gate either. (Operator scope only.)
    probe := *page
    probe.FolderID = strings.TrimSpace(folderID)
    return s.applyFolderToPage(ctx, &probe)  // folder_not_found / ok
  })
}
```

### PreviewBatchDelete

```go
func (s *wikiPageService) PreviewBatchDelete(ctx, kbID, slugs) (*WikiBatchPreviewResponse, error) {
  return s.buildPreviewResponse(ctx, kbID, slugs, func(ctx, slug) error {
    _, err := s.repo.GetBySlug(ctx, kbID, slug)
    return err  // not_found
  })
}
```

### PreviewBatchStatus

```go
func (s *wikiPageService) PreviewBatchStatus(ctx, kbID, slugs, status) (*WikiBatchPreviewResponse, error) {
  if !IsValidWikiPageStatus(status) {
    return nil, fmt.Errorf("invalid status %q: must be draft, published or archived", status)
  }
  return s.buildPreviewResponse(ctx, kbID, slugs, func(ctx, slug) error {
    page, err := s.repo.GetBySlug(ctx, kbID, slug)
    if err != nil { return err }
    if page.Status == status {
      return nil  // already at target — count as success
    }
    return nil  // status diff doesn't block success
  })
}
```

## 范围之外(not-in-scope)

- 跨 KB 预览过滤(同 batch-* 行为,KB 不匹配返回 400,不进 dry-run)
- 实时 preview(预览是 click-triggered,不订阅)
- "dry-run 模式"开关(预览就是独立 endpoint,无需开关)
- 把 preview 结果存表(预览不持久化)
- preview 触发 audit 事件
- ACL 检查:预览与 `BatchMovePages` / `BatchDeletePages` / `BatchUpdatePageStatus` 行为对齐,**不**调 `gateWikiPageAccess`(批量操作的 owner 守卫已足够;ACL 是页面级读取权限,与批量管理语义不冲突)

## 验收矩阵

### 后端(7 项)

- **A1** 类型 `WikiBatchPreviewResponse` / `WikiBatchPreviewSummary` 在 `internal/types/wiki_page.go` 定义,导出。
- **A2** `WikiPageService` 接口新增 `PreviewBatchMove` / `PreviewBatchDelete` / `PreviewBatchStatus` 三个方法,放在 `internal/types/interfaces/wiki_page.go`。
- **A3** `wikiPageService` 实现三个方法(纯只读路径),复用 `applyFolderToPage` + `IsValidWikiPageStatus` + `normalizeBatchSlugs` + `assertBatchKBOwnership` + `classifyBatchError`,代码中显式注释"无副作用"。
- **A4** handler `BatchPreviewMove` / `BatchPreviewDelete` / `BatchPreviewStatus` 三个方法,复用 `validateBatchSlugs` + `respondBatchServiceError`;Swagger 注释完整。
- **A5** route 注册 3 个 `POST /batch-preview-*`,RBAC 链与 `batch-*` 一致。
- **A6** harness test:`wiki_batch_preview_test.go` 覆盖 — (a) move:目标 folder 不存在 → folder_not_found; (b) delete:slug 不存在 → not_found; (c) status:非法 status → 400(in service 层 return err); (d) status:slug 已为 target status → 算 success; (e) move:全部 slug 成功 → summary will_succeed=total; (f) KB 越权 → ErrWikiBatchKBMismatch。
- **A7** 静态验证:`cd backend && go build ./...` 通过,`go vet ./internal/...` 通过。

### 前端(5 项)

- **A8** API client 新增 `batchPreviewMove` / `batchPreviewDelete` / `batchPreviewStatus`(`frontend/src/api/wiki/batchTypes.ts` + `frontend/src/api/wiki/index.ts`)。
- **A9** 新组件 `frontend/src/components/wiki/WikiBatchPreviewDialog.vue`:`TDialog`(TDesign)或自实现 modal;顶部 summary、主体表格(per-slug 行:slug + ✓/✗ + code badge + error)、底部"取消 / 确认执行"两按钮;emit `update:visible` + `confirm`。
- **A10** `WikiBulkActionBar.vue` 改造:`selectedSlugs.length >= WikiBatchAsyncThreshold` 时显示"预览"按钮(在"确认"前);点击预览 → 调 API → 打开 dialog;dialog 确认 → 走原 `batch-*` POST。
- **A11** i18n 8 keys × 4 locale(`zh-CN` / `en-US` / `ko-KR` / `ru-RU`)。
- **A12** 静态验证:`vue-tsc` + `npm run build` + `scripts/check-i18n-complete.sh` 全绿。

### 通用(3 项)

- **A13** smoke script `scripts/smoke-wiki-batch-preview.sh`(dry-run safe)覆盖 3 个 preview endpoint,展示请求/响应格式。
- **A14** 本地 verify:build + harness test + smoke script dry-run 全部绿;非绿项贴日志。
- **A15** 提交 commit 到 `lumos0826-batch-dryrun` 分支,合并到 `lumos0826`,推送,回复 LUM-20。

合计 15 项。