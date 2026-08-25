# weknora-wiki-backlinks — Build #11 简介(页面级反向链接面板)

## 目标

把 `WikiPage.in_links`(slug 数组)从后端字段升级为前端可视化面板,**让读者能直接看到哪些其它页面引用了当前页面**,并能一键跳转。

Wiki 内容的"发现"靠两种动作:主动搜索(全文 / 文件夹浏览),被动推荐(反向链接 / 引用)。前者已有 Build #9-A 全文搜索;**本 Build 把后者补齐**。

实现完成的标准:
- 后端 `go build / vet / gofmt / migrate` 全绿,新增端点 `GET /wiki/pages/:slug/backlinks` 返回 200 + 解析后的引用页列表
- 前端 `vue-tsc / vite build / check-i18n / vitest` 全绿,WikiBrowser 右侧新增可折叠"反向链接"section
- 端到端 smoke:在 KB 内创建 A → 创建 B(A 中含 `[[B]]`) → 打开 B 页面,反向链接面板显示 A

## 背景

- `WikiPage.out_links` 与 `in_links` 自 v1 起就是 `StringArray`(`internal/types/wiki_page.go:232-234`),由 `wikiPageService.parseOutLinks` 从 markdown 抽取(`internal/application/service/wiki_page.go:1085`),`updateInLinks` / `removeInLinks` 在 create / update / delete 时自动维护(`wiki_page.go:1260-1293`)
- 前端 `WikiBrowser.vue:1650 renderMarkdown` 已把 `[[slug]]` / `[[slug|display]]` 改写成 `<a data-slug="...">`,`handleContentClick` + `navigateToSlug` 已能跳转(`WikiBrowser.vue:3833,2198`)
- 链路图(Graph view)已经使用 `out_links` 字段构造边,但**单页视角下的反向链接面板尚未存在**

### 关键约束
- 现状是数据已经 100% 维护,但前端零可视化
- `WikiPage.in_links` 是 slug 数组,**不携带 title / page_type** —— 直接展示给用户不可读,需要后端做 slug → lite 投影解析
- 已删除的源页面可能仍留在 `in_links`(后端 remove 路径会在 delete 时清理,但历史数据 / 数据修复路径可能残留)
- Build #7 backend 之后,某些源页面可能因 ACL(私有 / allow_list)对当前用户不可读,理想情况应过滤;**本 Build 暂不过滤**(详见 D4)

## 范围

### 1. 后端 — 新端点 `GET /api/v1/knowledgebase/:kbId/wiki/pages/:slug/backlinks`

#### 1.1 Service 层(`internal/application/service/wiki_page.go`)

新增方法:
```go
func (s *wikiPageService) ListPageBacklinks(
    ctx context.Context, kbID, slug string,
) ([]*types.WikiPageBacklink, error)
```

实现要点:
- 调 `repo.GetBySlug(ctx, kbID, slug)` 取目标页 → `in_links`(若页不存在返回 404 等价错误)
- 调 `repo.ListBySlugs(ctx, kbID, inLinks)` 取 lite 投影(`WikiPageLite` 已存在,见 `internal/types/interfaces/wiki_page.go:125`)
- 过滤掉 `ListBySlugs` 没返回的 slug(对应页面已删除)→ 不污染前端面板
- 排除 `in_links` 中的自身 slug(理论上不会发生,但防御性过滤)
- 按 `updated_at` 倒序(最近编辑者优先),返回结构 `[{slug, title, page_type, status, updated_at}]`

#### 1.2 类型(`internal/types/wiki_page.go`)

新增:
```go
type WikiPageBacklink struct {
    Slug     string    `json:"slug"`
    Title    string    `json:"title"`
    PageType string    `json:"page_type"`
    Status   string    `json:"status"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

注:`WikiPageLite` 是私有 lite 投影,不要直接暴露为 API 响应类型;新建一个轻量公开类型更干净。

#### 1.3 Handler(`internal/handler/wiki_page.go`)

新增 `GetPageBacklinks(c *gin.Context)`,与 `GetPage` 同级:
- swagger annotation: `@Router /knowledgebase/{kb_id}/wiki/pages/{slug}/backlinks [get]`
- 调用 `wikiService.ListPageBacklinks(ctx, kbID, slug)`
- 404 路径:page not found → 404;`in_links` 为空 → 200 + 空数组(不是错误)

