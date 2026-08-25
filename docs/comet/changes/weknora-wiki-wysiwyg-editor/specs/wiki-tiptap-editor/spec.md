# wiki-tiptap-editor

## Purpose

Build #2b 在 Build #2a 铺好的存储层之上引入 wiki 页面 WYSIWYG 编辑能力：
- 后端新增 `GET /api/v1/features` 运行时 feature flag 接口
- 前端引入 Tiptap 编辑器组件，按 flag 动态加载
- 保存时双写 markdown 与 sanitized HTML，复用 Build #2a 的 `content_html` 落库路径
- XSS 把关集中在前端 DOMPurify 出口
- 既有 markdown 渲染路径完全不变，flag 关闭的 KB 行为与 Build #2a 一致

## ADDED Requirements

### Requirement: 运行时 feature flag

后端 SHALL 暴露 `GET /api/v1/features`，按 `WEKNORA_FEATURE_WIKI_WYSIWYG` 环境变量
返回当前生效的 flag map。前端 SHALL 在启动后异步加载该 flag，并基于 flag 决定
是否渲染 Tiptap 编辑器。

#### Scenario: flag 开启

- **WHEN** 进程启动时 `WEKNORA_FEATURE_WIKI_WYSIWYG=true`
- **THEN** `GET /api/v1/features` 返回 `{"code":0,"msg":"success","data":{"flags":{"wiki_wysiwyg":true}}}`，HTTP 200
- **AND** 前端 `useFeatureFlagsStore().flags.wiki_wysiwyg === true`

#### Scenario: flag 关闭 / 未设置

- **WHEN** 进程启动时 `WEKNORA_FEATURE_WIKI_WYSIWYG` 未设置、为空、为 `false`/`0`/`no`
- **THEN** 同接口返回 `{"data":{"flags":{"wiki_wysiwyg":false}}}`，HTTP 200
- **AND** 前端渲染 `<t-textarea>`，行为与 Build #2a 完全相同

#### Scenario: 未鉴权

- **WHEN** 未携带有效 JWT / API key 的请求访问 `GET /api/v1/features`
- **THEN** 返回 HTTP 401，响应体不含任何 flag 数据（继承 `/api/v1` 分组的 auth middleware）

#### Scenario: 接口不可达 / 5xx

- **WHEN** 前端 `ensureLoaded()` 收到非 2xx 响应或网络错误
- **THEN** store `flags.wiki_wysiwyg` 默认 `false`，`loaded` 仍为 `true`（不再重试，避免雪崩）
- **AND** 编辑器降级到 `<t-textarea>`，不抛错、不阻塞页面加载

### Requirement: Tiptap 编辑器组件

`<WikiTiptapEditor>` SHALL 提供 WYSIWYG 编辑能力，最小工具集包含 bold、italic、
heading(1-3)、ordered list、bullet list、code、code-block、link；`v-model` 暴露
`{ html, markdown }` 双字段。

#### Scenario: 工具栏按钮

- **WHEN** 编辑器挂载
- **THEN** 工具栏 SHALL 暴露 8 个按钮：bold / italic / heading(1-3) / ordered list / bullet list / code / code-block / link
- **AND** SHALL NOT 暴露 image / table / embed / mention / task-list / horizontal-rule / blockquote / 数学公式

#### Scenario: v-model 双字段输出

- **WHEN** 用户在编辑器内输入或修改内容
- **THEN** `v-model` 的 `html` 字段 SHALL 等于 `editor.getHTML()` 经 DOMPurify 过滤后的结果
- **AND** `markdown` 字段 SHALL 等于 `tiptap-markdown` 扩展的 `editor.storage.markdown.getMarkdown()` 结果
- **AND** 当 `html` 为空（用户清空全部内容）时，`markdown` SHALL 同样为空字符串

#### Scenario: DOMPurify sanitize

- **WHEN** `editor.getHTML()` 产出含 `<script>` / `onerror` 等 XSS payload 的字符串
- **THEN** sanitize 出口 SHALL 移除所有 script 标签与事件属性
- **AND** sanitize 函数 SHALL 复用 `frontend/src/utils/security.ts` 的 `sanitizeHTML` 既有白名单（a, p, h1-h3, ul, ol, li, blockquote, code, pre, strong, em, br, hr, span）

#### Scenario: sanitize 失败容错

- **WHEN** DOMPurify 调用抛异常（极少见，例如 DOMPurify 版本冲突）
- **THEN** `html` SHALL 返回空字符串
- **AND** 前端 SHALL 在 `console.warn` 记录一次
- **AND** 保存流程 SHALL 继续（markdown 路径不受影响）

#### Scenario: 动态加载

- **WHEN** `useFeatureFlagsStore().flags.wiki_wysiwyg === false`
- **THEN** `<WikiTiptapEditor>` SHALL NOT 被加载到 bundle
- **AND** WikiBrowser SHALL 渲染 `<t-textarea>`，行为与 Build #2a 相同

### Requirement: WikiBrowser 接线

`WikiBrowser.vue` SHALL 在 wiki 页面编辑 UI（line 573 附近 `<t-textarea>`）按 flag
切换渲染 `<WikiTiptapEditor>` 或 `<t-textarea>`；保存时按模式写入对应字段。

#### Scenario: WYSIWYG 模式保存

- **WHEN** `flags.wiki_wysiwyg === true` 且用户在编辑器内编辑
- **THEN** `savePageEdit` SHALL 把 `{ title, summary, content: markdown, content_html: html, version }` 一起 PUT
- **AND** `content_html` 在 PUT body 中 SHALL 非空
- **AND** 后端 SHALL 把 `content_html` 写入 `wiki_pages.content_html`（沿用 Build #2a 落库路径）

