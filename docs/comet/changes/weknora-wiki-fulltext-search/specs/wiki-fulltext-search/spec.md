# Spec · wiki-fulltext-search

> 配套 brief:`brief.md`

新增 WeKnora wiki 模块的全文搜索能力:用户从 wiki 浏览器顶栏输入关键词,前端按标题 + 正文匹配当前 KB 内页面,返回按相关度排序的结果列表(含标题、面包屑路径、高亮片段),点击跳转到对应页面。前端独力实现,后端 API 接口签名先定、Go 实现待本地环境补。

**范围**:前端 API client + Pinia store + 搜索框组件 + 结果弹层 + WikiBrowser 工具栏集成 + mock 索引 + 4 语言 i18n + 单元测试。

**不包含**:后端 Go 代码实现、搜索索引持久化、跨 KB 搜索、搜索分页(只返回 top 50)、搜索历史持久化、AI 语义搜索、拼写纠错、同义词扩展、自动补全。

---

## Acceptance criteria

1. **A1** WikiBrowser 工具栏出现搜索框 `WikiSearchBar.vue`,DOM 检查可见。
3. **A2** 输入 ≥ 2 字符触发搜索;`< 2` 字符清空结果;输入触发 debounce 200ms。
4. **A3** 搜索结果每条显示面包屑路径 + 标题 + 高亮片段;关键词在标题/片段中加 `<mark>` 标签。
5. **A4** 关键词大小写不敏感(全小写比较)。
6. **A5** 多关键词按空格 split 后 AND 匹配;每个关键词都必须在标题或正文出现。
7. **A6** 标题命中权重 > 正文命中(标题 +10,正文 +1)。
8. **A7** 按 ESC 键关闭结果弹层;点击弹层外部关闭。
9. **A8** 搜索结果为空时显示空态文案。
10. **A9** 搜索加载中(`store.loading=true`)显示 spinner。
11. **A10** 点击搜索结果跳转到 `/wiki/page/:slug`。
12. **A11** i18n 4 语言完整覆盖:`zh-CN / en-US / ko-KR / ru-RU`,通过 `npm run i18n:check`。
13. **A12** `npm run typecheck` 通过(vue-tsc)。
14. **A13** `npm run build` 通过。
15. **A14** `npm run test:unit` 覆盖搜索算法 + store + 组件,全绿。

---

## Component sketch

- `frontend/src/api/wiki/search.ts` — API client + 类型定义。
- `frontend/src/stores/wikiSearch.ts` — Pinia store(query / results / loading / error / showResults / history)。
- `frontend/src/utils/wikiSearch.ts` — 纯函数 `scoreMatch(keywords, title, content, path)`,供 store 与单元测试共用。
- `frontend/src/components/wiki/WikiSearchBar.vue` — 顶栏搜索框 + 防抖 + ESC 关闭 + 上下方向键导航。
- `frontend/src/components/wiki/WikiSearchResults.vue` — 结果弹层(loading / empty / error / list 四态)。
- `frontend/src/mock/wikiSearchIndex.ts` — 20 条样例数据,本地 mock 阶段使用。
- `frontend/src/locales/{zh-CN,en-US,ko-KR,ru-RU}/wiki.json` — 增量新增 `wiki.search.*` key。

---

## API contract(约定)

```
GET /api/v1/knowledgebase/:kb_id/wiki/search?q=<keywords>&limit=50

Response 200:
{
  "results": [
    { "pageId": "string", "slug": "string", "title": "string", "path": ["string"], "snippet": "string", "score": 0 }
  ],
  "total": 0
}
```

前端调用 `searchWikiPages(kbId, keywords, limit=50): Promise<WikiSearchResponse>`,当前阶段返回 mock 数据;接口签名稳定后,后端就绪即替换 mock 为真实调用。

---

## Integration

- 修改 `frontend/src/views/knowledge/wiki/WikiBrowser.vue`,在工具栏顶部插入 `<WikiSearchBar :kb-id="kbId" />`。
- 不修改任何后端文件。

---

## Test plan

- `frontend/src/utils/__tests__/wikiSearch.spec.ts` — `scoreMatch` 单关键词/多关键词/大小写/词频/AND 逻辑。
- `frontend/src/stores/__tests__/wikiSearch.spec.ts` — debounce / loading / empty / error / history 行为。
- `frontend/src/components/wiki/__tests__/WikiSearchBar.spec.ts` — 输入触发、ESC 关闭、点击外部关闭、点击结果 select 事件。

---

## 风险

| 风险 | 应对 |
| --- | --- |
| 关键词搜索召回率不够 | 已接受基础搜索,后续可接语义层 |
| mock 与真实 API 结构不一致 | 强类型 + 后端就绪即替换 |
| i18n key 漏语言 | `npm run i18n:check` 强制覆盖 |