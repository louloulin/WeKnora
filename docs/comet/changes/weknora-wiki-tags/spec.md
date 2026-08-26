# Build #17 — wiki 标签系统 (Wiki Tags) — 完整目标 Spec

## 1. 后端目标

### 1.1 数据模型(migration 000095)

```sql
CREATE TABLE IF NOT EXISTS wiki_tags (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    name VARCHAR(64) NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT 'blue',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (knowledge_base_id, name)
);

CREATE INDEX IF NOT EXISTS idx_wiki_tags_kb ON wiki_tags (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_wiki_tags_sort ON wiki_tags (knowledge_base_id, sort_order, name);

CREATE TABLE IF NOT EXISTS wiki_page_tags (
    wiki_tag_id UUID NOT NULL REFERENCES wiki_tags(id) ON DELETE CASCADE,
    wiki_page_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (wiki_tag_id, wiki_page_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_tags_page ON wiki_page_tags (wiki_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_tags_tag ON wiki_page_tags (wiki_tag_id);
```

Down 文件镜像 reverse。

### 1.2 类型契约(`internal/types/wiki_tag.go`)

```go
type WikiTag struct {
    ID              string    `json:"id"`
    TenantID        uint64    `json:"tenant_id"`
    KnowledgeBaseID string    `json:"knowledge_base_id"`
    Name            string    `json:"name"`
    Color           string    `json:"color"`
    SortOrder       int       `json:"sort_order"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

type WikiTagWithCount struct {
    WikiTag
    PageCount int64 `json:"page_count"`
}

type WikiPageTag struct {
    WikiTagID   string    `json:"wiki_tag_id"`
    WikiPageID  string    `json:"wiki_page_id"`
    CreatedAt   time.Time `json:"created_at"`
}

type WikiTagCreateRequest struct {
    Name  string `json:"name"  binding:"required,min=1,max=64"`
    Color string `json:"color"`
}

type WikiTagUpdateRequest struct {
    Name      *string `json:"name,omitempty"      binding:"omitempty,min=1,max=64"`
    Color     *string `json:"color,omitempty"`
    SortOrder *int    `json:"sort_order,omitempty"`
}

type WikiTagSetPageRequest struct {
    TagIDs []string `json:"tag_ids" binding:"required,max=10,dive,uuid"`
}

