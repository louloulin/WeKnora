# Spec delta · wiki-pages · ADDED Requirements

> 对应 WeKnora 上游 wiki 模块的"能力扩展"规范。原有 wiki_pages / wiki_folders / wiki_page_revisions 表与既有路由不变；本规范只定义 ADDED 能力。

---

## ADDED Requirements

### Requirement: WYSIWYG 富文本编辑器

系统 SHALL 在 wiki 页面编辑场景中提供 Tiptap（MIT 协议）富文本编辑器，与既有 Markdown 编辑器共存并通过 feature flag `FEATURE_WIKI_WYSIWYG`（默认 ON）切换。

#### Scenario: 编辑器加载

- **WHEN**  用户在 `WikiBrowser.vue` 编辑模式下打开任意 wiki 页面
- **THEN**  系统 SHALL 加载 `WikiTiptapEditor.vue`（新增组件）替换 `MarkdownEditor.vue`
- **AND**   工具栏 SHALL 显示粗体、斜体、标题、列表、链接、表格、代码块、行内公式按钮

#### Scenario: 内容持久化（双字段）

- **WHEN**  用户保存页面
- **THEN**  系统 SHALL 同时写入两个字段：`content_html`（前端序列化输出）与 `content_markdown`（既有 Markdown 字段，自动从 HTML 反向生成）
- **AND**   两个字段在 `wiki_pages` 表中以**新增可空列**形式落地（迁移 `migrations/versioned/000090_wiki_content_html.up.sql`），不修改既有列
- **AND**   旧页面 `content_html IS NULL` 时 SHALL 回退渲染既有 `content_markdown`

#### Scenario: 安全清洗

- **WHEN**  系统渲染 `content_html` 字段时
- **THEN**  系统 SHALL 必须 经过 DOMPurify（Apache-2.0）白名单清洗后再注入 Vue 模板
- **AND**   禁止 `<script>`、`<iframe>`、`onerror=` 等危险标签与属性

#### Scenario: 可回滚

- **WHEN**  `FEATURE_WIKI_WYSIWYG=false`
- **THEN**  系统 SHALL 完全等同 v0.7.2 既有 Markdown 编辑体验（`content_html` 字段不被读写）

---

### Requirement: Y.js 实时协同编辑

系统 SHALL 提供基于 Y.js（MPL-2.0，file-level copyleft）+ Hocuspocus（MIT）的页面级实时协同编辑能力。

#### Scenario: WebSocket 连接

- **WHEN**  两个用户同时打开同一 wiki 页面
- **THEN**  系统 SHALL 通过新独立进程 `cmd/yjs-collab/`（Hocuspocus 服务）建立 Y.js CRDT 同步通道
- **AND**   Hocuspocus 鉴权 SHALL 复用既有 WeKnora JWT（`Authorization: Bearer`），拒绝未授权连接

#### Scenario: 离线 + 重连

- **WHEN**  用户编辑过程中断网
- **THEN**  Y.js SHALL 在客户端保留本地状态；网络恢复后自动 merge，不丢失字符
- **AND**   重连时 SHALL 不覆盖他人未冲突的编辑

#### Scenario: 落库一致性

- **WHEN**  Y.js 文档与 v0.7.2 既有 Markdown 字段不一致
- **THEN**  系统 SHALL 仅在用户主动点"保存"时把 Y.js 文档通过 `wiki_replace_text` 链路落库为新版本快照（`wiki_page_revisions`）
- **AND**   实时协同期间不创建新版本（避免快照风暴）

#### Scenario: 部署形态

- `cmd/yjs-collab/` SHALL 作为独立 Go 二进制运行（端口 1234），与主 Go 服务解耦
- **AND**  `docker-compose.yml` SHALL 新增 `yjs-collab` 服务，不修改既有服务依赖图

---

### Requirement: 页面内评论与 @mention

系统 SHALL 支持在 wiki 页面任意段落上添加评论，并支持 `@用户名` 通知。

#### Scenario: 评论存储

- **WHEN**  用户在段落右击 → "添加评论" → 输入文本 → 保存
- **THEN**  系统 SHALL 写入新表 `wiki_page_comments`（迁移 `migrations/versioned/000091_wiki_page_comments.up.sql`）
- **AND**   字段：`id`, `page_id`, `anchor_paragraph_id`, `author_id`, `body`, `mentions JSONB`, `created_at`, `updated_at`, `deleted_at`

#### Scenario: @mention 通知

- **WHEN**  评论正文出现 `@username`
- **THEN**  系统 SHALL 通过既有 `EventBus`（`internal/event/event.go`）发出 `wiki_mention` 事件
- **AND**   既有通知订阅者 SHALL 收到推送（IM / 邮件）

#### Scenario: Agent 工具暴露

- **WHEN**  Agent 调用 `wiki_comment_*` 工具（追加到 `internal/agent/tools/definitions.go` 文件末尾插入块）
- **THEN**  系统 SHALL 提供 `wiki_list_comments`、`wiki_create_comment`、`wiki_resolve_comment` 三个 tool

---

### Requirement: 页面模板

系统 SHALL 支持从预置模板快速创建 wiki 页面。

#### Scenario: 模板存储

- 模板 SHALL 存储在新表 `wiki_templates`（迁移 `000092`），字段：`id`, `tenant_id`, `name`, `description`, `content_markdown`, `content_html`, `icon`, `category`, `created_at`
- **AND**   模板 SHALL 不进入 `wiki_pages` 表（独立生命周期，避免污染上游 schema）

#### Scenario: 系统预置模板

