# weknora-wiki-backlinks-v2 — Build #20 简介(反向链接图升级)

## 目标

把 Build #11 的扁平"反向链接"面板升级为**分组图谱视图**,一次性回 4 块信息让读者/编辑者立即看清页面在 KB 知识网络中的位置:

1. **直接引用**(1-hop) — Build #11 已经做了,这里保留并放在最上面
2. **间接引用**(2-hop) — 通过 1-hop 页面再向外扩散,看"页面 A 也被哪些页面引用"
3. **相关页面** — 与当前页 `out_links` 集合重合度最高(Jaccard ≥ 0.3)的 N 个页
4. **链接健康** — 当前页 `out_links` 中指向不存在 slug 的"无效引用"

实现完成的标准:
- 后端 `go build / vet / gofmt` 全绿,新增端点 `GET /wiki/pages/:slug/backlinks/graph` 返回 200 + 4 段 payload
- 既有 `GET /wiki/pages/:slug/backlinks`(Build #11)行为不变
- 前端 `vue-tsc / vite build / check-i18n / vitest` 全绿,`WikiBacklinksPanel.vue` 升级为 4-section 可折叠面板
- 端到端 smoke:KB 内建 A → B 链 A → C → B,A 链 D(孤儿),B 的 backlinks/graph 返回 `{direct: [C], indirect: [A], related: [...], broken: [D], stats: {...}}`

## 背景

Build #11(`docs/comet/changes/weknora-wiki-backlinks/`)已经把数据层和"扁平列表"面板打通了:

- `wiki_pages.in_links text[]` 列,`parseOutLinks` / `updateInLinks` / `removeInLinks` 在 create/update/delete/rename 时自动维护(`internal/application/service/wiki_page.go:1082-1307`)
- `WikiPageService.ListPageBacklinks(kbID, slug)` 按 `updated_at` 倒序返回引用页 lite 投影
- `GET /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/backlinks` 路由已注册(`internal/router/routes_knowledge.go:341`)
- 前端 `WikiBacklinksPanel.vue`(挂载在 `WikiBrowser.vue:660`)显示引用列表,默认折叠,click → `navigateToSlug`

全局图端点也存在:
- `WikiPageService.GetGraph(req)`(`internal/application/service/wiki_page.go:671-862`)支持 `WikiGraphModeOverview`(top-N 中心页)与 `WikiGraphModeEgo`(以 center slug 做 BFS,深度可配),`computeGraphSubset` 已被 `wiki_page_test.go:405-540` 多用例覆盖
- `WikiBrowser.vue` 已有 `graphData` ref + bloom-generation 累积逻辑,这是 KB 全图视角(与单页面板互为补充)
- `WikiLintService`(`internal/application/service/wiki_lint.go`)已经识别 `orphan_page` / `broken_link` / `stale_ref` / `missing_cross_ref` / `empty_content` / `duplicate_slug`,`GET /wiki/lint` 端点已注册(`internal/router/routes_knowledge.go:403`)

### 关键约束(也是 Build #20 的边界)

- **不重写 Build #11**:`GET /pages/:slug/backlinks` 端点形状、签名、行为必须保持(前端 panel 不显示断链的兼容降级路径仍可走旧端点)
- **不重写 GetGraph**:它是 KB 全图视角(多 page 节点 + 边),Build #20 是**单页中心视角**(以 1 个 page 为锚,4 段邻居聚合)。两者不冲突,前端在 panel header 加一个"View full graph →"链接到全图视图
- **不动 WikiLint**:`/wiki/lint` 是全 KB 慢查询,Build #20 只针对当前页的 out_links 做轻量检查,不做全 KB lint
- **2-hop 限定**:即便 BFS 也只走 2 层;更深留给 Build #21(全图 canvas 视图时再考虑)
- **related pages 走 Jaccard**:不引入 embedding 模型,字符串集合重合度即可,无新依赖
- **跨 KB 不动**:Build #19.x 已让 `kb_options` 出现在 UI,但单页 backlinks 仍是 KB 内闭环(跨 KB 留 Build #20.x+1)

## 范围

### 1. 后端 — 新端点 `GET /api/v1/knowledgebase/:kbId/wiki/pages/:slug/backlinks/graph`

#### 1.1 Service 层(`internal/application/service/wiki_page.go`)

新增方法:
```go
type WikiBacklinkGraphRequest struct {
    KbID             string
    Slug             string
    MaxIndirect      int  // 默认 50,0 表示不返间接
    MaxRelated        int  // 默认 10,0 表示不返相关
    JaccardThreshold float64 // 默认 0.3
}

type WikiBacklinkGraph struct {
    Direct   []*WikiPageBacklink      `json:"direct"`   // 1-hop
    Indirect []*WikiBacklinkIndirect  `json:"indirect"` // 2-hop,带来源 1-hop slug
    Related  []*WikiPageBacklinkRelated `json:"related"` // Jaccard >= threshold
    Broken   []*WikiBacklinkBroken    `json:"broken"`   // 当前页 out_links 中不存在 slug
    Stats    WikiBacklinkGraphStats   `json:"stats"`
}

type WikiBacklinkIndirect struct {
    *WikiPageBacklink            // 嵌入 lite
    Via string `json:"via"`      // 哪个 1-hop slug 引入了这条间接
}

type WikiPageBacklinkRelated struct {
    *WikiPageBacklink
    Jaccard float64 `json:"jaccard"`  // 0..1
}

type WikiBacklinkBroken struct {
    TargetSlug string `json:"target_slug"`
    // 不强求行内位置 — 服务端不解析 markdown 行号,只列目标 slug
}

type WikiBacklinkGraphStats struct {
    DirectCount   int `json:"direct_count"`
    IndirectCount int `json:"indirect_count"`
    RelatedCount  int `json:"related_count"`
    BrokenCount   int `json:"broken_count"`
    OutLinkCount  int `json:"out_link_count"`
}

func (s *wikiPageService) ListBacklinkGraph(
    ctx context.Context, req WikiBacklinkGraphRequest,
) (*WikiBacklinkGraph, error)
```

实现要点:
1. **Direct**(1-hop):复用 Build #11 `ListPageBacklinks`(`internal/application/service/wiki_page.go:1103`),只是把结果换 embed 形式
2. **Indirect**(2-hop):取 direct 的 slug 列表 → `repo.ListBySlugs(ctx, kbID, directSlugs)` → 取出每个 1-hop 的 `in_links` → 合并去重(扣除 self + direct)→ 按 `updated_at` 倒序截 `MaxIndirect`
3. **Related**:取当前页 `out_links` → `repo.ListBySlugs(ctx, kbID, outLinks)`(只解析存在的页)→ 对每个候选计算 Jaccard = `|out_links ∩ current.out_links| / |out_links ∪ current.out_links|`(对 out_links 为空的候选得 0,过滤掉;阈值默认 0.3,可配)→ 截 `MaxRelated`
4. **Broken**:当前页 `out_links` ∖ `existingSlugs`(差异集)→ 转 `[]WikiBacklinkBroken`,按字母序排
5. **Stats**:在以上计算过程中累加

性能预算:
- 4 段都从 in-memory 已经加载的页出发,不引入新表
- 单次调用 SQL 总数 ≤ 4(`GetBySlug` + `ListBySlugs` × 3),命中 Build #19 的 `WikiPageLite` 复用路径
- 极端 KB(N > 10000)走现有的 per-tenant 索引,不另加

#### 1.2 类型(`internal/types/wiki_page.go`)

新增 4 个公开类型(`WikiBacklinkIndirect` / `WikiPageBacklinkRelated` / `WikiBacklinkBroken` / `WikiBacklinkGraphStats`)+ 容器 `WikiBacklinkGraph`。
注:`WikiPageBacklink` 直接复用 Build #11 已公开的类型(`internal/types/wiki_page.go:1275`),不重新定义字段。

#### 1.3 Repo 增量

`internal/application/repository/wiki_page.go` 的 `WikiPageRepository` 接口不需要新方法;`ListBySlugs` 已经能 cover 1-hop/2-hop/related 三个候选集合的批量读取(见 `internal/types/interfaces/wiki_page.go:125`)。

#### 1.4 Handler(`internal/handler/wiki_page.go`)

新增 `GetPageBacklinksGraph(c *gin.Context)`:
- swagger annotation `@Router /knowledgebase/{kb_id}/wiki/pages/{slug}/backlinks/graph [get]`
- query 参数 `max_indirect`(默认 50,clamp 0..200)、`max_related`(默认 10,clamp 0..50)、`jaccard`(默认 0.3,clamp 0..1)
- 404 路径:page not found → 404;成功 → 200 + 4 段 JSON
- 调 `wikiService.ListBacklinkGraph(ctx, req)`

#### 1.5 路由(`internal/router/routes_knowledge.go`)

在 Build #11 的 `/pages/*slug/backlinks` 旁边加一行:
```go
wikiRead.GET("/pages/*slug/backlinks/graph", g.Viewer(), g.KBAccessRead("kb_id"), wikiHandler.GetPageBacklinksGraph)
```
viewer 守卫 + KB-read 中间件与现有 `/backlinks` 同级,**不加 ACL 后置过滤**(D4 见下)。

### 2. 前端

#### 2.1 API 客户端(`frontend/src/api/wiki/index.ts`)

新增:
```ts
export interface WikiBacklinkGraphRequest {
  max_indirect?: number;
  max_related?: number;
  jaccard?: number;
}
export interface WikiBacklinkIndirect extends WikiPageBacklink { via: string }
export interface WikiPageBacklinkRelated extends WikiPageBacklink { jaccard: number }
export interface WikiBacklinkBroken { target_slug: string }
export interface WikiBacklinkGraphStats {
  direct_count: number; indirect_count: number; related_count: number;
  broken_count: number; out_link_count: number;
}
export interface WikiBacklinkGraph {
  direct: WikiPageBacklink[];
  indirect: WikiBacklinkIndirect[];
  related: WikiPageBacklinkRelated[];
  broken: WikiBacklinkBroken[];
  stats: WikiBacklinkGraphStats;
}
export function getWikiBacklinkGraph(
  kbId: string, slug: string, params?: WikiBacklinkGraphRequest,
): Promise<WikiBacklinkGraph>
```

#### 2.2 Pinia store 增量(`frontend/src/stores/wikiBacklinks.ts`)

新增 `wikiBacklinkGraphFor(kbId, slug)`,内部:
- 缓存键 `(kbId, slug)`
- `loadBacklinkGraph(kbId, slug)` 调 API + 写入缓存
- `invalidate(kbId, slug)` 同时清空 `backlinksFor` 与 `wikiBacklinkGraphFor`
- 暴露 `loadFailed: boolean` 标志位(失败时降级回 Build #11 旧端点)

#### 2.3 `WikiBacklinksPanel.vue` 升级

保留 Build #11 的整个 props/emits 签名,内部从 1 段列表升级为 4 段折叠 section:

```
┌─────────────────────────────────────┐
│ 反向链接                             │  <- Build #11 已有 header
│ ↳ 直接引用 12  间接 38  相关 5 失效 2 │  <- 新增 summary line
├─────────────────────────────────────┤
│ ▾ 直接引用 (12)                      │  <- Build #11 列表,保留 click 行为
│   • page-a   (updated 2 days ago)
│   • page-b   ...
│ ▸ 间接引用 (38)                      │  <- 默认折叠,展开显示 via slug
│   • page-x via page-a
│ ▸ 相关页面 (5)                       │  <- Jaccard 分数 +0.45 等
│   • page-y 0.78
│ ▸ 失效引用 (2)                       │  <- 只读列表,无 click
│   • [[missing-slug]]
│ ─ View full graph →                  │  <- 跳 WikiBrowser 全图视图
└─────────────────────────────────────┘
```

实现细节:
- 4 段 section 各自独立折叠状态(`Set<'direct'|'indirect'|'related'|'broken'>`),localStorage 持久化
- summary line 的 4 个数字分别带 i18n 标签和颜色 chip:direct=主色、indirect=辅色、related=灰、broken=warn
- 间接引用的点击行为:跳到 `via` slug(更近的一跳),不是间接页本身;UI 显示 "(via page-a)" 提示用户
- 失效引用不可点击,只显示 slug + i18n 提示 "目标已删除或重命名"
- 加载中显示骨架屏,失败时降级只显示 direct(Build #11 端点),底部 toast 提示"高级信息加载失败"
- 每次 `selectedPage` 切换,store 自动 `loadBacklinkGraph`(debounce 200ms)

#### 2.4 `WikiBrowser.vue` 增量

- 现有 `<WikiBacklinksPanel>` 挂载点不动(`WikiBrowser.vue:660`)
- panel header 的 "View full graph →" 链接目标:复用现有 `graphData` 加载路径(`WikiBrowser.vue:3629-3795` 的 `loadGraphOverview`),只是把 `center` 设为当前 slug,触发 ego 模式

### 3. Smoke(`scripts/smoke-wiki-backlinks-v2.sh`,DRY_RUN-safe)

7 步 curl 走 Build #11 与 Build #20 双端点:
1. 直接命中 Build #11 旧端点 `/backlinks` → 形状不变(零回归)
2. 直接命中 Build #20 新端点 `/backlinks/graph` → 4 段都存在(空 KB 也返 `direct:[]`,不 400)
3. 间接命中(建 A → B 链 A → C → B,A 链 D 孤儿)→ B 的 graph.indirect 包含 C(via=B?),broken 包含 D
4. 相关命中(建 X,Y,Z 三页都引用同一组 `[[finance]]`)→ 任一页 graph.related 含其余两页,jaccard ≥ 0.3
5. 失效命中(本页 out_links 含 `[[missing-slug]]`)→ graph.broken 含 `missing-slug`
6. 参数 clamp:`max_indirect=0` → indirect=[];`max_related=999` → 被 clamp 到 50;`jaccard=1.5` → 被 clamp 到 1
7. KB 隔离:跨 KB slug 在 visibleKBIDs 之外 → 端点 403(借用 Build #19.x 的 `KBAccessRead` 中间件)

### 4. 验收矩阵

`specs/wiki-backlink-graph/spec.md` 编号 A1–A20,涵盖:
- A1-A4:后端端点、类型、4 段 payload、参数 clamp
- A5-A6:service harness 单元测试(空 KB / 2-hop 去重 / Jaccard 边界 / 失效识别)
- A7-A10:前端 API 类型、Pinia store、panel 4 段渲染、跨页加载
- A11-A13:i18n 4 locale × ~14 key(新增 `wiki.backlinksGraph.*` 命名空间)
- A14-A15:smoke + Build #11 旧端点零回归
- A16-A18:vitest(panel 折叠状态、summary 数字、broken 空状态)
- A19-A20:vue-tsc / npm build / check-i18n 通过 + commit

## 关键决策(等你拍板)

**D1 — 2-hop 深度上限**
- **A 全展开(默认 MaxIndirect=50,clamp 0..200)** — 推荐,小 KB 体验好;clamp 防止大 KB O(N²)
- B 每 1-hop 最多再展开 10 个 — 复杂边界条件,收益小

**D2 — related pages 算法**
- **A Jaccard on out_links(字符串集合)** — 推荐,无需 embedding,与 Build #11 `parseOutLinks` 输出同源
- B 双塔 embedding + cosine — 重,需新模型,留 Build #21

**D3 — broken links 范围**
- **A 仅当前页 out_links 中的孤儿 slug** — 推荐,复用 Build #11 的 `ListBySlugs` 差异集
- B 全 KB lint — 重,需要调 `WikiLintService.RunLint`,留 Build #22

**D4 — 是否做 ACL 后置过滤**
- **A 不加**(端点用 `g.KBAccessRead("kb_id")` 即可 — 用户能进 KB 就应能看到全部 1-hop / 2-hop,这是发现行为) — 推荐,与 Build #11 旧端点行为一致
- B 加 `WikiAclService.ResolveBulk` — Build #19.x 已经做过,但会让 1-hop 命中页被屏蔽时,indirect `via` 字段指向一片 "via (私有页)",UI 体验分裂

**D5 — 间接引用的点击目标**
- **A 点击 indirect 跳到 `via` slug(更近的 1-hop)** — 推荐,符合 "我看的是谁引用了我的引用者" 的语义
- B 点击 indirect 跳到 indirect 自身 — 简单,但用户会跳到与本页面关系更远的页面

**D6 — panel header 数字布局**
- **A 4 个数字 chip 同行**(`直接 12  间接 38  相关 5  失效 2`) — 推荐,信息密度高
- B 4 个数字 + 4 个图标分两行 — 美观但占 vertical space

## 推进建议

拍板后我直接进 Build(同 Build #19 / #19.x 节奏:直分支落盘 → commit → push → reply)。

commit 基线:`lumos0826` HEAD = Build #19 合并点 + Build #19.x uncommitted(本机)。

变更文件清单(预):
- backend:`internal/types/wiki_page.go`、`internal/application/service/wiki_page.go`(新增 `ListBacklinkGraph` + harness)、`internal/handler/wiki_page.go`、`internal/router/routes_knowledge.go`
- frontend:`src/api/wiki/index.ts`、`src/stores/wikiBacklinks.ts`、`src/components/wiki/WikiBacklinksPanel.vue`、`src/views/knowledge/wiki/WikiBrowser.vue`(小改)、`src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}.ts`
- scripts:`scripts/smoke-wiki-backlinks-v2.sh`

回我 "**按推荐走**" 或者对 D1-D6 任意子集做调整即可。