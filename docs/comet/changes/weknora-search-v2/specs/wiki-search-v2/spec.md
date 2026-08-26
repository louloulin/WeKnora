# weknora-search-v2 — Spec

完整目标规格。Build #19 验收矩阵 A1–A12 与 brief.md 一一对应。

## A1 新 endpoint 返回 `{hits,total,took_ms,kb_ids,query}`

`GET /api/v1/knowledgebase/:kb_id/wiki/search?v=2&q=finance&limit=20`

成功响应(`200`):
```json
{
  "hits": [
    {
      "slug": "alpha-finance-report",
      "title": "alpha finance report",
      "snippet": "this <mark>finance</mark> report covers Q3 numbers...",
      "score": 0.06079,
      "kb_id": "kbA",
      "kb_name": "Finance KB",
      "page_type": "concept"
    }
  ],
  "total": 1,
  "took_ms": 12,
  "kb_ids": ["kbA"],
  "query": "finance"
}
```

空 query:返回 `{hits:[], total:0, took_ms:0, kb_ids:[...], query:""}`(200,不是 400)
- `?v=2` 缺失:走老 endpoint(WikiPage[]),`?legacy=1` 强制走老 endpoint
- 越界参数:`limit > 100` clamp 到 100,`offset < 0` clamp 到 0

## A2 服务端 ts_headline 高亮命中片段

- `StartSel=<mark>, StopSel=</mark>`
- `MaxFragments=2`(最多 2 个命中片段)
- `MaxWords=20`(每段最多 20 词)
- `MinWords=5`(每段最少 5 词)
- snippet 用 `\n` 拼接多片段
- 校验:响应中必须含 `<mark>...</mark>`(命中 query 才出现)

## A3 ACL join

- 复用 `WikiPageACL`(Build #7)的 allow 决策:当前用户在 `wiki_page_acl.allow_user_ids[]` 里的页面**进入** hits
- 不在 allow 列表 + status != 'archived' 的 page 仍默认**可见**(ACL 是 additive,默认对 KB 内所有人可见)—— 但本 Build 增加 ACL `DENY_USER`(如果页面有 `deny_user_ids[]` 包含当前用户,**移除**该页面)
- 后端检查:`wiki_page_acl` join WHERE 不匹配 deny

## A4 跨 KB 模式

`?v=2&q=finance&kb_ids[]=kbA&kb_ids[]=kbB`:
- 合并 hits,统一按 `score DESC, updated_at DESC` 排序
- `kb_ids[]` 元素**必须全部**通过 `KBAccessRead` 守卫;任一不通过 → `403`
- 空 `kb_ids[]`:全租户内用户可访问的 KB 全跑(默认行为)

## A5 租户隔离

- `wiki_pages.kb_id → knowledge_bases.tenant_id` 关联过滤
- repo 注入参数 `tenantID = JWT(tenant_id)`,SQL 用 `JOIN knowledge_bases ON kb_id WHERE tenant_id = $1`
- 跨租户 KB 不出现在 hits(哪怕 KB 公开)

## A6 排序

```
ORDER BY ts_rank DESC,
         (title_match_weight) DESC,
         updated_at DESC
```

- `title_match_weight`:title 字段命中时 `1.0`,content 字段命中时 `0.5`(setweight A vs B)
- ties:用 `updated_at` 倒序

## A7 老 endpoint ?legacy=1

- `?v=2` 缺失 或 `?legacy=1` 显式:走 `wikiHandler.SearchPages`,返回 WikiPage[]
- 6 个月内双轨;Build #19.x 删除老 endpoint(独立 Build 处理)

## A8 前端默认 v2,失败 fallback legacy

- `WikiSearchBar` 组件:发请求时默认带 `?v=2`,HTTP 500 / 网络错 → 自动 fallback `?legacy=1`
- UI 上不提示用户(无声降级)
- 单 KB 时:`kb_ids[]=当前 KB`;无当前 KB(从未进过 KB)→ `kb_ids[]` 省略,全租户搜

## A9 服务端 <mark> 渲染

- `WikiSearchResults` 改用 `v-html="hit.snippet"`,直接渲染服务端拼好的 `<mark>`
- 移除客户端 `highlight()` 调用(`utils/wikiSearch`)
- 安全:snippet 内容来自服务端 ts_headline 输出,**信任**(已在 Go 端 escape 过 `<` `>` `&` 之外保留 `<mark>`)

## A10 跨 KB chip

- `WikiSearchBar` 顶部新增 KB 多选 chip 行
- 默认全选"我能访问的所有 KB"(从 `/api/v1/knowledgebase` 列表 GET 一次)
- 点击 chip 切换是否在 kb_ids[] 中
- chip 数量 > 6 时折叠为"+N more",点击展开

## A11 4 locale × 6 keys

```yaml
wiki.searchV2:
  empty: "未找到匹配页面"
  totalCount: "找到 {total} 条 · 用时 {tookMs}ms"
  loading: "正在搜索..."
  kbChip: "在 {count} 个 KB 中搜索"
  pageTypeChip: "类型过滤"
```

`zh-CN` / `en-US` / `ko-KR` / `ru-RU` 各完整覆盖。

## A12 smoke 脚本

`scripts/smoke-wiki-search-v2.sh`(DRY_RUN=1 safe):

1. `GET /wiki/search?v=2&q=` → 200 + 空 hits
2. 创建 KB-A + page 含 "finance" → `GET ?v=2&q=finance` → 命中
4. `?kb_ids[]=kbA&kb_ids[]=kbB` → 跨 KB 合并
5. 跨租户 KB → 不在 hits
6. ACL DENY → 不在 hits
7. `?legacy=1` → 返回 WikiPage
8. snippet 含 `<mark>finance</mark>`

## 验证矩阵

| 验收 ID | 实现位置 | 测试方式 |
|---|---|---|
| A1 | `internal/handler/wiki_search_v2.go` + repo | harness test + smoke step 1-3 |
| A2 | repo SQL `ts_headline` | smoke step 8 |
| A3 | repo SQL `wiki_page_acl` join | harness test |
| A4 | handler `kb_ids[]` parse + repo SQL | smoke step 4 + harness |
| A5 | repo SQL `JOIN knowledge_bases WHERE tenant_id` | harness test |
| A6 | repo SQL `ORDER BY ts_rank DESC` | harness test |
| A7 | handler 老 path 分流 | smoke step 7 |
| A8 | `frontend/src/api/wiki/searchV2.ts` try/catch | 前端 vitest |
| A9 | `WikiSearchResults.vue` `v-html="hit.snippet"` | 浏览器目测 |
| A10 | `WikiSearchBar.vue` chip UI | 浏览器目测 |
| A11 | i18n 4 locale × 6 keys | `npm run check-i18n` 11/11 |
| A12 | `scripts/smoke-wiki-search-v2.sh` | `DRY_RUN=1 ./scripts/smoke-wiki-search-v2.sh` |