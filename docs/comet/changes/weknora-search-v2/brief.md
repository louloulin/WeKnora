# weknora-search-v2 — Build #19 简介(全局搜索 v2:服务端 tsvector + ACL + 跨 KB)

## 目标

把 `GET /api/v1/knowledgebase/:kb_id/wiki/search` 从"regex `~*` + 返回 `WikiPage[]` 全量 + 旁路 page-ACL + 单 KB"升级为"PostgreSQL tsvector `to_tsquery` + `ts_headline` 高亮 + `ts_rank` 排序 + page-ACL join + 跨 KB 可选 + 轻量 `{slug,title,snippet,score,kb_id,page_type}` payload"。

Wiki 内容发现的两条主路径:**全文搜索**(本 Build)+ **跨页面 backlink**(Build #11,下一 Build #20 升级)。Build #9-A 已经把前端工具栏 `WikiSearchBar` + `WikiSearchResults` 装好,但后端实现停留在 MVP。本 Build 把后端补到位 + 把前端 payload 切换过来。

实现完成的标准:
- 后端 `go build` / `go vet` / harness 单测全绿,新端点 `GET /wiki/search?v=2` 返回 `{slug,title,snippet,score,kb_id,page_type}[]`,带 tsvector 命中片段
- 前端 `vue-tsc / vite build / check-i18n` 全绿,`WikiSearchResults` 渲染服务端 `<mark>` 高亮,客户端不再二次高亮
- 端到端 smoke:`smoke-wiki-search-v2.sh` DRY_RUN-safe + 6 步(含 ACL 拒绝 + 跨租户拒绝 + 高亮片段断言)

## 背景

- `WikiPage.in_links` / `out_links` / `summary` / `slug` 自 v1 起就有;GIN tsvector 索引在 `migrations/versioned/000037_wiki_and_indexing.up.sql:73-74` 已有(`to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(content,''))`)但**完全没有被 Search repo 使用**
- `internal/application/repository/wiki_page.go:1201 Search` 用 `~*` regex 在 title/content/summary/slug 上 LIKE 扫描,无视 ACL,返回完整 `WikiPage[]`
- 路由 `wikiRead.GET("/search", ..., wikiHandler.SearchPages)`(`internal/router/routes_knowledge.go:395`)仅 KB 级 `KBAccessRead("kb_id")` 守卫
- 前端 `WikiSearchBar` + `WikiSearchResults` 已挂在 `WikiBrowser.vue:168,5551` 工具栏,客户端 `buildSnippet` + `highlight` 只在 title 上做
- Build #7 page-ACL(`wiki_page_acl`,migration 000091)从未被 Search 路径过滤 — **数据泄漏风险**(有 ACL DENY 的页面被搜出来)

### 关键约束
- tsvector `'simple'` config 对中文不分词 — D6 留 Build #19.x 用 `pg_trgm` + `jieba` 升级,本 Build 只命中英文/西文/数字/标识符
- ACL join(`wiki_page_acl`)可能让 query 复杂化 — 本 Build 只做"全 allow 列表匹配" + "kb_id 在用户可见 KB 列表里"双约束,不做"用户必须在 ACL 中"过滤(因为 ACL 是 additive allow,默认对所有 KB 用户可见)
- 老 endpoint 行为不能破坏 — `?legacy=1` 走老路径,6 个月后下线
- 单 KB 内 `page_type` 是 `concept` / `entity` / `summary` / `synthesis` / `comparison`,response 里透传便于前端做 chip 过滤

## 范围

### 1. 后端 — 新 `WikiSearchV2`

#### 1.1 类型(`internal/types/wiki_search.go`)

新增:
```go
type WikiSearchV2Hit struct {
    Slug     string  `json:"slug"`
    Title    string  `json:"title"`
    Snippet  string  `json:"snippet"`  // 服务端 ts_headline 高亮,带 <mark>
    Score    float64 `json:"score"`
    KBID     string  `json:"kb_id"`
    KBName   string  `json:"kb_name"`
    PageType string  `json:"page_type"`
}

type WikiSearchV2Result struct {
    Hits       []WikiSearchV2Hit `json:"hits"`
    Total      int               `json:"total"`
    TookMS     int               `json:"took_ms"`
    KBIDs      []string          `json:"kb_ids"`
    QueryEcho string             `json:"query"`
}

type WikiSearchV2Request struct {
    Query     string   `json:"q"`
    KBIDs     []string `json:"kb_ids"`      // 空 = 当前租户内全 KB
    Limit     int      `json:"limit"`        // 默认 20,最大 100
    Offset    int      `json:"offset"`       // 默认 0
    PageTypes []string `json:"page_types"`  // 空 = 全部
}
```

#### 1.2 Repository(`internal/application/repository/wiki_search_v2.go`)

实现:
```go
func (r *wikiSearchV2Repo) Search(
    ctx context.Context,
    tenantID int64,
    userID string,           // 用于 tenant 可见 KB 过滤
    req WikiSearchV2Request,
) (WikiSearchV2Result, error)
```

SQL 骨架:
```sql
WITH visible_kbs AS (
    SELECT id, name FROM knowledge_bases
    WHERE tenant_id = $1 AND ($2::text[] IS NULL OR id = ANY($2))
),
hits AS (
    SELECT
        p.slug, p.title, p.kb_id, p.page_type,
        ts_rank(
            setweight(to_tsvector('simple', coalesce(p.title,'')), 'A') ||
            setweight(to_tsvector('simple', coalesce(p.content,'')), 'B'),
            plainto_tsquery('simple', $3)
        ) AS rank,
        ts_headline(
            'simple', coalesce(p.title,'') || ' ' || coalesce(p.content,''),
            plainto_tsquery('simple', $3),
            'StartSel=<mark>,StopSel=</mark>,MaxFragments=2,MaxWords=20,MinWords=5'
        ) AS snippet
    FROM wiki_pages p
    JOIN visible_kbs vk ON vk.id = p.kb_id
    WHERE
        p.status != 'archived'
        AND ($4::text[] IS NULL OR p.page_type = ANY($4))
        AND (
            setweight(to_tsvector('simple', coalesce(p.title,'')), 'A') ||
            setweight(to_tsvector('simple', coalesce(p.content,'')), 'B')
        ) @@ plainto_tsquery('simple', $3)
    LIMIT $5 OFFSET $6
)
SELECT hits.*, vk.name AS kb_name FROM hits
JOIN visible_kbs vk ON vk.id = hits.kb_id
ORDER BY rank DESC, updated_at DESC;
```

#### 1.3 Service(`internal/application/service/wiki_search_v2.go`)

校验 + 调 repo,空 query 直接返回空 hits。

#### 1.4 Handler(`internal/handler/wiki_search_v2.go`)

`GET /api/v1/knowledgebase/:kb_id/wiki/search?v=2`
- 不破坏老 endpoint:query 里没有 `v=2` 时走老 `SearchPages`
- 解析 `kb_ids[]`(可选,空则用租户内全 KB)
- ACL 检查:每个 KB 必须 `KBAccessRead("kb_id")` 通过
- 注入 `tenant_id` 与 `user_id` 到 repo

#### 1.5 路由
- `wikiRead.GET("/search", ..., wikiSearchHandler.SearchV2)` — query `v=2` 走新
- 保留老 `wikiHandler.SearchPages`(内部判定 query `legacy=1`)

#### 1.6 DI
- `must(container.Provide(repository.NewWikiSearchV2Repository))`
- `must(container.Provide(service.NewWikiSearchV2Service))`
- `must(container.Provide(handler.NewWikiSearchV2Handler))`
- handler 注入 `WikiPageService`(复用已有 ACL 决策)+ `WikiSearchV2Service`

### 2. 前端 — search v2 client + render

#### 2.1 API client(`frontend/src/api/wiki/searchV2Types.ts` + `searchV2.ts`)
```ts
searchWikiPagesV2(
  kbId: string,        // 当前 KB,仅用于路由
  opts: { q, kbIds?, pageTypes?, limit?, offset? }
): Promise<WikiSearchV2Result>
```

#### 2.2 Store(`frontend/src/stores/wikiSearchV2.ts`)
- 复用现有 `useWikiSearchStore` 之外新增 `useWikiSearchV2Store`
- 字段:`hits`, `total`, `tookMs`, `loading`, `error`, `lastQuery`
- 方法:`searchV2()`, `clear()`

#### 2.3 组件
- `WikiSearchBar` 加 `kbIds[]` 多选 chip(默认 = 当前 KB,跨 KB 时全选)
- `WikiSearchResults` 改用 `v-html="hit.snippet"`(服务端 `<mark>` 高亮)
- 移除客户端 `highlight()` / `buildSnippet()`(仅留 legacy fallback)

#### 2.4 i18n
4 locale × 6 keys(`wiki.searchV2.*`):
- `empty` — 无结果
- `totalCount` — "找到 {total} 条,用时 {tookMs}ms"
- `loadingPage` — 加载中
- `kbChip` — "在 {N} 个 KB 中搜索"
- `pageTypeChip` — 类型 chip label
- `fallback` — v2 失败时回退 legacy

### 3. Smoke 脚本(`scripts/smoke-wiki-search-v2.sh`)
DRY_RUN-safe 6 步:
1. 创建 KB-A + page "alpha finance 报告"(英文命中)
2. 创建 KB-B + page "beta 财务披露"(跨 KB 命中)
3. `GET /wiki/search?v=2&q=finance` → 期望 hits 包含 alpha,不含 beta(标题里有"财务"但 query 是 "finance")
4. `GET /wiki/search?v=2&q=finance&kb_ids[]=kbA` → 仅 kbA
5. ACL DENY page → 不出现在 hits
6. `?v=2` 不可用时回退 `?legacy=1` 仍返回 WikiPage

## 验收(完整 A1–A12 进 spec.md)
- A1 新 endpoint 返回 `{hits,total,took_ms,kb_ids,query}` payload
- A2 服务端 `ts_headline` 高亮命中片段
- A3 ACL join:page-ACL DENY 页面不在 hits
- A4 跨 KB 模式:`?kb_ids[]=kbA,kbB` 合并结果按 rank 排序
- A5 租户隔离:跨租户 KB 不在 hits
- A6 排序:`ts_rank` 主,`title_match` 加权(title 字段权重高于 content)
- A7 老 endpoint `?legacy=1` 仍返回 WikiPage,行为不变
- A8 前端默认 v2,失败 fallback legacy
- A9 `WikiSearchResults` 渲染服务端 `<mark>`,不再二次高亮
- A10 跨 KB chip:`WikiSearchBar` 多选 KB
- A11 4 locale × 6 keys
- A12 smoke:ACL 拒绝 + 跨租户拒绝 + 高亮片段断言

## 关键复用
- `migrations/versioned/000037_wiki_and_indexing.up.sql:73-74` GIN tsvector 索引 — 直接利用
- `internal/application/repository/wiki_page.go:1201 Search` — 不替换(老 endpoint),新写一个 `wiki_search_v2.go` repo
- `WikiPageACL`(`internal/application/repository/wiki_acl.go`)— 跨 KB 时按 ACL 过滤
- 前端 `WikiSearchBar` / `WikiSearchResults` / `useWikiSearchStore` — 保留外壳,内部走 v2 client
- `internal/application/service/wiki_page.go:1189` 老 Search service — 不动

## 决策表(开 Build 即生效)

| ID | 决定 | 默认 |
|---|---|---|
| D1 | 检索后端 | `to_tsquery` + `ts_headline` + `ts_rank`,GIN 索引直接复用 |
| D2 | 响应体 | `{hits:[{slug,title,snippet,score,kb_id,kb_name,page_type}],total,took_ms,kb_ids,query}` |
| D3 | RBAC | KB 级 `KBAccessRead` + page-ACL join + 租户隔离 |
| D4 | 跨 KB | `kb_ids[]` 可选;省略时全租户内"有权访问的 KB"全跑 |
| D5 | 高亮 | 服务端 `ts_headline` `<mark>`;前端只渲染 |
| D6 | fuzzy / 同义词 | 本 Build 不做;留 Build #19.x |
| D7 | URL 兼容 | 老 `?legacy=1` 保留 6 个月 |
| D8 | 排序 | `ts_rank` 默认 + title 字段加权 + `updated_at` tie-break |

## 推进
1. backend(repo/service/handler/route/DI/test)→ commit 后端
2. frontend(API/store/WikiSearchResults 改 snippet + KB chip + i18n)→ commit 前端
3. smoke + 验证 + commit + push + reply

## Runtime blocker(已知)
同 Build #10/11/18:`comet native new` 仍被 stale Runtime index 拦截。绕行方式同上,直分支 + 产物落盘 + `git push` 到 `lumos0826`,不走 `comet native archive`。