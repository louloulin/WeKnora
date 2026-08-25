# Build #11 — Wiki 页面级反向链接面板 验收矩阵

> 本 spec.md 与 Runtime 验收矩阵同构,扁平 A1–A18 编号,每条都是可独立验证的项。
> brief 在同目录 `../brief.md`。

### A1 — 后端端点 `GET /api/v1/knowledgebase/:kbId/wiki/pages/:slug/backlinks` 实现

`internal/handler/wiki_page.go` 中新增 `GetPageBacklinks(c *gin.Context)`,注册到与 `GetPage` 同级的路由组;`go build / vet / gofmt` 退出码 0。

### A2 — 后端响应类型 `[]WikiPageBacklink`

`internal/types/wiki_page.go` 中新增 `WikiPageBacklink` 结构,字段 `{slug, title, page_type, status, updated_at}` 全部存在且 JSON tag 正确。`updated_at` 为 `time.Time` 类型,序列化为 RFC3339。

### A3 — 后端排序与过滤

`internal/application/service/wiki_page.go` 新增 `ListPageBacklinks` 方法实现:
- 结果按 `updated_at` 倒序
- `ListBySlugs` 未返回的孤儿 slug(对应已删除页)被过滤
- `in_links` 中若包含目标页自身 slug(防御性),被排除
- 目标页 `in_links` 为空 → 返回 `[]`,HTTP 200

### A4 — 后端 harness 测试 4 用例

`internal/application/service/wiki_page_backlinks_test.go` 存在并覆盖:
1. 空 `in_links` → 返回 `[]`
2. 多个 in_links,均存在 → 按 `updated_at` 倒序
3. in_links 含已删源页 → 被过滤
4. in_links 含目标自身 slug → 被排除

### A5 — 前端 API 类型与函数导出

`frontend/src/api/wiki/index.ts` 中:
- `export interface WikiPageBacklink { slug, title, page_type, status, updated_at }` 存在
- `export function getWikiPageBacklinks(kbId, slug): Promise<WikiPageBacklink[]>` 存在
- 函数体内调用 `get<...>` 并使用 `encodeSlugPath(slug)`

### A6 — 前端 Pinia store `wikiBacklinks`

`frontend/src/stores/wikiBacklinks.ts` 存在并导出:
- `backlinksFor(kbId, slug): WikiPageBacklink[]`(同步,缓存命中)
- `loadBacklinks(kbId, slug): Promise<WikiPageBacklink[]>`(拉取并写入缓存)
- `invalidate(kbId, slug): void`(清缓存,可选实现)
- 内部按 `(kbId, slug)` 缓存;命中后 stale-while-revalidate(可选)

### A7 — `WikiBacklinksPanel.vue` 在 `WikiBrowser.vue` 挂载

`frontend/src/views/knowledge/wiki/WikiBrowser.vue` 右侧 sidebar 新增 `<WikiBacklinksPanel :kb-id="..." :slug="..." />`,位置在 acl banner 之后;默认折叠(header 显示反向链接 + 计数)。

### A8 — 点击 backlink 调用 `navigateToSlug`

`WikiBacklinksPanel.vue` 的 click handler 调 `useRouter().push` 或直接调 `navigateToSlug(slug)`,与现有 body `[[slug]]` click 处理(`WikiBrowser.vue:2198`)一致;无自定义路由逻辑。

### A9 — 4 locale `wiki.backlinks.*` keys 齐备

zh-CN / en-US / ko-KR / ru-RU 四个 locale 文件均新增:
- `wiki.backlinks.title`(标题)
- `wiki.backlinks.count`(`{n}` 计数模板)
- `wiki.backlinks.empty`(空状态)
- `wiki.backlinks.emptyHint`(空状态 + 教学)
- `wiki.backlinks.loadFailed`(失败,仅内部 log)

### A10 — `npm run check-i18n` 全通过

项目根目录跑 `npm run check-i18n` 返回全绿(11/11 → 16/16 或当前总数)。

### A11 — `npm run build` 成功

`vite build` 退出码 0,产出 `dist/`。

### A12 — `npm test` 全绿

`vitest run` 全部通过;新增用例:
- `frontend/src/stores/wikiBacklinks.test.ts` ≥ 4 用例(空缓存 / 命中缓存 / load 后写入 / 失败保持旧缓存)
- `frontend/src/components/wiki/WikiBacklinksPanel.test.ts` ≥ 5 用例(折叠 / 展开 / 点击跳转 / 空状态 / 数量徽章)

### A13 — `vue-tsc --build` 不引入新错误

`vue-tsc --build` 报错数 ≤ 当前 pre-existing 数(Build #10 后约 10 个)。

### A14 — `scripts/smoke-wiki-backlinks.sh` 存在,dry-run safe

`scripts/smoke-wiki-backlinks.sh` 文件存在并可执行;`bash scripts/smoke-wiki-backlinks.sh --dry-run`(或默认)打印 curl 命令样例不实际发出请求(除非带 `--live` 或设置了对应 env)。

Live 演示步骤:
1. 创建 page A(已知 slug)
2. 创建 page B(content 含 `[[A]]`)
3. `GET /wiki/pages/A/backlinks` → 200 + `[{slug: B, title, ...}]`
4. 删除 page B
5. `GET /wiki/pages/A/backlinks` → 200 + `[]`

### A15 — 纯函数抽到 sibling `.ts`,Node 测试可覆盖

`frontend/src/api/wiki/backlinksHelpers.ts` 存在并导出至少 3 个纯函数:
- `formatBacklinkTitle(b: WikiPageBacklink): string`
- `sortBacklinks(list: WikiPageBacklink[]): WikiPageBacklink[]`(倒序)
- `groupBacklinksByPageType(list: WikiPageBacklink[]): Record<string, WikiPageBacklink[]>`

对应单测覆盖(`backlinksHelpers.test.ts`),导入仅 sibling .ts 文件,Node `tsx --test` 可跑(不触发 axios / vue-i18n 链)。

### A16 — commit 消息符合 repo 风格

commit 标题以 `feat(wiki): Build #11 ...` 起头;正文包含 bullet 列表 + Co-authored-by trailer;与 `dfa8a891`(Build #10)/ `087d73b0`(Build #7 backend)风格一致。

### A17 — working tree 不含 handoff JSONs

`git status` 不显示 `dispatch-*.json`、`verifier-response-*.json`、`handoff-*.json`(这些在 stash 里,不进 commit)。

### A18 — Build #10 契约不破坏

合入 `lumos0826` 后,Build #10 commit `dfa8a891` 引入的 `WikiAclDialog`、`acl.ts`、`wikiPageAcl.ts`、`aclBanner*` 等 export / props / types 不被破坏;`getWikiPageAcl` / `putWikiPageAcl` / `searchWikiAclCandidates` 函数签名 / 入参 / 返回类型不变。