#### 1.4 路由注册(`internal/router` / `cmd/server/main.go`)

按 Build #7 backend 的路由注册模式新增一行,与现有 `GET /wiki/pages/:slug` 同级。

### 2. 前端 — API 客户端(`frontend/src/api/wiki/index.ts`)

新增:
```ts
export interface WikiPageBacklink {
  slug: string;
  title: string;
  page_type: string;
  status: string;
  updated_at: string;
}

export function getWikiPageBacklinks(kbId: string, slug: string) {
  return get<WikiPageBacklink[]>(`.../wiki/pages/${encodeSlugPath(slug)}/backlinks`)
}
```

### 3. 前端 — Pinia store(`frontend/src/stores/wikiBacklinks.ts`)

新增:
```ts
// 按 (kbId, slug) 缓存,避免每次切换页面都打 N 次请求
// 命中时立即返回,后台异步刷新(stale-while-revalidate)
export const useWikiBacklinksStore = defineStore('wikiBacklinks', () => { ... })
```

提供:
- `backlinksFor(kbId, slug): WikiPageBacklink[]` — 同步返回缓存值
- `loadBacklinks(kbId, slug): Promise<...>` — 拉取并写入缓存
- `invalidate(kbId, slug)` — 内容编辑成功后调用(可选)

### 4. 前端 — UI 组件(`frontend/src/components/wiki/WikiBacklinksPanel.vue`)

放置位置:**WikiBrowser 右侧 sidebar**(目前该 sidebar 容纳 toc / issues / acl banner,放在 acl banner 之后,作为内容区下方的独立 section)。

布局:
```
[toc / issues / acl banner — 现有内容]

─────────────────────────
▼ 反向链接 (3)          <- 折叠 header,可点击展开/收起
─────────────────────────
├─ Page A     summary   2026-08-20
├─ Page C     entity    2026-08-15
└─ Page D     synthesis 2026-08-10
─────────────────────────
[空状态时]
无反向链接。在其它页面使用 [[<slug>]] 即可创建。
─────────────────────────
```

行为:
- 默认**折叠**(避免视觉噪音)
- 点击某条 → 调用现有 `navigateToSlug(slug)` 切换页面
- 数据 stale-while-revalidate:页面切换到新 slug 时,先展示缓存(若有),再后台拉新数据
- 拉取失败 → 静默降级为显示缓存或隐藏整个 section,不弹 toast(网络抖动不应打扰阅读)
- 数量徽章 `(N)` 显示在 header;`N=0` 仍显示 section,但提示文案变成"无反向链接"

### 5. i18n(4 locale:`zh-CN / en-US / ko-KR / ru-RU`)

新增 keys(放 `wiki.backlinks.*` 命名空间):
- `wiki.backlinks.title`:反向链接 / Backlinks / 역링크 / Обратные ссылки
- `wiki.backlinks.count`:`{n} 条` / `{n} backlinks` / `{n}개` / `{n} ссылок`
- `wiki.backlinks.empty`:无反向链接。/ No backlinks yet. / 역링크 없음. / Обратных ссылок нет.
- `wiki.backlinks.emptyHint`:在其它页面使用 `[[<slug>]]` 即可创建反向链接。/ ... (per-locale wording)
- `wiki.backlinks.loadFailed`:反向链接加载失败 / Backlinks failed to load / ... (仅 silent log,不展示给用户)

### 6. 单元测试