type WikiBatchTagBody struct {
    Slugs []string `json:"slugs" binding:"required,min=1,max=200,dive,required"`
    TagID string   `json:"tag_id" binding:"required,uuid"`
    Op    string   `json:"op"     binding:"required,oneof=add remove"`
}
```

### 1.3 Service 接口(`internal/types/interfaces/wiki_tag.go`)

```go
type WikiTagService interface {
    List(ctx context.Context, kbID string) ([]WikiTagWithCount, error)
    Get(ctx context.Context, kbID string, tagID string) (*WikiTag, error)
    Create(ctx context.Context, kbID string, name string, color string) (*WikiTag, error)
    Update(ctx context.Context, kbID string, tagID string, patch WikiTagUpdateRequest) (*WikiTag, error)
    Delete(ctx context.Context, kbID string, tagID string) error
    GetPageTags(ctx context.Context, kbID string, slug string) ([]WikiTag, error)
    SetPageTags(ctx context.Context, kbID string, slug string, tagIDs []string) ([]WikiTag, error)
    BatchTag(ctx context.Context, kbID string, slugs []string, tagID string, op string) (*WikiBatchResponse, error)
}
```

### 1.4 REST 端点 + 路由(`internal/router/routes_knowledge.go`)

```
GET    /wiki/tags                            → WikiTagHandler.List      [KBAccessRead]
POST   /wiki/tags                            → WikiTagHandler.Create    [KBAccessWrite]
PUT    /wiki/tags/:id                        → WikiTagHandler.Update    [KBAccessWrite]
DELETE /wiki/tags/:id                        → WikiTagHandler.Delete    [KBAccessWrite]
GET    /wiki/pages/:slug/tags                → WikiTagHandler.GetPage   [KBAccessRead]
PUT    /wiki/pages/:slug/tags                → WikiTagHandler.SetPage   [KBAccessWrite]
POST   /wiki/pages/batch-tag                 → WikiTagHandler.BatchTag  [KBAccessWrite]
POST   /wiki/pages/batch-preview-tag         → WikiTagHandler.PreviewTag [KBAccessWrite]
```

### 1.5 错误码(对齐 Build #12)

| HTTP | code | 触发 |
|---|---|---|
| 200 | — | 正常 |
| 400 | `tag_name_invalid` | name 为空 / 超过 64 字符 / 仅空白 |
| 400 | `tag_color_invalid` | color 不在 8 色 Palette |
| 400 | `tag_limit_exceeded` | SetPageTags 超过 10 |
| 400 | `tag_op_invalid` | op 不是 'add' / 'remove' |
| 400 | `kb_mismatch` | tag_id 跨 KB |
| 404 | `tag_not_found` | 不存在 |
| 409 | `tag_name_conflict` | 同 KB 同名重复 |

## 2. 前端目标

### 2.1 API client(`frontend/src/api/wiki/tag.ts`)

```typescript
export async function listWikiTags(kbId: string): Promise<WikiTagWithCount[]>
export async function createWikiTag(kbId: string, body: WikiTagCreateRequest): Promise<WikiTag>
export async function updateWikiTag(kbId: string, id: string, patch: WikiTagUpdateRequest): Promise<WikiTag>
export async function deleteWikiTag(kbId: string, id: string): Promise<void>
export async function getWikiPageTags(kbId: string, slug: string): Promise<WikiTag[]>
export async function setWikiPageTags(kbId: string, slug: string, tagIDs: string[]): Promise<WikiTag[]>
export async function batchWikiPagesTag(kbId: string, body: WikiBatchTagBody): Promise<WikiBatchRouteResult>
```

### 2.2 Pinia store(`frontend/src/stores/wikiTags.ts`)

```typescript
export const useWikiTagsStore = defineStore('wikiTags', () => {
  const tags = ref<WikiTagWithCount[]>([])
  const byPageSlug = ref<Map<string, WikiTag[]>>(new Map())
  // actions: fetchAll, createTag, updateTag, deleteTag,
  //          fetchPageTags, setPageTags, batchTag
  // getters: tagsByColor: ComputedRef<Map<color, WikiTagWithCount[]>>
})
```

### 2.3 组件

| 组件 | 路径 | 用途 |
|---|---|---|
| `WikiTagPicker.vue` | `frontend/src/components/wiki/WikiTagPicker.vue` | 多选下拉 + create-on-fly |
| `WikiTagPanel.vue` | `frontend/src/components/wiki/WikiTagPanel.vue` | KB 全局标签面板 + 计数 |
| `WikiTagDialog.vue` | `frontend/src/components/wiki/WikiTagDialog.vue` | 新建/编辑对话框 |
| `WikiTagChip.vue` | `frontend/src/components/wiki/WikiTagChip.vue` | 单 chip 渲染(只读 + hover tooltip) |

### 2.4 集成点

- `WikiBrowser.vue` — 右侧栏 `<WikiTagPanel>` 收起/展开
- `WikiPageView.vue` — 标题下 `<WikiTagChip v-for>`(只读)/ 编辑模式切 Picker
- `WikiBulkActionBar.vue` — 第 4 个按钮 "批量打标签"
- `WikiTagDialog` 在 WikiTagPicker 内部使用(create-on-fly 弹窗)

## 3. i18n(10 keys × 4 locale)

```yaml
wiki.tags:
  namespace:
    title: "标签"           # panel title
    newTag: "新建标签"
    deleteTag: "删除标签"
    deleteConfirm: "删除标签 {name}?使用该标签的 {count} 个页面将取消关联"
    colorPalette:
      blue: "蓝"
      green: "绿"
      orange: "橙"
      red: "红"
      purple: "紫"
      teal: "青"
      gray: "灰"
      gold: "金"
    countLabel: "{count} 页"
    empty: "还没有标签,点击 + 新建一个"
    filterHint: "在搜索框输入 tag:<名称> 按标签筛选页面"
