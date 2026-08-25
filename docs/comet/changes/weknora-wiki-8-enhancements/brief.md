# Brief · weknora-wiki-8-enhancements

> Comet-Native change · branch `lumos0826` · base `main` · 上游 `Tencent/WeKnora` v0.7.2
> 配套 issue：LUM-20（louloulin）

---

## Outcome

把 WeKnora v0.7.2 自带的 Wiki 模块升级为 **可商用的协同文档平台**，补齐 WYSIWYG 编辑、实时协同、评论、模板、页面级 ACL、拖拽、公开分享、版本 diff 共 8 项能力。**全部以最小侵入方式落地**：新增文件 + 增量迁移为主；对上游既有文件做最少的"插入点"修改；保持所有下游 PR 可干净 rebase 上游 main。

完成后，外贸企业 / 中大型团队可以直接基于 WeKnora 私有部署，替代飞书 / Notion 做协同 + AI 知识管理。

---

## Scope

| # | 能力 | 实现位置 | 工时估算 |
|---|---|---|---|
| 1 | WYSIWYG 富文本编辑器（Tiptap 替换 Markdown 文本框） | `frontend/src/views/knowledge/wiki/WikiTiptapEditor.vue`（新）+ `internal/handler/wiki_page.go` 内容字段扩展 | 6 周 |
| 2 | Y.js CRDT 实时协同编辑 | 新建 `internal/handler/wiki_collab.go` + 前端 `y-websocket` 客户端 + 复用现有 `wiki_replace_text` 落库链路 | 3 周 |
| 3 | 页面内评论 + `@mention` | 新增 `wiki_page_comments` 表 + 新增 `wiki_comment_*` tool + 前端评论抽屉 | 2 周 |
| 4 | 页面模板 | 新增 `wiki_templates` 表 + 「从模板新建」UI 入口 | 1 周 |
| 5 | 页面级 ACL | `wiki_pages` 加 `acl JSONB` 列 + handler 层覆盖判断 | 2 周 |
| 6 | 拖拽目录/页面 | 前端 `WikiFolderActions.vue` + `WikiBrowser.vue` 拖拽事件，复用现有 `/move-page` API | 0.7 周 |
| 7 | 公开分享链接 | 新增 `wiki_share_links` 表 + 只读路由 `/wiki/share/:token` + 水印 | 2 周 |
| 8 | 历史 diff 视图 | 前端 `WikiRevisionDrawer.vue` + `diff-match-patch` 渲染 + 现有 `?version=N` 后端 | 1 周 |

**全部合计**：约 17 周（约 4 个月 × 2 名全栈工程师）。

---

## Non-goals

明确**不做**的事，避免 scope creep：

1. **不替换 wiki 既有核心机制**（Slug、6 种页面类型、Map-Reduce 生成管道、Wiki Fixer、版本快照策略）—— 这些是上游自有特色，全部复用。
2. **不引入新前端框架**（不引入 React、不引入 Next.js、不引入 Quasar）—— 复用 WeKnora 现有 Vue 3 + TDesign 技术栈。
3. **不做多租户联邦 / 跨 KB 协同**—— 留在单 KB 范围内。
4. **不做 Notion-style AI inline write**（cursor-level AI）—— 只补齐基础 8 项，AI 增强放在后续 change。
5. **不修改 WeKnora 现有 RAG / Agent / MCP 链路**—— wiki 是独立的视图层，不影响检索。
6. **不动 docreader / MCP server / CLI 等周边仓库**—— 改动仅限 `internal/` Go 代码、`frontend/` Vue 代码、`migrations/`、新增 `docs/openspec/`。

---

## Acceptance examples

每条验收项具体、可观察、可独立验证：

| ID | 验收标准 |
|---|---|
| **AC-1** | 用户在 WikiBrowser 编辑页面时，工具栏含粗体 / 斜体 / 标题 / 列表 / 链接 / 表格 / 代码块按钮；保存后内容字段 `content_html` 非空；Markdown 字段同步更新；旧页面纯 Markdown 仍可正常显示 |
| **AC-2** | 浏览器 A 和 B 同时打开同一页面，A 输入"abc"在 200ms 内出现在 B 屏幕；A 离线编辑后回到在线，光标位置正确、无内容丢失 |
| **AC-3** | 用户选中段落右击 → "评论" → 输入文本 → 保存；其他协作者看到段落右侧气泡；评论支持 @某用户 → 该用户收到通知 |
| **AC-4** | WikiBrowser 工具栏出现"从模板新建"按钮；至少预置 3 个模板（空白 / 会议纪要 / 项目周报）；新建页面默认套用模板内容 |
| **AC-5** | KB Owner 可在页面设置里添加"仅 X 组可见"；非该组成员访问该页面 → 403；ACL 设置保留 KB 默认 RBAC 不被覆盖 |
| **AC-6** | 用户拖动某页面到另一文件夹 → 松手 → 列表立即更新，刷新后位置保留；后端 `/move-page` 调用 200 |
| **AC-7** | 用户点击"分享" → 生成链接 `https://.../wiki/share/abc123` → 未登录用户访问可看到内容（带水印 + noindex meta）；token 可设过期时间；过期后访问 410 |
| **AC-8** | 在版本历史抽屉里选两个版本 → 看到 diff 视图（红色删除线 / 绿色新增）；行级 diff 准确率 > 95%（用一段已知变更文本验证） |

**总体验收**：8 项全部 `passed`，`go build ./...` 成功，`npm run build` 成功，所有原有 wiki 接口回归测试通过。

---

## Constraints and invariants

### 必须遵守

