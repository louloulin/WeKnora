# wiki-pages-html-storage

## Purpose

Build #2a 引入 wiki 页面 HTML 存储层：`wiki_pages.content_html TEXT NULL`。这是
Build #2（WYSIWYG 编辑器）的存储前置，不含前端编辑器。本 spec 只定义 ADDED 能力，
原有 markdown 编辑路径保持不变。

## ADDED Requirements

### Requirement: content_html 落库

系统 SHALL 在 wiki 页面表新增可空 TEXT 列 `content_html`，PUT 接口 SHALL 接受并落
库该字段，GET 接口 SHALL 暴露该字段；旧页面 `content_html IS NULL` 时 SHALL 保持
既有 markdown 渲染路径不变。

#### Scenario: 迁移加列

- **WHEN** fresh DB 执行 `migrations/versioned/000090_wiki_content_html.up.sql`
- **THEN** `wiki_pages` 增加 `content_html TEXT` 列，所有既有行该列值为 NULL
- **AND** 既有列、索引、约束不变
- **AND** `.down.sql` 执行后 `content_html` 列被移除

#### Scenario: PUT 接受 content_html

- **WHEN** 用户调用 `PUT /api/v1/knowledgebase/:kb_id/wiki/pages/:slug`，body 含
  `{"content_html": "<p>...</p>"}` 字段
- **THEN** 后端 SHALL 把该字段值写入 `wiki_pages.content_html`
- **AND** body 不含 `content_html` 字段时 SHALL 保持数据库原值（沿用既有
  `*string` 指针语义）

#### Scenario: GET 暴露 content_html

- **WHEN** 任意调用 `GET .../wiki/pages/:slug` 或 `GET .../wiki/pages`
- **THEN** 返回的 JSON SHALL 含 `content_html` 字段
- **AND** legacy 行 `content_html IS NULL` 时 SHALL 返回空字符串（与既有
  `omitempty` 约定一致）

#### Scenario: 乐观锁受保护

- **WHEN** `req.Version > 0` 且与 `existing.Version` 不一致
- **THEN** 后端 SHALL 拒绝更新并返回 409，content_html 与其他字段一样 SHALL 不被
  写入

#### Scenario: 0 前端变更

- **WHEN** Build #2a 完成
- **THEN** `frontend/src/` SHALL 无任何文件修改
- **AND** 前端 SHALL 仍使用既有 MarkdownEditor 写入 `content`（markdown）
- **AND** `wiki_pages.content_html` SHALL 保持 NULL 直至 Build #2b 上线

#### Scenario: 0 新依赖

- **WHEN** Build #2a 完成
- **THEN** `go.mod` 与 `frontend/package.json` SHALL 均无新增依赖
- **AND** 不引 Go 侧 HTML sanitizer（XSS 把关留 Build #2b 前端 DOMPurify）

### Requirement: Go 类型与请求体

系统 SHALL 在 `internal/types/wiki_page.go` 暴露 `WikiPage.ContentHTML string`
与 `WikiPageUpdateRequest.ContentHTML *string` 两个字段，gorm tag SHALL 与 SQL 列
名严格一致。

#### Scenario: WikiPage 字段

- **WHEN** Build #2a 完成
- **THEN** `WikiPage` 含 `ContentHTML string \`json:"content_html,omitempty" gorm:"column:content_html;type:text"\``
- **AND** GORM 自动映射到 `wiki_pages.content_html`

#### Scenario: UpdateRequest 字段

- **WHEN** Build #2a 完成
- **THEN** `WikiPageUpdateRequest` 含 `ContentHTML *string \`json:"content_html,omitempty"\``
- **AND** handler 在 `req.ContentHTML != nil` 时 SHALL 把 `*req.ContentHTML` 赋值到
  `page.ContentHTML`

## 上游 patch 列表（最小侵入）

| 上游文件 | 改动类型 | 改动量 |
|---|---|---|
| `migrations/versioned/000090_wiki_content_html.up.sql` | 新文件 | +N 行（1 条 ALTER TABLE） |
| `migrations/versioned/000090_wiki_content_html.down.sql` | 新文件 | +1 行（1 条 DROP COLUMN） |
| `internal/types/wiki_page.go` | 追加 `ContentHTML` 字段（不修改既有） | +2 行 |
| `internal/handler/wiki_page.go` | 追加 `req.ContentHTML` 分支（不修改既有） | +3 行 |

**总计**：2 个新迁移文件 + 2 个上游文件 +~6 行改动。

## Verification

- `go build ./...` 退出码 0
- `go test ./internal/handler/... ./internal/types/... ./internal/application/service/...` 既有测试 0 失败
- `migrations/versioned/000090_wiki_content_html.up.sql` 与 `.down.sql` 文件存在，up 仅 ALTER TABLE ADD COLUMN，down 仅 ALTER TABLE DROP COLUMN（grep 验证）
- `grep -n "ContentHTML" internal/types/wiki_page.go` 命中 ≥2（结构体 + Request）
- `grep -n "ContentHTML" internal/handler/wiki_page.go` 命中 ≥2（指针 nil 判断 + 赋值）
- `git diff main...lumos0826 -- frontend/` 为空（0 前端变更）