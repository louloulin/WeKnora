# Build #12 spec — wiki 页面批量操作

## 验收矩阵

### 后端 (Go)

**A1. `POST /wiki/pages/batch-move` 接受 `{slugs, folder_id}`,正常路径全部成功**
输入:3 个 slug 全在 kb1,folder_id = valid_folder。期望:`{succeeded: [3], failed: []}`,
HTTP 200,3 个 page 的 `folder_id` + `category_path` 已更新。

**A2. `POST /wiki/pages/batch-move` 部分失败**
输入:3 个 slug,其中一个不存在,folder_id 有效。期望:`{succeeded: [2], failed: [{slug, error}]}`,
HTTP 200,真实存在的两页被移走,不存在的那页不进 `succeeded`。

**A3. `POST /wiki/pages/batch-delete` 级联清理 in_links**
输入:A、B、C 三页,其中 B、C 都 `[[A]]`,批量删 A。期望:响应 succeeded 含 A;
再次 `GET /wiki/pages/B` 显示 `in_links` 不再含 A(级联 `removeInLinks`),
同样对 C 生效。HTTP 200。

**A4. `POST /wiki/pages/batch-status` 改 N 页 status**
输入:3 页 published → archived。期望:3 页 `status` 字段全为 archived,version 不变
(bookkeeping-only),HTTP 200。

**A5. 三端点 batch size 上限**
输入:`slugs.length = 101`。期望:HTTP 400 `too_many`(不调用下游)。

**A6. 三端点跨 KB 检测**
输入:某 slug 不属于 path 里的 `kb_id`。期望:HTTP 400 `kb_mismatch`,不污染数据。

**A7. 守卫矩阵**
非 KB-owner 的 Contributor 调任意批量端点 → HTTP 403(沿用 `OwnedWikiKBOrAdmin`)。

**A8. 输入去重**
输入:`slugs = ["a","a","b"]`(a 重复)。期望:响应 succeeded/failed 各自只出现
一次 a,数据库只更新一次(行级幂等)。

**A9. 空 slugs**
输入:`slugs = []`。期望:HTTP 400 `empty_slugs`,不调用下游。

**A10. 单元测试覆盖**
`wiki_page_batch_test.go`(harness,stub repo):
- 全成功 / 部分失败 / 跨 KB / 去重 / 顺序稳定
每类 ≥ 1 用例。

### 前端 (Vue3 / TS)

**A11. Select-mode toggle 不破坏默认行为**
toggle off 时,点击行 = `selectPage`(打开页面),所有现有交互(单击、双击、
dragstart)不变。toggle on 时,点击行 = toggle 选中状态,不打开页面。

**A12. WikiBulkActionBar 显示选中数 + 三按钮**
toggle on 后,顶部出现 toolbar,显示"已选 N 项";N=0 时三个按钮 disabled。

**A13. 批量 move 走 FolderPickerDialog(复用已有)**
点 "移动" → 弹 folder picker → 选 folder → 调 `batchMoveWikiPages(kbId, slugs, folderId)`
→ 成功后 toast,失败 toast 显示部分失败数(响应里 `failed.length`)。

**A14. 批量 delete 走确认弹窗**
点 "删除" → 弹确认 modal(类似 Build #5 comment delete)→ 用户确认 → 调
`batchDeleteWikiPages` → 成功后刷新列表 + toast。

### I18n / 错误降级

**A15. 4 locale × 6 keys**
`bulkSelect / bulkBar / bulkMove / bulkStatus / bulkDelete / bulkConfirm`
在 zh-CN / en-US / ko-KR / ru-RU 各键存在且编译通过。

**A16. 失败响应兜底**
`failed.length > 0` 时,toast 同时显示成功数 + 失败数,不只显示其中一个。

## 范围外(显式 not covered)

- 跨 KB 批量
- 批量编辑 content / title / aliases
- 批量加 tag
- Undo / redo
- 后台异步任务队列(批量删 1000 页走 SSE 进度 — 是 Build #14+ 的事)

## 与既有矩阵的关系

- 复用 `MovePage` 的 service 核心循环(`applyFolderToPage` + `UpdateMeta`),
  batch-move 不重复实现,而是循环调用。理由:保持 `applyFolderToPage` 的单一实现,
  未来加 invariant (cache invalidate) 时不会漏批量路径。
- 复用 `DeletePage` 的级联清理(`removeInLinks` + chunk 同步),同理。
- 复用 `UpdatePageMeta` 处理 status 切换(不 bump version)。