```

```yaml
wikiBrowser:
  bulkTag: "批量打标签"
  bulkTagDialogTitle: "为 {count} 个页面打标签"
  bulkTagOpAdd: "添加"
  bulkTagOpRemove: "移除"
  bulkTagSelectTag: "选择标签"
```

## 4. 验收清单

| # | 类型 | 验收项 | 验证手段 |
|---|---|---|---|
| A1 | 后端 | migration 000095 + down 文件齐全 | `ls migrations/versioned/000095*` |
| A2 | 后端 | `WikiTagService` 8 个方法全实现 | grep `func (s \*WikiTagService)` |
| A3 | 后端 | 8 个 endpoint 全注册 | grep `wikiTagHandler` + `/batch-preview-tag` |
| A4 | 后端 | `SetPageTags` 事务原子 | harness `TestSetPageTags_RollsBackOnMidwayFailure` |
| A5 | 后端 | `Delete` 级联清 wiki_page_tags | harness `TestDelete_CascadesToWikiPageTags` |
| A6 | 后端 | List 返回 page_count 正确 | harness `TestList_ReturnsCorrectPageCount` |
| A7 | 后端 | worker type='tag' 走通 batch | harness `TestBatchTag_RoutesToWorker` |
| A8 | 后端 | 8+ harness 测试 | `go test ./internal/application/service/... -run WikiTag` |
| A9 | 前端 | useWikiTagsStore Pinia store | grep `defineStore.*wikiTags` |
| A10 | 前端 | 4 个组件文件存在 | `ls frontend/src/components/wiki/WikiTag*` |
| A11 | 前端 | WikiBrowser / WikiPageView / WikiBulkActionBar 集成 | grep import statements |
| A12 | 前端 | WikiBulkActionBar 加标签动作 | grep `bulkTag` |
| A13 | i18n | 10 keys × 4 locale | `node scripts/check-i18n-complete.sh` |
| A14 | smoke | smoke-wiki-tags.sh dry-run safe | `bash scripts/smoke-wiki-tags.sh` |
| A15 | verify | vue-tsc + node:test + i18n check | 本地 verify |

## 5. 风险与缓解

- **风险 R1** — `wiki_page_tags.wiki_page_id` 不带 FK 到 `wiki_pages.id`(同 KB slug 是逻辑标识)。若 wiki_page 被删,关联行会成孤儿。**缓解**:在 `DeletePage` 内部加一行 `DELETE FROM wiki_page_tags WHERE wiki_page_id = ?`(服务层兜底,不依赖 DB FK)
- **风险 R2** — `SetPageTags` 传 100 个 tag_id 直接 400(超 10)。**缓解**:前端 Picker max-tag prop 提前 disable 提交按钮
- **风险 R3** — batch-tag 异步 worker 路径下,'remove' op 在 page 没该 tag 时记 ledger failure。**缓解**:op='remove' 的"找不到"不算失败,改成静默跳过(对调用方无感) — D3 落地写入 spec
- **风险 R4** — LUM-18 sibling 仍 running(nestjs-backend,与 WeKnora 仓库无关,无冲突)。**缓解**:无需缓解,本 Build 推进不冲突

## 6. 已知限制

- 本机无 Go binary,`go build ./...` 与 `go test` 不能跑;harness 落地后等用户本机验证
- tag 全文搜索集成留给后续 Build(本 Build 仅前端 UI 接受 `tag:` 前缀,后端过滤未实现)
- 标签颜色 hardcode 8 色,不改样式系统