- `frontend/src/stores/wikiBacklinks.test.ts`(≥ 4 用例):空缓存 / 命中缓存 / load 后写入 / 拉取失败保持旧缓存
- `frontend/src/components/wiki/WikiBacklinksPanel.test.ts`(≥ 5 用例):折叠 / 展开 / 点击跳转 / 空状态 / 数量徽章
- `frontend/src/api/wiki/backlinksHelpers.ts`(纯函数提取):`formatBacklinkTitle`、`sortBacklinks`、`groupBacklinksByPageType`
- 后端 Go 测试:`internal/application/service/wiki_page_backlinks_test.go`(harness,与 Build #7 backend `wiki_acl_test.go` 模式一致):空 in_links / 多 in_links / 源页已删除 / 源页自身 slug 出现在 in_links

### 7. 端到端 smoke 脚本(`scripts/smoke-wiki-backlinks.sh`)

- dry-run + curl 模板
- 演示步骤:创建 A → 创建 B(内容含 `[[A]]`) → GET `/wiki/pages/A/backlinks` 应返回 `[{slug:B, ...}]`
- 同时演示:删除 B → 再次 GET 应返回 `[]`(因 in_links 自动清理)

## 决策(已采纳,无 blocking)

| ID | 决策 | 选项 | 理由 |
|----|------|------|------|
| D1 | 链接语法 | A: 沿用 `[[slug]]` + `[[slug\|display]]`,**不引入新语法** | 后端 `parseOutLinks` + 前端 `renderMarkdown` 已实现;`WikiPage.out_links`/`in_links` 已 100% 维护。Build #11 是把数据可视化,不是造新东西 |
| D2 | 范围 | A: KB-local(不做 cross-KB) | `in_links` 是 KB-scoped 字段;cross-KB 需要 tenant 级索引 + 跨域权限审查,与"页面级反向链接"的用户意图不符 |
| D3 | 后端实现 | A: 新端点 `GET /wiki/pages/:slug/backlinks`,用现有 `ListBySlugs` 解析 | 避免前端 N+1 拉每个 slug;返回结构稳定、排序由后端控制 |
| D4 | ACL 过滤 | C: 暂不过滤(已读 + 私有 / allow_list 源页都显示在面板) | Build #7 backend 的 ACL 决策是逐页 gate,ListBySlugs 不感知 ACL。完整 ACL-aware 过滤需要新 `ListReadableBySlugs` repo 方法,scope 偏大;**推到 #11.5 follow-up** |
| D5 | 面板位置 | A: 右侧 sidebar 折叠 section,放在 ACL banner 之后 | 现有右侧已有 toc / issues / acl banner;同区域新增折叠项视觉一致 |
| D6 | 点击行为 | A: 调用 `navigateToSlug(slug)` | 与 body 中 `[[slug]]` 点击行为完全一致,降低认知成本 |
| D7 | 空状态 | B: 显示 hint + 字面 `[[<slug>]]` 教学 | 帮助用户自我教育如何创建反向链接,降低首次使用门槛 |
| D8 | 失败处理 | A: silent degrade(显示缓存或隐藏 section,不 toast) | 阅读是主流程,网络抖动不应打扰;与 Build #9-A 全文搜索的失败处理一致 |
| D9 | Stale-while-revalidate | A: 命中后立即返回缓存 + 后台异步刷新 | 页面切换是高频操作,延迟应 < 50ms |

## 非目标

- 编辑时实时更新:源页保存时不需要实时推送 in_links 变化到目标页 — 用户下次打开目标页即看到最新
- 反向链接的"转出"(out_links 可视化)— `WikiBrowser` 现有 outline / toc 已承担一部分,**本 Build 不重复**
- 跨 KB 反向链接(D2 决策)
- ACL-aware 过滤(D4 决策,推迟到 #11.5)
- 删除源页时的"反向链接孤儿"通知 — 当前 `removeInLinks` 已自动清理 in_links,**不需要额外 UI**
- 后端全文搜索索引同步 in_links(已经是数据,无需索引) — `out_links` 在 Build #2b 时代已可被 graph endpoint 使用
- 把 in_links 做成单独接口调用 `wikiGraph` — 已有 graph endpoint(`GetGraph`)覆盖 overview 场景,**单页 backlinks 不重做**

## 沙箱限制

- 沙箱无 Go toolchain / Postgres:后端 `go build / vet / gofmt / migrate` 走用户本地
- 沙箱无浏览器:`WikiBacklinksPanel.vue` 的真实 DOM 测试由 `vitest` 组件测覆盖
- 沙箱无后端实例:smoke 脚本为 dry-run + curl 命令模板(同 Build #10)

## 验收

- A1: 后端端点 `GET /api/v1/knowledgebase/:kbId/wiki/pages/:slug/backlinks` 实现并通过本地 `go build / vet / gofmt`
- A2: 后端响应类型 `[]WikiPageBacklink` 含 `{slug, title, page_type, status, updated_at}`
- A3: 后端按 `updated_at` 倒序;`in_links` 中的孤儿 slug(对应已删除页)被过滤;目标页自身 slug(若出现)被排除
- A4: 后端 harness 测试覆盖空 / 多 / 已删源页 / 自身 slug 4 个用例
- A5: 前端 `frontend/src/api/wiki/index.ts` 导出 `WikiPageBacklink` 类型与 `getWikiPageBacklinks` 函数
- A6: 前端 `frontend/src/stores/wikiBacklinks.ts` 实现,提供 `backlinksFor` / `loadBacklinks` / `invalidate`
- A7: 前端 `frontend/src/components/wiki/WikiBacklinksPanel.vue` 在 `WikiBrowser.vue` 右侧 sidebar 挂载,默认折叠
- A8: 点击 backlink 调用 `navigateToSlug`,与 `[[slug]]` body click 一致
- A9: 4 locale `wiki.backlinks.*` keys 齐备(zh-CN / en-US / ko-KR / ru-RU)
- A10: `npm run check-i18n` 全通过(11/11 → 16/16 或当前 spec 数)
- A11: `npm run build` 成功(`vite build`)
- A12: `npm test` 全绿;新增 store 4+ / panel 5+ 用例全过
- A13: `vue-tsc --build` 不引入新错误(已有 ~10 个 pre-existing 不计)
- A14: `scripts/smoke-wiki-backlinks.sh` 存在,dry-run safe;live 模式演示创建 A → B 含 `[[A]]` → GET backlinks
- A15: 纯函数(`formatBacklinkTitle` / `sortBacklinks` / `groupBacklinksByPageType`)抽到 sibling `.ts` 文件,Node `tsx --test` 可直接覆盖(避免 axios / vue-i18n 副作用)
- A16: commit 消息符合 repo 风格(`feat(wiki): Build #11 ...`)
- A17: working tree 不含 handoff / dispatch / verifier-response JSONs
- A18: Build #10 (ACL E2E) commit `dfa8a891` 的 export / props / types 不被破坏

## 关联文件

### 新增
- `internal/types/wiki_page.go` — 新增 `WikiPageBacklink` 结构(修改)
- `internal/application/service/wiki_page.go` — 新增 `ListPageBacklinks` 方法(修改)
- `internal/application/service/wiki_page_backlinks_test.go` — harness 测试
- `internal/handler/wiki_page.go` — 新增 `GetPageBacklinks` handler(修改)
- 路由注册文件 — 新增一行(具体路径以现有注册模式为准)
- `frontend/src/api/wiki/index.ts` — 新增类型 + API 函数(修改)
- `frontend/src/api/wiki/backlinksHelpers.ts` — 纯函数 + 单元测试
- `frontend/src/stores/wikiBacklinks.ts` — Pinia store
- `frontend/src/stores/wikiBacklinks.test.ts`
- `frontend/src/components/wiki/WikiBacklinksPanel.vue`
- `frontend/src/components/wiki/WikiBacklinksPanel.test.ts`
- `frontend/src/views/knowledge/wiki/WikiBrowser.vue` — 挂载 panel(修改,< 30 行)
- `scripts/smoke-wiki-backlinks.sh`
- 4 locale 文件 `frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}/knowledgeEditor.ts` — 新增 keys(修改)
- `docs/comet/changes/weknora-wiki-backlinks/{brief.md, specs/wiki-page-backlinks/spec.md}`(本文件)

### 修改(沿用 Build #10 既有路径)
- 复用 `WikiBrowser.vue` 现有 `navigateToSlug` / `handleContentClick` / `selectedPage`
- 复用 `request.ts` 的 `get<T>` HTTP 层
- 复用 `wikiPageService` 的 `repo.ListBySlugs` 与 `repo.GetBySlug`

## Runtime blocker(已记录,不影响产物)

同 Build #10:`comet native new` 仍被 stale Runtime index 拦截(`weknora-wiki-fulltext-search` / `weknora-wiki-wysiwyg-editor`)。

**绕行方式**:沿用 Build #7 backend + Build #10 的"直分支 + 产物落盘"路径:
- 在分支 `lumos0826-wiki-backlinks` 上做工作
- `brief.md` / `spec.md` 写到 `docs/comet/changes/weknora-wiki-backlinks/`
- 完成时手动 `git merge --ff-only` 合回 `lumos0826`
- 不走 `comet native archive` 走 workspace finish(因为没有 Runtime 注册)

## 当前态

- 分支 `lumos0826`,HEAD `dfa8a891`(Build #10 ACL E2E)
- 工作树 clean
- 待 Shape 确认 → 进入 Build