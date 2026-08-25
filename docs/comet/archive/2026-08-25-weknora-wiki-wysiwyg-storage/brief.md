# Build #2a: Wiki 页面 content_html 存储层

## Outcome

让 WeKnora wiki 页面支持「同一份内容并存 Markdown 与 HTML 两种表示」：数据库多一列
`wiki_pages.content_html TEXT NULL`，后端 PUT 接受 `content_html` 字段并落库，GET
返回该字段。

这是 Build #2（WYSIWYG 编辑器）的子集：先把存储层和落库路径铺好，前端编辑器
（Tiptap + DOMPurify）由后续 Build #2b 完成。Build #2a 跑完后，所有写入仍走既有
MarkdownEditor，前端不会被任何变化影响；上线的 KB 也不会有行为差异。

## Scope

- 新迁移 `migrations/versioned/000090_wiki_content_html.up.sql`（与 .down.sql）：
  - `ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS content_html TEXT;`
- `internal/types/wiki_page.go`：
  - `WikiPage` 结构体新增字段 `ContentHTML string` (JSON `content_html`, gorm
    `column:content_html;type:text`)
  - `WikiPageUpdateRequest` 新增 `ContentHTML *string` (`json:"content_html,omitempty"`)
- `internal/handler/wiki_page.go` 的 update handler：
  - 若 `req.ContentHTML != nil`，把它赋值到 `page.ContentHTML` 后再调
    `h.wikiService.UpdatePage(...)`
- `internal/types/wiki_page.go` 的 `WikiPage` JSON tag 自动把 `ContentHTML` 暴露给
  GET 响应（既有 `Content` 字段同理）

## Non-goals

- 不引入前端 Tiptap 编辑器（Build #2b）
- 不引入 DOMPurify 渲染路径（Build #2b）
- 不引入 `FEATURE_WIKI_WYSIWYG` feature flag（Build #2b）
- 不修改 `WikiPage` 既有字段（`content` 仍是 markdown 主源）
- 不修改 WikiPageRevision 既有结构（content_html 不进 revision 表，避免快照膨胀；
  revision 只存 markdown 全文，HTML 在 PUT 时由前端覆盖当前行的 column）
- 不修改 `wikiService.UpdatePage` 内部实现（既有 GORM 自动映射会处理新列）
- 不引入 Go 侧 HTML sanitizer（信任已鉴权用户的输入；前端 DOMPurify 在 Build #2b
  渲染路径上把关）
- 不修改 `wiki_pages` 既有索引或约束
- 不动 frontend/src/，不动 yjs-collab/，不动 router.go（路由不变）

## Acceptance

- AC-1：迁移 `000090_wiki_content_html.up.sql` 在 fresh DB 上 `psql -f` 成功，新列
  `content_html TEXT` 出现；`.down.sql` 执行后该列消失
- AC-2：`PUT /api/v1/knowledgebase/:kb_id/wiki/pages/:slug` body 含 `content_html`
  时，后端把值写入 `wiki_pages.content_html`；不含时该列保持原值
- AC-3：`GET .../wiki/pages/:slug` 返回的 JSON 含 `content_html` 字段；旧页面
  （content_html IS NULL）返回空字符串或 null（保持一致即可）
- AC-4：content_html 字段在乐观锁 `req.Version != existing.Version` 触发时同样
  受保护（沿用既有分支）
- AC-5：0 前端代码变更；`frontend/src/` 不在本次 commit 里
- AC-6：`go build ./...` 退出码 0；`go test ./internal/handler/... ./internal/types/...`
  既有测试 0 失败
- AC-7：迁移的 up/down 严格对称：up 加列、down 删列；不做数据迁移
- AC-8：commit 列表只含 1 个新迁移 + `internal/types/wiki_page.go` + `internal/handler/wiki_page.go`
  共 3 个文件变更；不引新依赖

## Constraints

- 0 新 Go 模块依赖（HTML sanitize 留前端；Go 侧只增字段）
- 0 新 npm 依赖
- 0 数据库表新增（ALTER COLUMN 而非 CREATE TABLE）
- `WikiPage` JSON tag `content_html` 与 SQL 列名 `content_html` 完全一致
- 不在 GORM AutoMigrate 里硬编码 schema 演进（保持既有手工迁移约定）

## Decisions

- **D-1**：`content_html` 选 `TEXT` 而非 `JSONB`：HTML 是序列化字符串，存 TEXT 与
  `content`（markdown）同型；JSONB 会强迫前端在保存前做一次 stringify，无收益
- **D-2**：不进 `WikiPageRevision`：revision 是 LLM 写回与人工编辑的版本回滚通道，
  HTML 只是 markdown 的另一种投影；存两份会让快照表翻倍且回滚路径复杂
- **D-3**：不校验 `content_html` 内容：信任已鉴权用户；真正的 XSS 把关在 Build #2b
  前端 DOMPurify 渲染路径上
- **D-4**：handler 沿用 `*string` 指针约定（与既有 `Content *string` 一致）：nil = 不
  改，非 nil = 覆盖。Markdown→HTML 的衍生不归 handler 做，由 Build #2b 前端编辑器
  主动产出
- **D-5**：不引入 `expected_updated_at` 乐观锁扩展：Build #1 风险条目已记入；
  Build #2a 不动既有乐观锁协议

## Open questions

- Q-1：是否在 Build #2a 同时新增 `wiki_page_revisions.content_html` 列供 diff？
  默认：否。理由：revision HTML 是快照的镜像，没必要；回滚基于 markdown 文本即可。
  若用户反对则拆分 Build #2c 处理。
- Q-2：`WikiPage` 的 JSON 输出里 `content_html` 是 `omitempty` 还是总输出？
  默认：与 `Content` 保持一致——`omitempty`（空字符串/null 都不输出，减少 legacy
  页面 payload 体积）。需在 spec 中明确。

## Verification

- `go build ./...`：退出 0
- `go test ./internal/handler/... ./internal/types/... ./internal/application/service/...`：
  既有测试 0 失败（变更范围小，新字段不影响既有逻辑）
- 手工：在 fresh DB 上跑 `psql -f migrations/versioned/000090_wiki_content_html.up.sql`
  再 `\d wiki_pages` 看到新列；再 `psql -f migrations/versioned/000090_wiki_content_html.down.sql`
  列消失
- Verifier 跑：grep `content_html` 在 internal/types/ + internal/handler/ + migrations/
  各出现至少 1 次；migrations/versioned/000090_* 存在且 up 加列 / down 删列