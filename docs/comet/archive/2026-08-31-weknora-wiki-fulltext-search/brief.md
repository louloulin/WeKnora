# Brief · weknora-wiki-fulltext-search

> Comet-Native change · branch `lumos0826` · base `lumos0826`(cc530015) · 上游 `Tencent/WeKnora` v0.7.2
> 配套 issue:LUM-20(louloulin)

---

## Outcome

在 WeKnora wiki 模块里补齐**全文搜索**能力,让用户能在当前 KB 内按标题 + 正文快速找到目标页面,关键词高亮、按相关度排序、可点击跳转。前端独力实现,后端 API 留好接口签名待本地 Go 工具链就绪再补。

完成后,用户从 wiki 工具栏的搜索框输入任意关键词,即可看到所有命中页面的标题 + 路径 + 高亮片段,点进去直接跳到对应页面;无需刷新整个 KB。

---

## Scope

| # | 能力 | 实现位置 | 类型 |
| --- | --- | --- | --- |
| 1 | API client + 类型定义 | `frontend/src/api/wiki/search.ts`(新) | 前端 |
| 2 | Pinia store(查询 / 结果 / 加载态 / 历史) | `frontend/src/stores/wikiSearch.ts`(新) | 前端 |
| 3 | 顶栏搜索框 | `frontend/src/components/wiki/WikiSearchBar.vue`(新) | 前端 |
| 4 | 搜索结果弹层 | `frontend/src/components/wiki/WikiSearchResults.vue`(新) | 前端 |
| 5 | 集成到 WikiBrowser 工具栏 | 修改 `frontend/src/views/knowledge/wiki/WikiBrowser.vue` | 前端 |
| 6 | 模拟数据 + 客户端索引 | `frontend/src/mock/wikiSearchIndex.ts`(新,临时,后端就绪后可删) | 前端 mock |
| 7 | i18n 4 语言 | `frontend/src/locales/{zh-CN,en-US,ko-KR,ru-RU}/wiki.json` 增量 | 前端 |
| 8 | 单元测试(高亮 + 排序逻辑) | `frontend/src/stores/__tests__/wikiSearch.spec.ts`(新) | 前端 |

**全部合计**:前端独力可完成,沙箱内可验证 `vue-tsc` + `npm run build` + `npm run test`。

---

## Non-goals

明确**不做**的事,避免 scope creep:

1. **不做后端 Go 代码**(`internal/handler/wiki_search.go` 等)—— 沙箱无 Go 工具链,接口签名先定,后端等本地环境就绪后另开 Build
2. **不引入新搜索算法库**(不引入 lunr.js / fuse.js / flexsearch)—— 当前 KB 页面数 < 10000,简单的关键词扫描 + 词频打分足够,避免依赖膨胀
3. **不做跨 KB 搜索**—— 留在当前 KB scope,跨 KB 是后续 Build
4. **不做搜索结果分页**—— 前端一次性返回 top 50,后续按需扩展
5. **不做搜索历史持久化**—— 当前会话 in-memory 即可,后续可接 localStorage
6. **不做 AI 语义搜索**—— 关键词搜索足够,语义是另一条线
7. **不动后端既有结构**—— wiki_pages / wiki_page_revisions 表不动

---

## Acceptance examples

每条验收项具体、可观察、可独立验证:

| ID | 验收标准 | 可观察证据 |
| --- | --- | --- |
| A1 | WikiBrowser 工具栏出现搜索框 | 截图 + DOM 检查 `WikiSearchBar` 元素存在 |
| A2 | 输入关键词 ≥ 2 字符触发搜索 | 输入 "meeting",debounce 200ms 后 store 触发 fetch |
| A3 | 搜索结果含标题 + 面包屑路径 + 高亮片段 | 命中页面显示 "Meeting Notes" 标题,路径 "/Team/Weekly",正文片段 "...the **meeting** agenda..." 高亮 |
| A4 | 关键词大小写不敏感 | 输入 "MEETING" 与 "meeting" 返回相同结果 |
| A5 | 多个关键词 AND 匹配 | 输入 "weekly meeting",只返回同时含两个词的页面 |
| A6 | 标题命中权重 > 正文命中 | "meeting" 在标题命中的页面排前,在正文命中后排后 |
| A7 | 搜索结果按 ESC 关闭 / 点击外部关闭 | 弹层响应 ESC + outside-click 事件 |
| A8 | 搜索结果为空时显示空态文案 | 显示本地 "没有匹配的页面" 提示 |
| A9 | 搜索加载中显示 spinner | store.loading=true 时显示 loading 图标 |
| A10 | 搜索结果点击跳转到对应页面 | 点击结果项,Vue router 跳到 `/wiki/page/:slug` |
| A11 | 4 语言 i18n 完整覆盖 | `vue-tsc --noEmit` 通过 + i18n completeness check |
| A12 | vue-tsc 通过 | `npm run typecheck` exit 0 |
| A13 | npm run build 通过 | `npm run build` exit 0 |
| A14 | 单元测试覆盖搜索 + 高亮 + 排序逻辑 | `npm run test:unit` 全绿 |

---

## Constraints and invariants

- **最低侵入**:不修改任何后端文件,前端只新增文件 + WikiBrowser 工具栏插入点
- **依赖一致**:`frontend/package.json` 不新增依赖(用浏览器原生 `String.includes` + 简单词频打分)
- **i18n 一致**:所有新文案都进 4 个 locale 文件,与 Build #5/#6/#7 风格一致(每个 locale 文件 `wiki` 命名空间同步新增 key)
- **TDesign 风格**:`WikiSearchBar` 用 `t-input` + `t-popup`,`WikiSearchResults` 用 `t-list` + `t-tag`,与现有 wiki 组件保持一致
- **沙箱可验证**:`vue-tsc` + `npm run build` + `npm run test:unit` 必须本地通过

---

## Decisions

| 决定 | 选项 | 理由 |
| --- | --- | --- |
| 算法选择 | 客户端关键词扫描 + 词频打分 | 零依赖、KB 内页面 < 10000 性能足够、可控 |
| 后端 API 设计 | `GET /api/v1/knowledgebase/:kb_id/wiki/search?q=<keywords>&limit=50` | RESTful、跟现有 wiki API 风格一致 |
| 后端返回结构 | `{results: [{pageId, slug, title, path, snippet, score}]}` | 跟搜索结果 UI 直接对应 |
| 评分公式 | 标题命中 +10 / 正文命中 +1 / 词频 × 1 | 简单、可解释 |
| Debounce | 200ms | 平衡响应感 + 不卡顿 |
| 最大结果数 | top 50 | 用户友好,性能可控 |
| 空态 | 显示本地 "没有匹配的页面" + 建议 | 跟 Build #6 分享页面空态一致 |
| 是否走 Comet wrapper | 走(走 Shape → Build → Verify → Archive) | 跟 Build #5/6/7/8 节奏一致;Build #2b 已解锁,工作区干净 |
| 是否新分支 | 否(用 `--isolation current`,直接在 `lumos0826` commit) | 跟前端 Build 一致,后续 squash merge 不需要 |
| 是否需要本地 dev env | 不需要(纯前端 + mock) | 沙箱可独立完成 |

---

## Open questions

无。所有实现 + 验收 + 范围都已确定,可以进入 Build 阶段。

---

## Verification expectations

Build 阶段完成后,Comet 会进入 Verify 阶段,verifier 会:

1. 读 brief + spec + 全部 14 条验收项
2. 实际检查代码 + 跑 `vue-tsc` + `npm run build` + `npm run test:unit`
3. 检查 4 语言 i18n 完整性
4. 逐项判定 pass / fail / blocked

预期结果:全部 14 条 pass。如果某项 fail,回到 Build 修。

---

## Risks

| 风险 | 应对 |
| --- | --- |
| 关键词搜索召回率不够(同义词 / 词形变化) | 用户已接受"先做基础搜索",后续可接语义层 |
| 搜索结果 score 不符合直觉 | 通过 A6 测试用例覆盖,可调整打分 |
| i18n key 漏掉某语言 | `npm run i18n:check` 强制覆盖 |
| mock 数据与真实 API 数据结构不一致 | 在 API client 层定义强类型,后端就绪时直接替换 mock |