#### Scenario: 标记模式保存

- **WHEN** `flags.wiki_wysiwyg === false` 或未加载完成
- **THEN** `savePageEdit` SHALL 保持既有 PUT body `{ title, summary, content, version }`（无 `content_html`）
- **AND** 后端 `content_html` 列 SHALL 保持原值（`ContentHTML *string` 指针 nil）

#### Scenario: 渲染优先级

- **WHEN** `GET .../wiki/pages/:slug` 返回的页面 `content_html` 非空且 `content` 非空
- **THEN** WikiBrowser SHALL 优先用 `content_html` 渲染（绑 innerHTML，过 DOMPurify）
- **AND** SHALL NOT 走 `safeMarkdownToHTML` 二次转换

#### Scenario: legacy 行渲染

- **WHEN** 页面 `content_html` 为 NULL / 空（Build #2a 之前的行）
- **THEN** WikiBrowser SHALL 走既有 `safeMarkdownToHTML` 路径，行为与 Build #2a 完成时一致
- **AND** SHALL NOT 抛错或显示空白

### Requirement: 类型与 API 契约

前端 SHALL 扩展 `WikiPageUpdatePayload` 增加 `content_html?: string` 字段；
后端 SHALL 新增 `internal/types/features.go` 暴露 `FeaturesResponse` 与
`FeaturesFlags` 类型。

#### Scenario: WikiPageUpdatePayload

- **WHEN** Build #2b 完成
- **THEN** `frontend/src/api/wiki/index.ts` 的 `WikiPageUpdatePayload` SHALL 含 `content_html?: string` 字段
- **AND** 既有字段 SHALL 不变

#### Scenario: 后端 FeaturesResponse

- **WHEN** Build #2b 完成
- **THEN** `internal/types/features.go` SHALL 暴露：
  - `FeaturesFlags { WikiWysiwyg bool \`json:"wiki_wysiwyg"\` }`
  - `FeaturesResponse { Flags FeaturesFlags \`json:"flags"\` }`
- **AND** handler 响应 envelope 沿用 `{"code":0,"msg":"success","data":<FeaturesResponse>}` 风格

## 上游 patch 列表（最小侵入）

| 上游文件 | 改动类型 | 改动量 |
|---|---|---|
| `internal/config/config.go` | 增加 `WEKNORA_FEATURE_WIKI_WYSIWYG` env 解析 | +10 行（含注释） |
| `internal/handler/features.go` | 新文件：`GetFeatures` handler | +30 行 |
| `internal/types/features.go` | 新文件：`FeaturesResponse` / `FeaturesFlags` | +15 行 |
| `internal/router/routes_knowledge.go` | 注册 `/features` GET 路由 | +5 行 |
| `frontend/package.json` | 新增 4 个 npm 依赖 | +5 行 |
| `frontend/src/api/features/index.ts` | 新文件：`getFeatures()` | +15 行 |
| `frontend/src/stores/featureFlags.ts` | 新文件：`useFeatureFlagsStore` | +50 行 |
| `frontend/src/utils/sanitize/wysiwyg.ts` | 新文件：DOMPurify 包装 | +20 行 |
| `frontend/src/components/wiki/WikiTiptapEditor.vue` | 新文件：Tiptap 编辑器组件 | +200 行 |
| `frontend/src/views/knowledge/wiki/WikiBrowser.vue` | 修改编辑 UI 区块（line 573 附近）+ savePageEdit 路径 | +20/-5 行 |
| `frontend/src/api/wiki/index.ts` | `WikiPageUpdatePayload` 增加 `content_html` | +1 行 |

**总计**：5 个新后端/前端文件 + 6 个上游文件修改 +~370 行（含 Tiptap 组件主体）；
新增 4 个 npm 依赖，**0 新 Go 依赖**、**0 新迁移**。

## Verification

- `go build ./...` 退出码 0
- `go test ./internal/handler/... ./internal/router/... ./internal/config/...
  ./internal/types/...` 既有测试 0 失败
- `cd frontend && npm run build` 退出码 0
- 手工：`WEKNORA_FEATURE_WIKI_WYSIWYG=true` 启动，浏览器 DevTools 调
  `GET /api/v1/features` 返回 `wiki_wysiwyg=true`；进入 KB wiki 编辑页，看到 Tiptap
  工具栏 8 个按钮；输入内容点保存；`GET .../wiki/pages/:slug` 返回的 JSON 含非空
  `content_html` 字段
- 手工 fallback：`WEKNORA_FEATURE_WIKI_WYSIWYG=false` 启动，进入同样 KB，看到既有
  `<t-textarea>`，行为与 Build #2a 完成后完全相同
- Verifier 跑：
  - `grep -rn "WEKNORA_FEATURE_WIKI_WYSIWYG" internal/config/config.go` 命中 1+（env 解析）
  - `grep -rn "wiki_wysiwyg" internal/handler/features.go internal/types/features.go` 命中各 1+
  - `grep -rn "GetFeatures" internal/router/routes_knowledge.go` 命中 1（路由注册）
  - `grep -rn "useFeatureFlagsStore\|getFeatures" frontend/src/` 命中
  - `git diff 1064e39e..HEAD -- migrations/` 为空（Build #2b 的有效 diff 基准是 Build #2a 归档点 `1064e39e`；`fc9d6f8c` 早于 Build #2a 的 `000090_wiki_content_html` 迁移，单独跑会误报）
  - `git diff c92052f0^1..HEAD -- go.mod go.sum` 为空（Build #2b 自己的 diff 基准是后端提交 `c92052f0` 的父提交）
  - `frontend/package.json` 新增 `@tiptap/vue-3` / `@tiptap/starter-kit` / `@tiptap/pm` / `tiptap-markdown`