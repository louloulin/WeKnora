# weknora-search-v2-zh — Spec

完整目标规格。Build #19.x 验收矩阵 B1–B14 与 brief.md 一一对应。

## B1 迁移 `000096_wiki_search_zh.up.sql`

新建迁移文件 `migrations/versioned/000096_wiki_search_zh.up.sql`:

```sql
-- Migration 000096: wiki search zh + pg_trgm fuzzy
DO $$ BEGIN RAISE NOTICE '[Migration 000096] Applying wiki search zh + pg_trgm fuzzy schema'; END $$;

-- pg_trgm 幂等(Build #11 已加载)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 中文 tsvector 列(写入时由 jieba 计算填入,NULL-able 便于 backfill)
ALTER TABLE wiki_pages
  ADD COLUMN IF NOT EXISTS content_ts_zh tsvector;

-- 中文 GIN 索引
CREATE INDEX IF NOT EXISTS wiki_pages_ts_zh_idx
  ON wiki_pages USING GIN (content_ts_zh);

-- title trigram GIN 索引(英文 fuzzy 用)
CREATE INDEX IF NOT EXISTS wiki_pages_title_trgm_idx
  ON wiki_pages USING GIN (title gin_trgm_ops);

-- downgrade
DROP INDEX IF EXISTS wiki_pages_title_trgm_idx;
DROP INDEX IF EXISTS wiki_pages_ts_zh_idx;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS content_ts_zh;
-- pg_trgm 不 drop,可能被其它功能依赖
```

迁移注册:`migrations/migration.go`(或项目对应 migrator registry)新增 `000096`。

## B2 jieba 离线分词 + backfill

**写时双写**:`internal/application/service/wiki_page.go` 在 `Create` / `Update` 时:

```go
import "github.com/yanyiwu/gojieba"

var searchJieba = gojieba.NewJieba(
    filepath.Join(dictDir, "jieba.dict.utf8"),
    filepath.Join(dictDir, "hmm_model.utf8"),
    filepath.Join(dictDir, "user.dict.utf8"),
    filepath.Join(dictDir, "idf.utf8"),
    filepath.Join(dictDir, "stop_words.utf8"),
)

func tsZhFromContent(s string) string {
    words := searchJieba.CutForSearch(s) // 搜索引擎模式,粒度更细
    return strings.Join(words, " ")
}

func (s *wikiPageService) Create(ctx, ...) {
    // … 原有逻辑 …
    page.ContentTSZh = tsZhFromContent(page.Content + " " + page.Title)
    return s.repo.Create(ctx, page)
}
```

`dictDir` 复用 `internal/types/evaluation.go:dictDir` 的解析路径(env `WEKNORA_DICT_DIR` 优先,fallback `internal/dict/`)。

**Backfill**:`cmd/server/main.go` 启动时:

```go
go func() {
    var n int64
    db.Model(&WikiPage{}).Where("content_ts_zh IS NULL").Count(&n)
    if n == 0 { return }
    log.Infof("backfilling content_ts_zh for %d pages", n)
    const batch = 200
    for {
        var pages []WikiPage
        db.Where("content_ts_zh IS NULL").Limit(batch).Find(&pages)
        if len(pages) == 0 { break }
        for _, p := range pages {
            p.ContentTSZh = tsZhFromContent(p.Content + " " + p.Title)
            db.Model(&p).Update("content_ts_zh", p.ContentTSZh)
        }
        log.Infof("backfilled %d / %d", batch, n)
    }
    log.Info("content_ts_zh backfill complete")
}()
```

并发 N=2(同 wikiPageService 写入路径的 worker pool 复用),每次 200 条,batch 提交,跑完 → `content_ts_zh IS NULL` 永远 0 → 不再触发。

## B3 中文 query 命中

`GET /api/v1/knowledgebase/:kb_id/wiki/search?v=2&q=财务披露`:

响应:
```json
{
  "hits": [
    {
      "slug": "finance-disclosure",
      "title": "财务披露",
      "snippet": "本报告涵盖 <mark>财务</mark> <mark>披露</mark> 的关键要点…",
      "score": 0.06079,
      "kb_id": "kbA",
      "kb_name": "Finance KB",
      "page_type": "concept",
      "matched_by": "ts_zh"
    }
  ],
  "total": 1,
  "took_ms": 18,
  "kb_ids": ["kbA"],
  "query": "财务披露"
}
```

校验:
- `hits[0].matched_by == "ts_zh"`
- snippet 含 `<mark>财务</mark>` 和 `<mark>披露</mark>`(jieba `CutForSearch` 切成 2 个 token)
- `took_ms > 0`(service 层 jieba 计算 + tsvector 查询)

## B4 英文 fuzzy

