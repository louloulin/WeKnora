# Build #17 — wiki 标签系统 (Wiki Tags)

## 背景

`P0 全清` 完成后(末班 Build #16 = dry-run preview 已合入 `lumos0826` HEAD `4692d989`)。按选项 A 顺序进 P1。

P1.1 标签系统 = 第一步。理由(已记于 Build #16 reply):
1. 范围最小、最干净 — 一张 `wiki_tags` 表 + 一张 `wiki_page_tags` 关联表 + CRUD + Pinia store + 树视图右侧栏"标签"筛选面板
2. **Build #13 的 `wiki_batch_jobs.type` 枚举已包含 `'tag'`** — 异步 batch 脚手架已经准备好,只缺 endpoint + worker 实现即可复用
3. 不依赖 Build #3 模板抽象(避免现状摸底)

## 范围

| 层 | 内容 |
|---|---|
| **后端 (Go)** | migration 000095(`wiki_tags` + `wiki_page_tags`)+ types / interfaces / repo / service + 6 个 REST endpoint + 1 个 batch endpoint(`/wiki/pages/batch-tag`)+ worker 接 `'tag'` type |
| **前端 (Vue 3 + TDesign)** | API client + Pinia store `useWikiTagsStore` + `WikiTagPicker`(多选 + autocomplete + create-on-fly)+ `WikiTagPanel`(KB 全局标签列表 + 计数)+ `WikiBrowser` 树视图集成 + `WikiPageView` 内容顶部 chip 展示 + `WikiBulkActionBar` 加"标签"批量动作 |
| **i18n** | 10 keys × 4 locale(zh-CN / en-US / ko-KR / ru-RU) |

## 核心设计

### 后端

1. **migration 000095** — 两张表:
   - `wiki_tags`(`id UUID PK` / `tenant_id BIGINT` / `knowledge_base_id UUID` / `name VARCHAR(64)` / `color VARCHAR(16)` / `sort_order INT` / `created_at` / `updated_at`;索引 `(knowledge_base_id, name)` UNIQUE)
   - `wiki_page_tags`(`wiki_tag_id UUID` / `wiki_page_id UUID` / `created_at`;PK = `(wiki_tag_id, wiki_page_id)`;索引 `(wiki_page_id)`)
   - 镜像现有 `KnowledgeTag`(`internal/types/tag.go`)的形状,但 KB 字段换 UUID(wiki 用 UUID),name 限长 64(短标签)
2. **types** — `WikiTag` / `WikiPageTag` / `WikiTagWithCount` / `WikiTagCreateRequest` / `WikiTagUpdateRequest` / `WikiTagSetPageRequest` / `WikiBatchTagBody`
3. **`WikiTagService` 接口**:
   - `List(ctx, kbID)` — 返回 tags 列表 + 每 tag 的 `page_count`(LEFT JOIN wiki_page_tags GROUP BY)
   - `Create(ctx, kbID, name, color)` — 409 if 同名重复
   - `Update(ctx, kbID, tagID, name?, color?, sortOrder?)`
   - `Delete(ctx, kbID, tagID)` — 同步删 `wiki_page_tags` 关联行(无 cascade)
   - `GetByPageID(ctx, kbID, slug)` — 单页标签
   - `SetPageTags(ctx, kbID, slug, tagIDs[])` — 事务内 `DELETE FROM wiki_page_tags WHERE wiki_page_id = ?` 然后批量 insert
4. **REST 路由**(`internal/router/routes_knowledge.go` 注册):
   - `GET /wiki/tags` — 列表(带 count)
   - `POST /wiki/tags` — 创建
   - `PUT /wiki/tags/:id` — 更新
   - `DELETE /wiki/tags/:id` — 删除
   - `GET /wiki/pages/:slug/tags` — 单页标签
   - `PUT /wiki/pages/:slug/tags` — 单页标签替换(事务)
   - 守卫:`OwnedWikiKBOrAdmin` + `KBAccessRead/Write`
5. **batch-tag 端点**(关键复用点):
   - `POST /wiki/pages/batch-tag` body `{slugs[], tag_id, op: 'add'|'remove'}` — 落到现有 batch infra(type='tag' 已存在)
   - **预览对应** `POST /wiki/pages/batch-preview-tag` — 复用 Build #16 `previewBatchResponse` 模式 + D2-alt 只读
6. **worker** — `wiki_batch_job_service.runSlugs` 已经遍历 `slugs[]` + 调单条方法;新增 `TagPage(slug, tagID, op)` 单条方法,op='add' → insert ignore / 'remove' → delete;复用 Build #15 的节流 + progress + failure ledger
7. **测试**(`internal/application/service/wiki_tag_test.go` — harness):
   - List 默认排序 / Create 同名 409 / Delete 级联清 wiki_page_tags / SetPageTags 原子性 / batch-tag add 全成功 / batch-tag remove 部分失败 / 跨 KB tag_id 拒绝 / ListPageTags 空集 / Update 改 name 后原 page 引用自动同步
   - 边界:tag name trim + 去空 + max 64 char

### 前端

1. **`useWikiTagsStore`**(Pinia):
   - state:`tags: WikiTagWithCount[]`(按 sort_order + name 排序)/ `byPageSlug: Map<slug, WikiTag[]>` cache
   - actions:`fetchAll(kbID)` / `createTag(kbID, name, color)` / `updateTag(kbID, id, patch)` / `deleteTag(kbID, id)` / `getPageTags(kbID, slug)` / `setPageTags(kbID, slug, tagIDs)`
   - getter:`tagsByColor: Map<color, WikiTagWithCount[]>`(面板按颜色分组用)
2. **`WikiTagPicker.vue`**(核心 UI 组件):
   - TSelect + filterable,弹出层列出所有 KB 标签(带 color 圆点 + 名称)
   - 输入时本地过滤;无匹配时底部出现"创建标签 X"按钮(走 `createTag` 然后自动选中新标签)
   - 模式:`single` / `multi`(默认 multi)
3. **`WikiTagPanel.vue`**:
   - KB 维度标签列表,每行:[color dot] [name] [count badge](右侧)
   - 行点击 → 触发 `WikiBrowser` 顶部搜索 query 加 `tag:<tag-name>`(复用 Build #9-A 全文搜索过滤)
   - 顶部"+ 新建标签"按钮 → 弹出 mini-dialog(name + 8 色色板选一)
4. **`WikiBrowser.vue` 集成**:
   - 树视图右侧栏(`slot="aside"` 或 `aside-position`)嵌入 `WikiTagPanel`,默认收起,可点开
   - 顶部搜索框接受 `tag:foo` 前缀过滤(Build #9-A 全文搜索 index 加 tag 维度 — 留给后续 Build,本 Build 范围只在 store / REST)
5. **`WikiPageView.vue`**:
   - 内容顶部(`<h1>` 下方)展示 `WikiTagPicker`(只读模式 — chips 列表 + X 删除按钮);编辑模式 → Picker 可编辑
   - chip 颜色 = `tag.color`;hover 显示 tooltip(tag 创建时间 + 使用次数)
6. **`WikiBulkActionBar.vue`** 加第 4 个动作"标签":
   - 按钮文案 `批量打标签`(仅多选时显示)
   - 点开 → mini-dialog:选已有 tag / 新建 → 选 op(add/remove) → 调用 `batchWikiPagesTag` 走现有 batch 路径(>= 20 走异步 + 进度 toast;< 20 走同步)
   - **不接 Build #16 preview 按钮**(标签批量是高频小操作,预览价值不大)
7. **i18n**(`frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}.ts`):
   - `wiki.tags.namespace.{title, newTag, deleteTag, deleteConfirm, colorPalette.*, countLabel, empty, filterHint}`
   - `wikiBrowser.bulkTag` / `bulkTagDialogTitle` / `bulkTagOpAdd` / `bulkTagOpRemove` / `bulkTagSelectTag`
   - 4 locale × 10 key = 40 条翻译

## 关键决定(D1–D5)

| ID | 决定 | 默认 | 备注 |
|---|---|---|---|
| **D1** | 标签作用域 | **全 KB 共享**(跨 KB 看同一标签名) | 用户在 Build #16 reply "P1.1 或 P1.2" + 此 Build 我推荐 = 全 KB 共享 |
| **D2** | 标签颜色 | **8 色固定调色板**(blue/green/orange/red/purple/teal/gray/gold) | 不开放自定义;Palette hard-code 在前端 store |
| **D3** | batch-tag 阈值 | **复用 `WikiBatchAsyncThreshold=20`** | sync(<20) / async(>=20);直接走 Build #13 脚手架 |
| **D4** | 单页标签上限 | **每页最多 10 个标签** | 防止滥用(超出业务价值);`SetPageTags` 校验 |
| **D5** | 删除标签 | **硬删除 + 级联清 wiki_page_tags** | 不用 soft delete;用户重名新建会拿到新 ID |

## 范围之外(not-in-scope)

- 嵌套标签(parent_tag_id)— 扁平够用,后续再说
- 跨 KB 共享池(全 KB 共享 = 同名同 KB 共享,不是全局共享池)
- 全文搜索 index 加 tag 维度(Build #9-A 之后)
- 标签自动补全(ML 推荐)
- 标签权限(KB 内任何角色都能创建/删除 — 简化)
- 标签重命名后 wiki_page_tags 自动迁移(本期不做 — name 改了 page chip 还是显示新 name,因为 store 实时同步)

## 验收矩阵(15 项)

### 后端 A1–A8

- **A1** migration 000095 + down 文件齐全
- **A2** `WikiTagService` 接口完整(8 个方法)
- **A3** 6 个 REST endpoint + `batch-tag` + `batch-preview-tag` 注册到路由
- **A4** `SetPageTags` 事务原子性(失败回滚)
- **A5** `Delete` 级联清 `wiki_page_tags`
- **A6** List 返回 `page_count` 正确(LEFT JOIN + GROUP BY)
- **A7** worker 接 `type='tag'` 走通 batch 流程(progress + failure ledger)
- **A8** harness 8+ 测试全过(同名 409 / 跨 KB / op='add' ignore / op='remove' 等)

### 前端 A9–A13

- **A9** `useWikiTagsStore` Pinia store 完整
- **A10** `WikiTagPicker` + `WikiTagPanel` + chip 集成
- **A11** `WikiBrowser` 树视图右侧栏 + 顶部搜索接受 `tag:` 前缀(仅 UI 层,后端过滤留给 #9-A 增量)
- **A12** `WikiBulkActionBar` 加标签动作 + sync/async 路由
- **A13** 4 locale × 10 key = 40 条翻译完整

### 通用 A14–A15

- **A14** `scripts/smoke-wiki-tags.sh` dry-run safe
- **A15** `vue-tsc` + `node --test` + `check-i18n` 全过

## 待你拍板的 1 件事

**D6 是否需要"标签创建权限"细分**(默认 A 不分):

- **A 全员可创建 / 删除**(推荐 — P1.1 是组织维度,降低门槛)
- **B 仅 KB 管理员能创建,任何成员可在自己 page 用现有标签**

如果选 B,需要再加一层 ACL check,本 Build 顺延一周。

回我 "D6 用 A" / "D6 用 B" / "按 brief 走(A)" 就开始。其他范围/决定如要调也直说。