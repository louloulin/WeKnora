# weknora-search-v2-zh — Build #19.x 简介(全文搜索 v2 中文分词 + pg_trgm fuzzy + KB-ACL visible-KB 列表)

## 目标

把 Build #19 的 PostgreSQL `to_tsvector('simple', …)` 全量英文 search 升级到三层路径:

1. **中文**:用 `gojieba` 在写入/更新时离线分词,索引到新增 `tsvector_zh` 列 → `to_tsquery('simple', jieba_terms)` 命中
2. **英文 fuzzy**:`pg_trgm` 扩展(已在 Build #11 deduplication 路径就位,复用)+ GIN trigram 索引 → `similarity(q, title) > threshold` 或 `slug LIKE '%q%'` 命中
3. **KB-ACL 可见列表**:service 层用 `WikiAclService.Resolve` 批量决策用户可见 KB → 前端 `WikiSearchBar` chip 行渲染真实"我能搜哪些 KB",`?kb_ids[]` 缺省时用此集合替代"租户内全 KB"

Build #19.x 是 Build #19 的同 endpoint 升级(`?v=2` 走新逻辑),不破坏老 endpoint(`?legacy=1`),不引入新 endpoint。

实现完成的标准:
- 后端 `go build` / `go vet` / harness 单测全绿,迁移 `000096_wiki_search_zh.up.sql` 创建 `tsvector_zh` 列 + GIN 索引,`pg_trgm` 扩展幂等 `CREATE EXTENSION IF NOT EXISTS`
- 中文 query `"财务披露"`(无空格的 4 个汉字)能命中 title/content 包含该短语的页面
- 英文 fuzzy:title 单字符 typo(`"finace"` → `"finance"`)经 `similarity()` 命中
- 前端 `WikiSearchBar` chip 行从空数组升到"我能搜的 N 个 KB",点击 chip 切换 `kb_ids[]`
- i18n 4 locale × 新增 `wiki.searchV2.fuzzy` / `wiki.searchV2.partialMatch` 文案

## 背景

- Build #19 用 `to_tsvector('simple', …)`,`simple` config 不分词 → 中文 query `"财务披露"`、`"风控"`、`"跨境"` 完全不命中,zh-CN 是默认 locale,**主用户场景不可用**
- 现成的 `gojieba`(Build #2a `internal/types/evaluation.go:9-23`)在 evaluation 服务里用过;web 服务目前未 import `gojieba`
- `pg_trgm` 在 Build #11 ingest dedup 路径里用过(`internal/application/service/wiki_ingest_dedup.go:30`),扩展已加载;`wiki_pages.title` 上是否有 GIN trigram 索引需要迁移确认
- `WikiAclService.Resolve`(`internal/application/service/wiki_acl.go:43-118`)已有 60s TTL 缓存的 allow/deny 决策;调用粒度"单 slug",批量决策需要 `ResolveBulk(slug, kbID)[]` 扩展
- Build #19 handler `loadVisibleKBIDs`(`internal/handler/wiki_search_v2.go:103-115`)目前 `return nil, nil` 占位 — Build #19.x 必须落地
- 前端 `WikiBrowser.vue:kbOptions = ref<KBOption[]>([])` 是空数组占位 — Build #19.x 必须填充

### 关键约束
- jieba 分词**必须离线**(写入时算好存到 `tsvector_zh`),不能 query 时再分词(query 里有英文/数字/标点时分词结果差,且要保证 tsvector 列稳定可索引)
- pg_trgm `similarity()` 对中文无效(中文没空格切 trigram),fuzzy 仅作用英文/西文/数字/标识符
- KB-ACL 决策必须走 service 层(60s TTL 缓存),不能在 repo SQL 里 join `wiki_page_acl`(性能 + 缓存语义破坏)
- `?v=2` 必须**零回归**:Build #19 现有 A1-A12 验收保持绿

## 范围

### 1. 后端 — 中文 + fuzzy + KB-ACL

#### 1.1 迁移 `000096_wiki_search_zh.up.sql`

```sql
-- pg_trgm 扩展(Build #11 已加载,IF NOT EXISTS 幂等)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- tsvector 中文分词列(写入时由 jieba 计算填入)
ALTER TABLE wiki_pages
  ADD COLUMN IF NOT EXISTS content_ts_zh tsvector
    GENERATED ALWAYS AS (
      -- 触发器 + 应用层 jieba 双写:迁移只创建列 + 触发器
      -- 占位:NULL,触发器 BEFORE INSERT/UPDATE 用 to_tsvector('simple', jieba_result) 填
      NULL
    ) STORED;
```

迁移策略改成 **应用层双写**(触发器需要 superuser,且 jieba 词库要随包更新,触发器内嵌 jieba 不现实):
- 迁移只 `ALTER TABLE … ADD COLUMN content_ts_zh tsvector`(无 GENERATED,NULL-able)
- 已有数据由 `cmd/server` 启动时一次性 backfill(若 `count(*) WHERE content_ts_zh IS NULL > 0` 触发一个 batch job)
- 后续 `WikiPageService.Create/Update` 在写库前算好 `content_ts_zh` 一并写入

GIN 索引:
```sql
CREATE INDEX IF NOT EXISTS wiki_pages_ts_zh_idx ON wiki_pages USING GIN (content_ts_zh);
CREATE INDEX IF NOT EXISTS wiki_pages_title_trgm_idx ON wiki_pages USING GIN (title gin_trgm_ops);
```

#### 1.2 类型 + 接口(`internal/types/wiki_search_v2.go` + `interfaces/wiki_search_v2.go`)

扩 `WikiSearchV2Request`:
```go
type WikiSearchV2Request struct {
    Query           string   `json:"q"`
    KBIDs           []string `json:"kb_ids"`
    Limit           int      `json:"limit"`
    Offset          int      `json:"offset"`
    PageTypes       []string `json:"page_types"`
    Fuzzy           bool     `json:"fuzzy"`             // 新增:英文 fuzzy 开关
    PartialMatch    bool     `json:"partial_match"`     // 新增:title LIKE %q% 兜底
}
```

扩 `WikiSearchV2Hit`:
```go
type WikiSearchV2Hit struct {
    // … 原有 …
    MatchedBy string `json:"matched_by"` // 新增:"ts_zh" | "ts_simple" | "trgm" | "partial"
}
```

#### 1.3 Repository(`internal/application/repository/wiki_search_v2.go`)

SQL 改成三层 OR,每层独立 `matched_by` 标记:
```sql
WITH visible_kbs AS (…),
hits AS (
  SELECT
    p.slug, p.title, p.kb_id, p.page_type,
    p.updated_at,
    ts_rank(
      setweight(to_tsvector('simple', coalesce(p.title,'')),'A') ||
      setweight(to_tsvector('simple', coalesce(p.content,'')),'B'),
      plainto_tsquery('simple', $3)
    ) AS rank_simple,
    CASE WHEN p.content_ts_zh @@ plainto_tsquery('simple', $jieba) THEN 1.0 ELSE 0 END AS rank_zh,
    CASE WHEN $fuzzy AND similarity(lower(coalesce(p.title,'')), lower($3)) > 0.3 THEN similarity(lower(coalesce(p.title,'')), lower($3)) ELSE 0 END AS rank_trgm,
    ts_headline(
      'simple', coalesce(p.title,'') || ' ' || coalesce(p.content,''),
      plainto_tsquery('simple', $3),
      'StartSel=<mark>,StopSel=</mark>,MaxFragments=2,MaxWords=20,MinWords=5'
    ) AS snippet,
    CASE
      WHEN p.content_ts_zh @@ plainto_tsquery('simple', $jieba) THEN 'ts_zh'
      WHEN setweight(...) @@ plainto_tsquery('simple', $3)     THEN 'ts_simple'
      WHEN $fuzzy AND similarity(...) > 0.3                     THEN 'trgm'
      WHEN $partial_match AND lower(p.title) LIKE '%'||$3||'%'  THEN 'partial'
      ELSE 'none'
    END AS matched_by
  FROM wiki_pages p
  JOIN visible_kbs vk ON vk.id = p.kb_id
  WHERE
    p.status != 'archived'
    AND (
      p.content_ts_zh @@ plainto_tsquery('simple', $jieba)
      OR setweight(...) @@ plainto_tsquery('simple', $3)
      OR ($fuzzy AND similarity(...) > 0.3)
      OR ($partial_match AND lower(p.title) LIKE '%'||$3||'%')
    )
  LIMIT $5 OFFSET $6
)
SELECT hits.*, vk.name AS kb_name FROM hits
JOIN visible_kbs vk ON vk.id = hits.kb_id
ORDER BY (rank_zh + rank_simple + rank_trgm) DESC, updated_at DESC;
```

`$jieba` 是 query 字符串经 jieba 分词后空格拼接的结果(`"财务披露"` → `"财务 披露"`),由 service 层在调用 repo 前算好。

#### 1.4 Service(`internal/application/service/wiki_search_v2.go`)

- `q := jieba.CutForSearch(req.Query)` → `zhQuery := strings.Join(q, " ")` → 传给 repo
- 当 `req.Fuzzy == false && req.PartialMatch == false`(默认),中文 query 命中 `ts_zh` 列,英文 query 命中 `simple` tsvector(等价 Build #19 行为)
- 当 `req.Fuzzy == true`,额外 join `pg_trgm` 路径(默认 `?fuzzy=1`)
- 当 `req.PartialMatch == true`,title LIKE 兜底(默认 `?partial=1`)

KB-ACL 集成(替换 `internal/handler/wiki_search_v2.go:103-115` 占位):
- service 调 `acl.ResolveBulk(ctx, tenantKBs, userID)` 拿到 `visibleKBIDs`
- 把 `visibleKBIDs` 传给 repo 作为 `kb_ids` 上界

#### 1.5 WikiAclService 扩展(`internal/application/service/wiki_acl.go`)

新增方法:
```go
type WikiAclService interface {
    // Resolve … 已有
    ResolveBulk(ctx context.Context, items []AclResolveItem, userID string) (map[string]Decision, error)
}

type AclResolveItem struct {
    KBID string
    Slug string
}
```

实现:`map[string]Decision` keyed by `kbID+":"+slug`,内部按 `kbID` 分组调 `wiki_page_acl` 一次,缓存语义同 `Resolve`(60s TTL)。

#### 1.6 可见 KB 列表(`internal/application/service/knowledgebase_search.go` 或新文件)

新接口 `ListAccessibleKBs(ctx, tenantID, userID) ([]KB, error)`:
- 复用 `ListKnowledgeBases(tenantID, page, size)`(`internal/application/service/knowledgebase.go:xxx`)拿全租户 KB 列表
- service 层并发(worker pool N=4)调 `acl.ResolveBulk` 检查每个 KB 是否可读
- 过滤掉非可见 KB → 返回 `[]KB`

handler `loadVisibleKBIDs(ctx, tenantID, userID)`:
- Build #19.x 起调 `kbSvc.ListAccessibleKBs(...)` 而不是 `return nil, nil`
- 返回 `(IDs, error)`:`IDs` 直接作为 repo 默认 `kb_ids` 上界

#### 1.7 DI(`internal/container/container.go`)

```go
must(container.Provide(repository.NewWikiSearchV2Repository))
must(container.Provide(service.NewWikiSearchV2Service))
must(container.Provide(handler.NewWikiSearchV2Handler))
must(container.Provide(service.NewAccessibleKBListService)) // 新
```

`WikiSearchV2Service` 构造注入新增的 `AclResolver`(批量)+ `KBListService`。

### 2. 前端 — fuzzy toggle + KB chip 行激活

#### 2.1 API client(`frontend/src/api/wiki/searchV2.ts`)

扩 `searchWikiPagesV2`:
```ts
searchWikiPagesV2(kbId, {
  q, kbIds?, pageTypes?, limit?, offset?,
  fuzzy?: boolean,         // 默认 true
  partialMatch?: boolean,  // 默认 false
})
```

#### 2.2 组件 `WikiSearchBarV2.vue`

- 新增 fuzzy toggle(单 checkbox):label = `wiki.searchV2.fuzzy`
- chip 行:Build #19 留空数组 → Build #19.x 改为 `kbOptions` prop 传入,组件渲染 `<WikiKBChipRow>` 子组件
- 默认全选当前用户可见 KB;点击 chip 切换是否在 `kb_ids[]` 中
- chip 数量 > 6 时折叠为 `+N more`

#### 2.3 `WikiBrowser.vue`

Build #19 留的 `kbOptions = ref<KBOption[]>([])` → Build #19.x 通过 `GET /api/v1/knowledgebase?visible=1`(新建)或 service 端 ListAccessibleKBs 暴露的接口拿到真实 `kbOptions`,传给 `<WikiSearchBarV2>`。

#### 2.4 i18n(4 locale × 新增 keys)

```yaml
wiki.searchV2:
  fuzzy: "模糊匹配"
  fuzzyHint: "允许 1-2 个拼写错误"
  partialMatch: "包含子串"
  noVisibleKB: "您没有可搜索的知识库"
  kbChipScopedToVisible: "在 {count} 个可见 KB 中搜索"
```

`zh-CN` / `en-US` / `ko-KR` / `ru-RU` 各完整覆盖。

### 3. Smoke 脚本(`scripts/smoke-wiki-search-v2-zh.sh`,DRY_RUN-safe)

7 步:
1. `GET /wiki/search?v=2&q=财务披露` → 期望 `hits[0].matched_by == "ts_zh"`
2. `GET /wiki/search?v=2&q=finance&fuzzy=1&partial=1&typo=finace` → 期望命中 finance 页面(`matched_by == "trgm"`)
3. `GET /wiki/search?v=2&q=alpha&partial=1` → 期望 title 含 "alpha" 的页面命中(`matched_by == "partial"`)
4. ACL 拒绝:用户不在某 KB allow 列表 → 该 KB 不出现在 `?kb_ids[]` 缺省响应的 kb_ids 字段
5. `GET /api/v1/knowledgebase?visible=1` → 返回 `kb_ids[]` 中所有 KB 都在结果中
6. 中文 query 但 `pg_trgm` 扩展未加载 → graceful fallback(只走 `ts_zh` + `ts_simple`)
7. `?v=2` 缺失 + 中文 query → 仍走 Build #19 simple tsvector(不命中中文),作为已知 limitation

## 验收(完整 B1–B14 进 spec.md)

- B1 迁移 `000096_wiki_search_zh.up.sql`:`pg_trgm` 幂等 + `content_ts_zh` tsvector 列 + GIN zh 索引 + GIN title trigram 索引
- B2 jieba 离线分词:`WikiPageService.Create/Update` 在写库前算好 `content_ts_zh`,已存在的 wiki_pages 在 `cmd/server` 启动时一次性 backfill
- B3 中文 query 命中:含 `"财务披露"` 的页面 title → `hits[0].matched_by == "ts_zh"`,snippet 含 `<mark>财务</mark>` 或 `<mark>披露</mark>`
- B4 英文 fuzzy:typo `"finace"` → `"finance"` 页面命中,`hits[0].matched_by == "trgm"`
- B5 partial 兜底:typo `"alpha"` 实际存 `"Alpha"`(大小写),开 `partial=1` 命中,`matched_by == "partial"`
- B6 三层 OR 互斥:同一 page 只会有一条 hit,`matched_by` 取优先级 `ts_zh > ts_simple > trgm > partial`
- B7 KB-ACL 集成:handler `loadVisibleKBIDs` 不再返回 `nil`,而是 `kbSvc.ListAccessibleKBs(tenantID, userID)` 的结果
- B8 跨 KB 模式:`?kb_ids[]` 缺省时 scope = 可见 KB;`?kb_ids[]` 显式时仍按 Build #19 行为(只是每个 KB 必须 ACL 通过)
- B9 `WikiAclService.ResolveBulk` 批量决策:`map[string]Decision` keyed by `kbID+":"+slug`,60s TTL 缓存,worker pool N=4 并发
- B10 前端 chip 行激活:`kbOptions` prop 从空数组升到真实 KB 列表;点击 chip 切换 `kb_ids[]`;chip 数 > 6 折叠
- B11 fuzzy toggle:`WikiSearchBarV2` 加 checkbox,默认开,改变 `req.Fuzzy` 传给 API
- B12 4 locale × 5 keys(`fuzzy` / `fuzzyHint` / `partialMatch` / `noVisibleKB` / `kbChipScopedToVisible`)
- B13 smoke 7 步:中文命中 + trgm typo + partial 兜底 + ACL 过滤 + ListAccessibleKBs + 缺扩展 graceful + Build #19 回归
- B14 Build #19 A1-A12 零回归(同 endpoint `?v=2` 已有英文场景仍绿)

## 关键复用

- `gojieba`(`internal/types/evaluation.go:9`)— 已有依赖,Build #19.x 在 web 服务中 import 后初始化 `var Jieba = newJieba()` 单例
- `pg_trgm` 扩展 — Build #11 已加载,迁移 `IF NOT EXISTS` 幂等
- `WikiAclService.Resolve`(`internal/application/service/wiki_acl.go:43`)— 已有 60s TTL,`ResolveBulk` 复用同 cache key
- `ListKnowledgeBases`(tenant) — Build #19.x 加 `ListAccessibleKBs` 包装,内部调 `ListKnowledgeBases` + ACL 过滤
- `migrations/versioned/000037_wiki_and_indexing.up.sql:73-74` 已有 `tsvector simple` GIN — Build #19.x 不动,新增 `content_ts_zh` 走自己的 GIN
- `frontend/src/api/wiki/searchV2.ts` — Build #19 已有,扩 `fuzzy` / `partialMatch` 参数
- `frontend/src/components/wiki/WikiSearchBarV2.vue` — Build #19 已有,fuzzy toggle + chip 行数据流通

## 决策表(开 Build 即生效,锁定 D1-D4 来自推荐)

| ID | 决定 | 默认值 | 来自 |
|---|---|---|---|
| D1 | jieba 分词存储 | 双 tsvector 列:`content_ts`(simple,Build #19 已有)+ `content_ts_zh`(jieba 写时算) | D1 推荐 |
| D2 | fuzzy 实现 | `pg_trgm` 仅作用英文/西文/数字/标识符(title `gin_trgm_ops` + `similarity() > 0.3`);中文 fuzzy 留 Build #19.x+1(需要 `ngram` 或 `pg_jieba`) | D2 推荐 |
| D3 | ACL 批量决策 | service 层 `WikiAclService.ResolveBulk(map[kbid+":"+slug]→Decision)`,60s TTL 缓存 + worker pool N=4;repo SQL 不动 ACL | D3 推荐 |
| D4 | 可见 KB 列表来源 | 复用 `ListKnowledgeBases(tenantID)` + service 层并发 ACL 过滤 → 不新增 interface | D4 推荐 |
| D5 | fuzzy 默认开 | `req.Fuzzy == true`(默认) | 来自 jieba 也只对中文好,英文 typo 是常见场景 |
| D6 | partial 兜底默认关 | `req.PartialMatch == false`(默认);仅当 fuzzy + ts_simple 都未命中时,UI 上提示用户"开启包含匹配" | 来自 partial 假阳多 |
| D7 | backfill 触发 | `cmd/server` 启动时:`SELECT count(*) FROM wiki_pages WHERE content_ts_zh IS NULL`,> 0 触发 background batch(并发 N=2,每次 200 条,跑完置 NULL → 关掉) | 来自存量数据可能百万级 |
| D8 | 优先级 | `matched_by` 取 `ts_zh > ts_simple > trgm > partial`,同一 page 三层都命中只入一条 | 来自中文质量最高 |

## 推进

1. backend(迁移 000096 + jieba 集成 + repo 三层 OR + service bulk ACL + kbSvc.ListAccessibleKBs)→ commit 后端
2. frontend(API 扩 fuzzy/partial + chip 行激活 + fuzzy toggle + 5 keys i18n)→ commit 前端
3. smoke + 验证 + commit + push + reply

## Runtime blocker(已知)

同 Build #10/11/18/19:`comet native new` 仍被 stale Runtime index 拦截。绕行方式同上,直分支 + 产物落盘 + `git push` 到 `lumos0826`,不走 `comet native archive`。
