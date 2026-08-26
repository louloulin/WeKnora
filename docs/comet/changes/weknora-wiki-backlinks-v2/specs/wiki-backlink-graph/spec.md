# Build #20 — Wiki 反向链接图升级(Backlinks Graph v2)验收矩阵

> 与 Runtime 验收矩阵同构,扁平 A1–A20 编号,每条都是可独立验证的项。
> brief 在同目录 `../brief.md`。

## 后端(A1–A6)

### A1 — 新端点 `GET /api/v1/knowledgebase/:kbId/wiki/pages/:slug/backlinks/graph`

`internal/handler/wiki_page.go` 新增 `GetPageBacklinksGraph(c *gin.Context)` 方法,注册到与 Build #11 的 `GetPageBacklinks` 同级路由组;`go build / vet / gofmt` 退出码 0。

### A2 — 公开类型 4 个 + 容器 1 个

`internal/types/wiki_page.go` 新增:
- `WikiBacklinkIndirect { via string }`(嵌入 `WikiPageBacklink`)
- `WikiPageBacklinkRelated { jaccard float64 }`(嵌入 `WikiPageBacklink`)
- `WikiBacklinkBroken { target_slug string }`
- `WikiBacklinkGraphStats { direct_count, indirect_count, related_count, broken_count, out_link_count int }`
- `WikiBacklinkGraph { direct, indirect, related, broken, stats }`

JSON tag 全部 snake_case,`time.Time` 序列化为 RFC3339。

### A3 — Service `ListBacklinkGraph` 4 段计算正确

`internal/application/service/wiki_page.go` 新增 `ListBacklinkGraph(ctx, req WikiBacklinkGraphRequest) (*WikiBacklinkGraph, error)`:
- direct = Build #11 `ListPageBacklinks` 输出
- indirect = ∪{p.in_links for p in direct} ∖ {self} ∖ direct,带 `via` 标记来源 direct slug,按 `updated_at` 倒序,截 `req.MaxIndirect`
- related = 解析当前页 `out_links` → 排除自身/空 out_links 候选 → Jaccard ≥ 阈值 → 按 jaccard 倒序,截 `req.MaxRelated`
- broken = `out_links` ∖ `existingSlugs`(当前 KB 内存在的 slug 集合),按字母排序
- stats = 4 个 count + out_link_count

### A4 — 参数 clamp

query 参数处理:
- `max_indirect`:空 → 50;< 0 → 0;> 200 → 200
- `max_related`:空 → 10;< 0 → 0;> 50 → 50
- `jaccard`:空 → 0.3;< 0 → 0;> 1 → 1
- 解析失败 → 400 + `{error: "invalid query"}`

### A5 — Harness 单元测试 6 用例

`internal/application/service/wiki_page_backlinks_v2_test.go` 新文件,覆盖:
1. 空 KB(目标页 in_links 为空 + out_links 为空)→ `{direct:[], related:[], indirect:[], broken:[], stats:{0,0,0,0,0}}`
2. 2-hop 去重:A → C → B,A → D → B;B 的 indirect 应包含 C 和 D 各一次,不应包含 A(`via` 字段标记)
3. Jaccard 边界:页 X out_links = {a,b,d},页 Y out_links = {a,b,c},Z out_links = {a,b,d,e};threshold=0.5 → Y 不在 X 的 related(0.5 刚好不满足应被过滤掉)
4. 失效识别:页 B out_links 含 `[[missing]]`;B.broken 含 `{target_slug:"missing"}`
5. self-exclude:B 的 in_links 含自身 slug(防御性)→ 被排除不出现在 direct/indirect
6. 间接页缺失过滤:direct.in_links 含孤儿 slug(对应页已删)→ 不出现在 indirect

### A6 — 不影响 Build #11 旧端点

`GET /wiki/pages/:slug/backlinks` 返回形状与字段不变(`[]WikiPageBacklink`);Build #11 的 smoke 脚本 `scripts/smoke-wiki-backlinks.sh` 必须仍然 ALL PASSED。

## 前端(A7–A13)

### A7 — API 类型与函数导出

`frontend/src/api/wiki/index.ts` 新增:
- 5 个 interface(WikiBacklinkGraphRequest / WikiBacklinkIndirect / WikiPageBacklinkRelated / WikiBacklinkBroken / WikiBacklinkGraphStats / WikiBacklinkGraph)
- `getWikiBacklinkGraph(kbId, slug, params?): Promise<WikiBacklinkGraph>`,函数体内调用 `get<...>` + 用 `encodeSlugPath(slug)`

### A8 — Pinia store `wikiBacklinks` 增量