- 上线时 SHALL 预置 3 个模板：空白 / 会议纪要 / 项目周报
- **AND**   用户可在设置里复制、修改、新增自己的模板

---

### Requirement: 页面级 ACL

系统 SHALL 支持在 KB 默认 RBAC 之上为单个页面设置额外 ACL 覆盖。

#### Scenario: 存储选型（关键决定）

- 系统 SHALL 使用**独立表 `wiki_page_acls`**（迁移 `000093`），**不**使用 JSONB 列
- **WHY**  JSONB 与上游未来 schema 演进冲突；独立表可索引、可 JOIN、未来加列不影响上游
- 字段：`id`, `page_id`, `subject_type` ∈ {user, group, role}, `subject_id`, `permission` ∈ {read, comment, write, admin}, `granted_by`, `created_at`

#### Scenario: 权限解析

- **WHEN**  用户访问某 wiki 页面
- **THEN**  系统 SHALL 先检查 KB 级 RBAC；通过后再检查 `wiki_page_acls` 覆盖
- **AND**   ACL 设置 `admin` 时 SHALL 覆盖 KB Viewer/Contributor 默认
- **AND**   未通过 SHALL 返回 403，不返回 404（防枚举）

---

### Requirement: 拖拽目录与页面

系统 SHALL 提供可视化拖拽能力以重新组织 wiki 目录与页面。

#### Scenario: 拖拽落库

- **WHEN**  用户拖动某页面到另一文件夹
- **THEN**  前端 SHALL 调用既有 `POST /api/v1/knowledgebase/:kb_id/wiki/move-page`（上游已有 API）
- **AND**  0 后端改动；仅前端 `WikiBrowser.vue` 与 `WikiFolderActions.vue` 增加 HTML5 drag-and-drop 事件
- **AND**   拖拽期间 SHALL 走 WeKnora 既有乐观锁（`updated_at` 比对）

---

### Requirement: 公开分享链接

系统 SHALL 支持生成只读公开链接（可设密码、过期时间）以分享 wiki 页面。

#### Scenario: 链接生成

- **WHEN**  用户在页面设置里点"生成分享链接"
- **THEN**  系统 SHALL 在新表 `wiki_share_links`（迁移 `000094`）插入：`id`, `page_id`, `token` (32-char), `password_hash`, `expires_at`, `created_by`, `revoked_at`
- **AND**  token SHALL 通过 crypto/rand 生成，URL 形如 `/wiki/share/:token`

#### Scenario: 只读访问

- **WHEN**  未登录用户访问 `/wiki/share/:token`
- **THEN**  系统 SHALL 走新路由 `internal/handler/wiki_share.go`（独立 handler，不修改 wiki_page.go）
- **AND**  页面 SHALL 注入 `<meta name="robots" content="noindex,nofollow">` 防止搜索引擎收录
- **AND**  若设置了密码 SHALL 弹出密码输入（hash 比对，bcrypt cost=10）
- **AND**  若过期 SHALL 返回 410 Gone

---

### Requirement: 版本 diff 视图

系统 SHALL 在版本历史抽屉里提供两个版本之间的行级 diff 视图。

#### Scenario: Diff 渲染

- **WHEN**  用户在 `WikiRevisionDrawer.vue` 里选两个版本
- **THEN**  前端 SHALL 通过既有 `GET /api/v1/knowledgebase/:kb_id/wiki/revisions/:slug?version=N`（上游已有）取两版 Markdown
- **AND**  SHALL 用 `diff-match-patch`（Apache-2.0）渲染为"红色删除线 + 绿色新增"行级 diff
- **AND**  0 后端改动；仅前端扩展

---

## 上游 patch 列表（最小侵入）

| 上游文件 | 改动类型 | 改动量 |
|---|---|---|
| `internal/handler/router.go` | 追加路由（不修改既有） | +5 行 |
| `internal/agent/tools/definitions.go` | 末尾插入块，不修改既有 tool | +20 行 |
| `internal/event/event.go` | 末尾追加事件类型常量 | +1 行 |
| `frontend/src/views/knowledge/wiki/WikiBrowser.vue` | 追加 `<script setup>` 引入 WYSIWYG，模板分两路渲染 | +30 行 |
| `frontend/src/api/wiki/index.ts` | 追加新 API 调用方法 | +40 行 |

**总计**：约 96 行上游文件改动 + 约 18 个新文件。

详细 patch 计划在 `docs/openspec/changes/weknora-wiki-8-enhancements/upstream-patches.md`。

---

## 验证策略

### Build 阶段必须通过

- `go build ./...` 退出码 0
- `npm run build` bundle 增量 < 500 KB（gzipped）
- `migrations/versioned/000090` ~ `000094` 可在 fresh DB 顺序 apply 通过
- `go test ./internal/...` 既有测试 0 失败
- `npm run test` 既有测试 0 失败

### Verify 阶段独立 Verifier subagent 验收

每条 Requirement 跑 happy path 一次；feature flag 关闭后行为必须等同 v0.7.2；`git rebase origin/main` 在干净 workdir 下无冲突。

### 许可证合规

- 新增 `go.mod` 依赖：仅 MIT / Apache-2.0 / BSD
- 新增 `package.json` 依赖：仅 MIT / Apache-2.0 / BSD
- Tiptap = MIT ✅
- Y.js = MPL-2.0（file-level copyleft，仅自身文件需保留 NOTICE，与 WeKnora MIT 不冲突）✅
- Hocuspocus = MIT ✅
- DOMPurify = Apache-2.0 ✅
- diff-match-patch = Apache-2.0 ✅