1. **MIT 协议兼容**：仅引入 MIT / Apache-2.0 / BSD 协议依赖；**禁用 AGPL / SSPL / 商业协议**。前端组件优先 Tiptap（MIT）、Y.js（MPL-2.0）、diff-match-patch（Apache-2.0）。
2. **最小侵入**：
   - **不重命名**任何上游公共函数 / 公共类型 / 公共接口
   - **不修改**任何上游既有文件的核心逻辑（仅允许在 hooks / extension points 插入新代码）
   - **新增文件**数量优先于"修改既有文件"
   - **迁移**：每项能力独立 migration `migrations/versioned/0000XX_wiki_*.up.sql`，按 WeKnora 现有风格（pg + sqlite 双份）
3. **可 rebase**：
   - 所有 patch 集中在 `frontend/src/views/knowledge/wiki/` 与新增 `internal/application/service/wiki_*.go` / `internal/handler/wiki_*.go`
   - 已有 `wiki_page.go` / `wiki_ingest.go` / `tools/wiki_*.go` 尽量**不改**，必须改时记录在 `openspec/changes/weknora-wiki-8-enhancements/upstream-patches.md`
4. **可回滚**：每个能力独立 feature flag（`FEATURE_WIKI_WYSIWYG` 等），关闭后行为等同于 v0.7.2 原生
5. **不动数据**：新增列必须 nullable 或带默认值；不改既有列类型
6. **可观测**：每条新路由必须接入 Langfuse trace span

### 上游 hook 点（已在源码中确认）

- `WikiBrowser.vue` 的编辑入口 → 新增 `WikiTiptapEditor.vue`
- `internal/handler/wiki_page.go` 的 `POST /pages` / `PUT /pages/:slug` → 加 `content_html` 字段（向后兼容）
- `internal/agent/tools/definitions.go` 的 tool 注册 → 追加 `wiki_comment_*` / `wiki_share_*` tool
- `migrations/versioned/` 已有 0000XX wiki 迁移 → 续编 000090+

---

## Decisions

来自前 5 轮收敛：

| 编号 | 决策 | 出处 |
|---|---|---|
| D-1 | WeKnora 是 MIT，**fork + 私有化 + 加外贸 skill 自由商业化**无需授权 | LUM-20 第 5 轮 |
| D-2 | 不引入 Docmost / AFFiNE / Mattermost / Cal.com / n8n / 自建妙记 | LUM-20 第 4 轮 |
| D-3 | 不引入 Docmost，**8 项能力补到 WeKnora 内部 wiki** | LUM-20 第 4 轮（后续） |
| D-4 | 12-18 个月 roadmap 第一阶段（M1-M4）就是这 8 项 | LUM-20 第 6 轮 |
| D-5 | **2 组件架构**：WeKnora + Teable（Teable 由 LUM-18 同步推进） | LUM-20 第 6 轮 |
| D-6 | 目标分支 = `main`，change 分支 = `lumos0826` | LUM-20 第 7 轮（用户指定） |
| D-7 | 走 Comet-Native 工作流（不选 Classic） | LUM-20 第 7 轮（用户指定） |

---

## Open questions

需要用户（louloulin）在 confirm-shape 前决定：

| # | 问题 | 候选 | 默认 |
|---|---|---|---|
| Q-1 | WYSIWYG 选 **Tiptap**（MIT，ProseMirror 内核）还是 **ByteMD**（MIT，字节团队，类 Notion 体验）？ | Tiptap / ByteMD | **Tiptap**（生态更成熟、Y.js 集成官方支持） |
| Q-2 | Y.js 协同需要常驻 **y-websocket** 服务（独立 Node 进程），加到 docker-compose 还是嵌入 Go（用 gorilla websocket）？ | 独立 Node / Go 嵌入 | **Go 嵌入**（少一个组件） |
| Q-3 | 公开分享链接是否需要**密码保护**？ | 要 / 不要 | **要**（按 Notion / 飞书习惯） |
| Q-4 | 8 项能力是否**全部做**还是**只做前 4 项**（1+2+3+4 是高频刚需）？ | 全部 8 项 / 仅 1+2+3+4 | **全部 8 项**（4 个月可完成） |
| Q-5 | 是否把 8 项能力**上游化**回 `Tencent/WeKnora`（贡献 PR）？ | 上游化 / 仅内部使用 | **仅内部使用**（保留差异化壁垒） |
| Q-6 | 提交粒度：8 个独立 PR（推荐），还是 1 个大 PR？ | 8 个 PR / 1 个 PR | **8 个独立 PR**（每项可独立回滚、独立 review） |

如果用户不指定，按默认推进。

---

## Verification expectations

### Build 阶段验收

- `go build ./...` 退出码 0，无 warning
- `npm run build` 成功，bundle 大小增量 < 500 KB（gzipped）
- 所有 174 个现有 migration 在新 migration 前成功 apply
- `go test ./internal/...` 既有测试 0 失败
- `npm run test` 既有测试 0 失败

### Verify 阶段验收

由独立 Verifier subagent 验收：

1. **8 项能力端到端**：每个 AC 跑一遍 happy path
2. **回归**：原有 wiki 5 个核心场景（创建 / 编辑 / 版本 / 回滚 / 删除）不破
3. **可 rebase**：`git rebase origin/main` 在干净 workdir 下无冲突
4. **MIT 兼容**：所有新增依赖 `go.mod` / `package.json` 的 license 为 MIT / Apache-2.0 / BSD
5. **可回滚**：每个 feature flag 关闭后行为完全等同 v0.7.2

### 不验收

- 不验收上游 Tencent/WeKnora v0.7.2 → v0.7.3 的兼容性（后续 PR 再说）
- 不验收 SaaS 多租户联邦 / Notion AI inline write
- 不验收外贸领域知识包（独立 change）