`frontend/src/stores/wikiBacklinks.ts` 新增:
- `graphFor(kbId, slug): WikiBacklinkGraph | null`(同步读缓存)
- `loadGraph(kbId, slug): Promise<WikiBacklinkGraph>`(拉取并写缓存)
- `invalidate(kbId, slug)`:同时清 `backlinksFor` 与 `graphFor` 缓存
- `loadFailed(kbId, slug): boolean` 标志位

### A9 — `WikiBacklinksPanel.vue` 升级为 4 段

组件保留 Build #11 props/emits 签名,模板结构升级为:
- header(标题 + summary 4 chip:直接/间接/相关/失效)
- 4 个 `<details>` 或自定义 collapse section,各自独立折叠状态(localStorage 持久化)
- 底部"View full graph →"链接,调 `WikiBrowser` 已有 `loadGraphEgo(center=currentSlug)` 触发全图 ego 模式

每段行为:
- direct:click → `navigateToSlug(slug)`,显示 `updated_at` 相对时间(与 Build #11 一致)
- indirect:click → `navigateToSlug(via)`,显示 `(via page-a)` 副标题
- related:click → `navigateToSlug(slug)`,显示 `+0.78` jaccard chip
- broken:无 click,显示 slug + i18n 提示

### A10 — `WikiBrowser.vue` 接入

`WikiBrowser.vue:660` 现有 `<WikiBacklinksPanel>` 不变;panel header 的"View full graph →"链接绑定到 `selectGraphCenter(slug)` 触发 ego 模式。

切换页面时 store 自动 `loadGraph`(debounce 200ms);加载失败 → 降级只显示 direct(Build #11 端点),底部 toast"高级信息加载失败,仅显示直接引用"。

### A11 — i18n 4 locale × 14 key

`frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}.ts` 新增 `wiki.backlinksGraph.*`:
- `title`(升级后的 section 总标题)
- `sections.direct / indirect / related / broken`(4 段标题)
- `summary.fourCounts`(`{direct} {indirect} {related} {broken}` 模板)
- `via`(`(via {slug})` 副标题)
- `jaccard`(`+{n}` 显示格式)
- `viewFullGraph`("View full graph →")
- `loadFailedToast`(降级 toast)
- `brokenHint`("目标已删除或重命名")

i18n key 检查脚本 `npm run check-i18n` 4 locale 必须全 PASS。

### A12 — `WikiBacklinksPanel` 折叠状态持久化

折叠状态写入 `localStorage[wikiBacklinksPanel:collapse]: {direct, indirect, related, broken}`,key 命名空间 + 默认 `direct:false,indirect:true,related:true,broken:true`(直接展开、其余折叠)。

### A13 — `WikiBacklinksPanel.test.ts` vitest 覆盖

`frontend/src/components/wiki/WikiBacklinksPanel.test.ts` 新增用例:
1. 4 段 payload 完整 → 4 section 都渲染,数字 chip 与 summary 一致
2. 折叠 → click section header → 内部列表展开
3. indirect click 触发 `navigateToSlug(via)` 事件(而非 indirect slug)
4. broken 不可点击(无 click handler 暴露)
5. 加载失败 → 仅 direct 渲染,toast 文本匹配 `loadFailedToast` i18n key

## Smoke + Verify(A14–A20)

### A14 — smoke 脚本 7 步

`scripts/smoke-wiki-backlinks-v2.sh` DRY_RUN-safe,7 步 curl + 断言:
1. Build #11 旧端点 `/backlinks` → `[]WikiPageBacklink`(零回归)
2. Build #20 新端点 `/backlinks/graph` 空 KB → 4 段都 `[]`(200)
3. 2-hop 间接识别
4. Jaccard ≥ threshold → related 非空
5. broken 含缺失 slug
6. 参数 clamp:`max_indirect=0` → indirect=[];`max_related=999` → clamp;`jaccard=1.5` → clamp
7. 跨 KB → 403

### A15 — Build #11 零回归

`scripts/smoke-wiki-backlinks.sh` Build #11 6 步必须 ALL PASSED 不变。

### A16 — `vue-tsc --build` 0 Build #20 错误

允许与 Build #11 / #19.x 同源的历史 TS 错误继续存在(不属 Build #20 范围)。

### A17 — `npm run build` 通过

产物 `dist/` 大小增长 ≤ 5KB(Build #20 主要是类型 + panel 升级,无大依赖)。

### A18 — `npm run check-i18n` 14 key × 4 locale 通过

### A19 — `go build ./...` + `go test ./internal/application/service/...` 通过

`wiki_page_backlinks_v2_test.go` 6 用例全 pass,既有 `wiki_page_backlinks_test.go`(Build #11)4 用例不破。

### A20 — commit + push + reply

分支 `lumos0826`,commit message 含 `#20` 前缀;push 到 `origin/lumos0826`;LUM-20 reply 挂在父 `01a03d9f` 下。