`GET ?v=2&q=finace&fuzzy=1`:

数据库存 `"finance report"`,title 单字符 typo `"finace"` 经 `pg_trgm`:

```sql
similarity(lower(title), 'finace') -- > 0.3
```

→ 命中 `matched_by == "trgm"`,`score = similarity 值`。

阈值 `0.3` 是经验值(Build #11 ingest dedup 也用 0.3)。`req.Fuzzy == true` 时加进 SQL 的 OR 链;`false` 时跳过 fuzzy 路径。

## B5 partial 兜底

`GET ?v=2&q=alpha&partial=1`:

数据库存 `"Alpha Report"`(大小写不一致 + 没 fuzzy threshold),`lower(title) LIKE '%alpha%'` 命中 → `matched_by == "partial"`。

`req.PartialMatch == true` 时加进 SQL OR 链。默认 `false`(假阳多)。

## B6 三层 OR 互斥 + 优先级

同一 page 在三层都命中时,**只入一条 hit**,`matched_by` 取优先级:
```
ts_zh > ts_simple > trgm > partial
```

SQL 实现(伪码):
```sql
matched_by = CASE
  WHEN p.content_ts_zh @@ plainto_tsquery('simple', $jieba) THEN 'ts_zh'
  WHEN setweight(...) @@ plainto_tsquery('simple', $3)     THEN 'ts_simple'
  WHEN $fuzzy AND similarity(...) > 0.3                     THEN 'trgm'
  WHEN $partial_match AND lower(p.title) LIKE '%'||$3||'%'  THEN 'partial'
  ELSE 'none'
END
```

`CASE WHEN` 短路求值,所以第一条命中就 return,后续 WHEN 不再评估。

## B7 KB-ACL 集成(handler `loadVisibleKBIDs`)

`internal/handler/wiki_search_v2.go:loadVisibleKBIDs` 当前 `return nil, nil`,Build #19.x 改为:

```go
func (h *wikiSearchV2Handler) loadVisibleKBIDs(ctx context.Context, tenantID int64, userID string) ([]string, error) {
    if h.kbListSvc == nil {
        return nil, nil // 测试桩
    }
    kbs, err := h.kbListSvc.ListAccessibleKBs(ctx, tenantID, userID)
    if err != nil {
        return nil, err
    }
    ids := make([]string, len(kbs))
    for i, k := range kbs { ids[i] = k.ID }
    return ids, nil
}
```

`kbListSvc.ListAccessibleKBs` 见 B9。

## B8 跨 KB 模式(scope 默认 = 可见 KB)

请求 `?v=2&q=财务披露`(`kb_ids[]` 缺省):

repo 收到 `kb_ids == visibleKBIDs`(handler 算好注入),SQL:
```sql
JOIN visible_kbs vk ON vk.id = p.kb_id
WHERE vk.id = ANY($visible_kbs::text[])
```

请求 `?v=2&q=财务披露&kb_ids[]=kbA&kb_ids[]=kbB`(`kb_ids[]` 显式):

handler:
1. `effective = intersect(req.kb_ids, visibleKBIDs)` — 不在可见集合的 KB 直接 403
2. repo 收到 `kb_ids == effective`
3. SQL 同上

行为:用户传的 `kb_ids[]` 必须**全部** ACL 通过;否则 403。**不会**"自动收缩到可见集合"。

## B9 WikiAclService.ResolveBulk + ListAccessibleKBs

`internal/application/service/wiki_acl.go`:

```go
type AclResolveItem struct {
    KBID string
    Slug string
}

type WikiAclService interface {
    Resolve(ctx, kbID, slug, userID string) (Decision, error)
    ResolveBulk(ctx, items []AclResolveItem, userID string) (map[string]Decision, error)
}
```

实现:
```go
func (s *wikiAclService) ResolveBulk(ctx context.Context, items []AclResolveItem, userID string) (map[string]Decision, error) {
    // 60s TTL 缓存(同 Resolve)
    cacheKey := func(kb, slug string) string { return kb + ":" + slug }
    out := make(map[string]Decision, len(items))
    var toFetch []AclResolveItem
    for _, it := range items {
        if d, ok := s.cache.Get(cacheKey(it.KBID, it.Slug)); ok {
            out[cacheKey(it.KBID, it.Slug)] = d
        } else {
            toFetch = append(toFetch, it)
        }
    }
    // worker pool N=4 并发
    var wg sync.WaitGroup
    sem := make(chan struct{}, 4)
    var mu sync.Mutex
    for _, it := range toFetch {
        it := it
        wg.Add(1)
        sem <- struct{}{}
        go func() {
            defer wg.Done()
            defer func() { <-sem }()
            d, err := s.Resolve(ctx, it.KBID, it.Slug, userID)
            if err == nil {
                mu.Lock()
                out[cacheKey(it.KBID, it.Slug)] = d
                mu.Unlock()
            }
        }()
    }
    wg.Wait()
    return out, nil
}
```

`internal/application/service/knowledgebase_search.go` 新文件(若不存在):

```go
type AccessibleKBListService interface {
    ListAccessibleKBs(ctx context.Context, tenantID int64, userID string) ([]types.KnowledgeBase, error)
}

type accessibleKBListService struct {
    kbSvc      KnowledgeBaseService  // 复用 ListKnowledgeBases
    aclSvc     WikiAclService
    kbLimit    int
}

func (s *accessibleKBListService) ListAccessibleKBs(ctx, tenantID, userID) ([]KB, error) {
    // 1) 列全租户 KB
    kbs, _, err := s.kbSvc.ListKnowledgeBases(ctx, &ListKBRequest{TenantID: tenantID, Page: 1, PageSize: s.kbLimit})
    if err != nil { return nil, err }
    // 2) 对每个 KB,看 user 是否有 KBAccessRead
    visible := make([]KB, 0, len(kbs))
    for _, kb := range kbs {
        if hasRead, err := s.kbSvc.CheckAccess(ctx, kb.ID, userID, "read"); err == nil && hasRead {
            visible = append(visible, kb)
        }
    }
    return visible, nil
}
```

`kbLimit` 默认 200(单租户 KB 数通常 < 50)。

## B10 前端 chip 行激活

`frontend/src/components/wiki/WikiSearchBarV2.vue`:

```vue
<template>
  <div class="wiki-search-bar-v2">
    <WikiKBChipRow
      v-if="kbOptions.length > 0"
      :options="kbOptions"
      :selected="selectedKBIds"
      @toggle="onToggleKB"
    />
    <input v-model="query" @input="onInput" :placeholder="t('wiki.searchV2.placeholder')" />
    <label>
      <input type="checkbox" v-model="fuzzy" /> {{ t('wiki.searchV2.fuzzy') }}
    </label>
  </div>
</template>
```

`kbOptions` 是 prop,由父组件 `WikiBrowser.vue` 提供:

```ts
const kbOptions = ref<KBOption[]>([])

onMounted(async () => {
  const res = await api.listAccessibleKBs()  // GET /api/v1/knowledgebase?visible=1
  kbOptions.value = res.kbs.map(k => ({ id: k.id, name: k.name }))
})

const selectedKBIds = computed(() => /* 默认全选 kbOptions,用户点击切换 */)
```

`WikiKBChipRow` 子组件:Build #19 留 stub,Build #19.x 实现:
- 渲染 N 个 chip,每个 `kb.id` 一个
- 点击 chip → `@toggle(kbId)` → 父组件切换 `selectedKBIds`
- `kbOptions.length > 6` 时折叠:显示前 6 + `+N more` 按钮,点击展开全部

## B11 fuzzy toggle

`WikiSearchBarV2.vue` 加 checkbox:
```vue
<label class="fuzzy-toggle">
  <input type="checkbox" v-model="fuzzy" />
  {{ t('wiki.searchV2.fuzzy') }}
  <span class="hint">{{ t('wiki.searchV2.fuzzyHint') }}</span>
</label>
```

`fuzzy` 状态进 store `useWikiSearchV2Store`,搜索时传给 API。`partial` 不在 UI 暴露(默认 false),仅供未来 debug 或 KB 内部用。

## B12 4 locale × 5 keys

```yaml
# zh-CN
wiki.searchV2.fuzzy: "模糊匹配"
wiki.searchV2.fuzzyHint: "允许 1-2 个拼写错误"
wiki.searchV2.partialMatch: "包含子串"
wiki.searchV2.noVisibleKB: "您没有可搜索的知识库"
wiki.searchV2.kbChipScopedToVisible: "在 {count} 个可见 KB 中搜索"

# en-US
wiki.searchV2.fuzzy: "Fuzzy match"
wiki.searchV2.fuzzyHint: "Allow 1–2 typos"
wiki.searchV2.partialMatch: "Contains substring"
wiki.searchV2.noVisibleKB: "You have no searchable knowledge bases"
wiki.searchV2.kbChipScopedToVisible: "Searching across {count} visible KBs"

# ko-KR
wiki.searchV2.fuzzy: "퍼지 매치"
wiki.searchV2.fuzzyHint: "1~2개의 오타 허용"
wiki.searchV2.partialMatch: "부분 문자열 포함"
wiki.searchV2.noVisibleKB: "검색 가능한 지식 베이스가 없습니다"
wiki.searchV2.kbChipScopedToVisible: "{count}개의 표시된 KB에서 검색"

# ru-RU
wiki.searchV2.fuzzy: "Нечёткий поиск"
wiki.searchV2.fuzzyHint: "Допускаются 1–2 опечатки"
wiki.searchV2.partialMatch: "Содержит подстроку"
wiki.searchV2.noVisibleKB: "У вас нет доступных баз знаний"
wiki.searchV2.kbChipScopedToVisible: "Поиск по {count} видимым базам"
```

## B13 smoke 脚本 `scripts/smoke-wiki-search-v2-zh.sh`(DRY_RUN-safe)

```bash
#!/usr/bin/env bash
# Build #19.x — wiki search v2 zh + fuzzy + KB-ACL smoke

BASE="${BASE:-http://localhost:8080}"
KB_ID="${KB_ID:-kb-smoke}"
TOKEN="${TOKEN:-}"
DRY_RUN="${DRY_RUN:-1}"

# 1) 中文 query → ts_zh 命中
# 2) 英文 fuzzy typo → trgm 命中
# 3) partial 兜底 → partial 命中
# 4) ACL 拒绝的 KB 不在默认 kb_ids
# 5) GET /knowledgebase?visible=1 → 含全部可见 KB
# 6) pg_trgm 扩展未加载 graceful(仅走 ts_zh + ts_simple)
# 7) Build #19 回归:英文 query 仍 ts_simple 命中
```

7 步断言 + DRY_RUN 模式,沿用 `smoke-wiki-search-v2.sh` 的 `set -euo pipefail` 风格。

## B14 Build #19 A1-A12 零回归

Build #19 已通过的 12 个验收(见 `docs/comet/changes/weknora-search-v2/specs/wiki-search-v2/spec.md`)在 Build #19.x 完成后仍全部绿:

- A1 payload shape 不变(`hits/total/took_ms/kb_ids/query`),仅 `hit` 多 `matched_by` 字段(向后兼容:老客户端忽略未知字段)
- A2 ts_headline `<mark>` 不变
- A3 ACL join 不变(本 Build 不改 ACL SQL)
- A4 跨 KB 行为不变,只是 default scope = visible 而非租户全
- A5 租户隔离不变
- A6 排序:`ts_rank DESC, updated_at DESC` 主排序,trgm/partial 加进排序 OR但不影响主路径
- A7 `?legacy=1` 不动
- A8 前端默认 v2 + 失败 fallback 不变
- A9 服务端 `<mark>` 渲染不变
- A10 chip 行激活是 Build #19 留空数组 → Build #19.x 激活,**不破坏 Build #19 行为**(Build #19 是空数组也能跑,只是没 chip)
- A11 4 locale × 6 keys 不变(Build #19.x 新增 5 keys,不删除)
- A12 smoke 脚本 Build #19 仍可独立跑(Build #19.x 新 smoke 是 `smoke-wiki-search-v2-zh.sh` 独立文件)

## 验证矩阵

| 验收 ID | 实现位置 | 测试方式 |
|---|---|---|
| B1 | `migrations/versioned/000096_wiki_search_zh.up.sql` | 迁移 dry-run + GIN 索引 `\d wiki_pages` 验证 |
| B2 | `internal/application/service/wiki_page.go` 写时 + `cmd/server/main.go` backfill | harness test:create page 后查 `content_ts_zh` 非 NULL |
| B3 | repo SQL `content_ts_zh @@ plainto_tsquery` | smoke step 1 + harness |
| B4 | repo SQL `similarity() > 0.3` | smoke step 2 + harness |
| B5 | repo SQL `LIKE '%q%'` | smoke step 3 + harness |
| B6 | repo SQL `CASE WHEN` 短路 | harness test:三层命中只一条 hit |
| B7 | `internal/handler/wiki_search_v2.go loadVisibleKBIDs` | harness test + smoke step 4 |
| B8 | handler intersect + repo SQL | harness test |
| B9 | `internal/application/service/wiki_acl.go ResolveBulk` + `knowledgebase_search.go ListAccessibleKBs` | harness test:50 KB × 4 worker 并发 + 60s TTL cache |
| B10 | `frontend/src/components/wiki/WikiSearchBarV2.vue` + `WikiKBChipRow` | vitest + smoke step 5 |
| B11 | `WikiSearchBarV2.vue` checkbox | vitest + 浏览器目测 |
| B12 | i18n 4 locale × 5 keys | `npm run check-i18n` |
| B13 | `scripts/smoke-wiki-search-v2-zh.sh` | `DRY_RUN=1 ./scripts/smoke-wiki-search-v2-zh.sh` |
| B14 | Build #19 既有验收 | `DRY_RUN=1 ./scripts/smoke-wiki-search-v2.sh` 仍 ALL PASSED |
