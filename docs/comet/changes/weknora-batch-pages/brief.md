# Build #12 — wiki 页面批量操作

## 背景

Build #2a 已经搭好单点 `MovePage`(KB-owner/Admin+,KB 本地,`{slug, folder_id}` body),
但 KB 管理员面对几百到几千页的 wiki 时一个个手动操作不可行;`status` 切换、删除同
样如此。本 Build 在不引入新写入面的前提下,把现有"逐页"操作打包成"批量"调用,让
管理员能一次完成 N 页同类操作。

Build #12 不动页面内容、不引入新权限面、不改 schema。它是纯调用层聚合。

## 范围

3 个批量端点 + 1 个前端"批量工具栏":

| 端点 | 方法 | 作用 |
|---|---|---|
| `/wiki/pages/batch-move` | POST | 把 N 页一次性移到同一 folder |
| `/wiki/pages/batch-delete` | POST | 软删除 N 页(级联清理 `in_links` + chunk 同步) |
| `/wiki/pages/batch-status` | POST | 改 N 页的 `status`(draft/published/archived) |

## 决定(D1–D8)

### D1 — 部分成功而非全或全无

返回结构 `{succeeded: string[], failed: {slug, error}[]}`。一行失败不阻塞其他行。这
与 Build #10 ACL 校验、`BatchDeleteKnowledge` 已有 /knowledge/batch-delete 行为一致
(参 `routes_knowledge.go:130`);管理员一次提交 50 页,5 页因为 ACL 卡掉,剩 45 页
应该照样进。

### D2 — 同一 KB 内

`kb_id` 从 URL path 拿,所有 page 必须属于该 KB(否则 400 `kb_mismatch`)。不做跨
KB,因为 `FolderID` / `KB.AccessPolicy` 都不是跨域的;跨域批量会引入新矩阵。

### D3 — 守卫复用

批量端点全部挂 `OwnedWikiKBOrAdmin` + `KBAccessWrite("kb_id")`,与 `MovePage` 单
点同矩阵(参 `routes_knowledge.go:304`)。**不**降级到 Contributor,因为批量破坏性
操作必须有 KB-owner 守门(与 Build #10 ACL 守门同档)。

### D4 — 批量上限

`max_batch_size = 100`(服务端硬上限;超 100 → 400 `too_many`)。理由:保护事务锁、
保护 Redis 缓存、防止一个 10000 页请求把 DB 拖死。100 是与 `BatchDeleteKnowledge`
单批对齐的合理档位(参 `internal/handler/knowledge.go:BatchDeleteKnowledge`)。

### D5 — 单 SQL 事务,bookkeeping-only 写

后端三个 batch 方法共用一个事务助手 `runBatchOnTx(ctx, fn)`,内部开 GORM 事务;
每行失败记 `failed`,不回滚整批。`batch-move` 和 `batch-status` 是 bookkeeping-only
(`UpdateMeta`),不 bump version;`batch-delete` 走 `Delete`(已 soft-delete,不
hard-delete),并级联调用 `removeInLinks` 清理其它页的 `in_links` 字段。

### D6 — 输入限长与去重

请求体里 `slugs: string[]`,`len(unique) ≤ 100`,空字符串直接被剔除;重复 slug
去重后处理。响应里 `succeeded` 也只返回实际成功的 slug(去重)。

### D7 — 响应 payload 与 HTTP 码

- 全部成功 → `200` + `{succeeded, failed: []}`
- 全部失败(如 100 行都 404) → `200` + `{succeeded: [], failed: [...]}`(不 4xx,
  因为请求格式本身合法,只是"结果没成功")
- 请求格式非法(`slugs` 空 / 超 100 / 含非法字符)→ `400`
- ACL/守卫失败 → `403`
- KB 不存在 → `404`

理由:批量场景下"请求合法但结果部分失败"是常态,不应让客户端解析 5xx;`200` + 内嵌
`failed` 与 GitHub / Stripe 批量 API 风格一致。

### D8 — 前端选择模式 + 工具栏

不在每一行常态渲染 checkbox(太嘈杂);引入一个"select mode" toggle,放在现有
`wiki-reader-actions` toolbar 里(`WikiBrowser.vue:517` 附近):

- toggle off: 默认行为,单击 = 打开页面
- toggle on: 每一行左侧出现 `t-checkbox`,单击 = 多选,**不**打开页面;顶部浮出
  一个 `WikiBulkActionBar`,显示"已选 N 项" + 三个按钮(移动 / 改状态 / 删除)

选择状态用组件内 `ref` 持有,不进 Pinia(避免跨页面泄漏);**切页 / 切 folder / 退出
select mode 时清空**,防止"我刚选了一堆东西,跳个页就被后台操作"的惊吓。

## 范围之外(显式 not-in-scope)

- **跨 KB 批量**:不在本 Build 范围。
- **批量更新 `content` / `title`**:是另一种语义,留待 Build #13 单独评估。
- **批量加 tag**:同上,留待后续。
- **Undo 撤销栈**:批量删除没 undo,显式由用户确认弹窗兜底(参 D7)。
- **后台异步队列**:本 Build 是同步请求,前端直接等响应;长任务走队列是
  Build #14+ 的话题。

## 验收口径

详见 `specs/wiki-page-batch-ops/spec.md`(A1–A14)。