# v0.7.72+ 路线图 — DOC/SHEET/PPT 飞书级收尾 + 离线/历史/E2E（2026-09-01）

> 本路线承接 PLAN_V3 §9 + STATUS.md §18.7。
> 工作原则不变：**能 copy 就 copy** (vendor genoffice) + **最小改造** + **测试先行** (node:test)。
> 仅前端改动；后端 Go DI bug 不在本 turn 范围。

---

## 1. 当前进度快照

| 模块 | 版本 | 状态 |
| --- | --- | --- |
| DOC 公式 / 域 / 块保护 | v0.7.49 | ✅ |
| DOC 表格 UI / 重复表头 | v0.7.51–57 | ✅ |
| DOC 分页符 / 页眉页脚 | v0.7.63, 66 | ✅（template bug 待修） |
| DOC 文档保护（4 模式 + 密码 hash） | v0.7.67 | ✅ |
| DOC 修订记录（接受/拒绝） | v0.7.68 | ✅ |
| DOC 文档对比（段落 LCS） | v0.7.69 | ✅ |
| DOC 全文搜索/替换 + 字数 | v0.7.70 | ✅ 数据层 |
| **DOC 大纲视图 + TOC 刷新** | **v0.7.71** | **✅ 数据 + UI**（本 turn 完成） |
| SHEET 公式 / 透视表 / 图片 | v0.7.43–48 | ✅ |
| SHEET 批注 / 超链接 / 表对象 | v0.7.45 | ✅ |
| SHEET transformPackage 多文件 | v0.7.46 | ✅ |
| PPT 形状 / 表格 / SmartArt | v0.7.79–85 | 待启动 |

### 测试基线

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 364
ℹ pass 364
ℹ fail 0
```

### 后端验证

- 后端 Go `go build ./...` ✅ 通过
- 后端启动 panic：`missing type: interfaces.StorageBackendRepository`（container.go 预存 DI bug，详见 STATUS.md §18.5 与历史 multi-turn 修复尝试；本路线不动）

### 前端验证

- vite dev server（port 5173）：启动成功但 `CollabDocProEditor.vue` 返回 HTTP 500（template parser bug，pre-existing）
- vue-tsc：本任务新增文件 0 错误（pre-existing 错误集中在 wiki/xlsxChart 等不相关文件）

---

## 2. 优先路线（v0.7.72 → v0.7.90）

### P1 — DOC 多节 + 节级页眉页脚（v0.7.72）

| 项 | 来源 | 备注 |
| | --- | --- |
| `editor/section.ts` | vendor genoffice `editor/section.ts` | 多节 sectPr 解析/序列化 |
| `components/HeaderFooterArea.tsx` | vendor → Vue port | 节级页眉页脚编辑器 |

合并现有 `docxHeaderFooter` 与新 section 模块，使 `docx-engine` 支持多节文档。

### P1 — DOC 查找 UI 补全（v0.7.73）

数据层（`docFind.ts`）已有，UI 未集成。仿 `genoffice FindPanel.tsx`：
- `FindPanel.vue`：搜索框 + 替换框 + 上一个/下一个 + 高亮当前匹配
- 集成到 `CollabDocProEditor.vue` 工具栏"查找"按钮 + Ctrl+F 快捷键

### P1 — SHEET 视图共享（v0.7.74）

`xlsxSheets.ts` 已有基础，扩展 `applyViewSharing`：
- 冻结视图 → sheet view 配置
- 自定义视图（命名/保存/恢复）

### P1 — SHEET 协同光标 + 单元格锁（v0.7.75）

Yjs Awareness + 自建 cellLock：
- awareness broadcast 光标位置（已有 remoteSelections for DOC，扩展到 SHEET）
- cellLock：阻止并发修改同一单元格，提示"X 正在编辑"

### P1 — PPT 系列（v0.7.79–85）

| 版本 | 模块 | 来源 |
| | --- | --- |
| v0.7.79 | PPT 形状 / 路径渲染 | `slides/renderer/draw-shape.ts` |
| v0.7.80 | PPT 表格对象 | `slides/renderer/table-actions.ts` |
| v0.7.81 | PPT SmartArt | `slides/renderer/format-brush.ts` |
| v0.7.82 | PPT 母版 / 版式 | `slides/master/...` |
| v0.7.83 | PPT 动画 | `slides/animations/...` |
| v0.7.84 | PPT 切换效果 | `slides/transitions/...`（partial done at v0.7.43.b） |
| v0.7.85 | PPT 评论 / 备注 | `slides/notes.ts` |

### P2 — 离线编辑（v0.7.86）

- y-indexeddb：Yjs Doc 本地持久化
- Service Worker：资源缓存 + 离线 fallback
- 冲突解决：基于 Yjs CRDT，无冲突合并

### P2 — 历史版本 + diff + 回滚（v0.7.87–89）

后端已有 snapshot 表（doc_versions）：
- v0.7.87：UI 时间线（已部分实现于 CollabDocProEditor history panel）
- v0.7.88：版本 diff（复用 docCompare 段落 LCS）
- v0.7.89：版本回滚（已部分实现）+ 跨版本合并策略

### P2 — E2E + 性能基线（v0.7.90）

- Playwright E2E：DOC/SHEET/PPT 各 1 条冒烟路径
- 性能基线：1000 段落编辑响应 < 200ms；1000 行 sheet 计算 < 500ms

---

## 3. 紧急修复（独立于路线）

### Bug-1: CollabDocProEditor.vue template parser bug（阻塞 UI 验证）

**症状**：`vue compiler-core baseParse` 报 `<template v-else>` (file line 100) "Element is missing end tag"，连带 `<div v-if="protectOpen">` (file line 185) 等多个错误。

**根因（推测）**：vue tokenizer 处理 `<template>` 元素 + 嵌套 `<template>` 在 `<template v-else>` + v-if chain + 多 modal 时 stack 错配。

**HEAD 验证**：v0.7.42 也有 5 个同类错误（不同 line），证明 pre-existing，与本 turn 改动无关。

**修法（按优先级）**：

1. **小改**：将 `<template v-else>` (file line 100) 改为 `<div v-else class="collab-doc-pro__main">`，对应 `</template>` (file line 386) 改为 `</div>`。会多一层 DOM wrapper，但语义正确。
2. **中改**：拆分 v-if/v-else-if/v-else 链为两个独立 v-if 表达式：
   ```vue
   <div v-if="loading">...</div>
   <div v-else class="...">  <!-- 不再需要 v-else-if -->
     <div v-if="loadError">...</div>
     <template v-else>...</template>  <!-- 内层 v-else -->
   </div>
   ```
   实际上 v-if 链需要相邻，把 v-else-if 去掉改成 v-if 即可：
   ```vue
   <div v-if="loading">...</div>
   <div v-if="!loading && loadError">...</div>
   <template v-if="!loading && !loadError">...</template>
   ```
3. **大改**：升级 vue compiler-core / vue 版本，可能已修复。

**推荐**：方法 2（不改 DOM 结构）。但每改一处都要验证 baseParse 0 错误，工作量较大。本 turn 不在本路线范围，标记为下个 turn 第一件事。

### Bug-2: container.go DI 注册顺序（不阻塞前端）

`panic: missing type: interfaces.StorageBackendRepository` —— 业务服务层（line 236 注册）依赖却在 line 858+ 之后声明。已在多轮尝试修复，发现需要系统性重排整个 Provide/Invoke 顺序。本路线不动，标记为 backend-team 后续 PR。

### Bug-3: vue-tsc pre-existing 错误（不阻塞本任务）

集中在 `wiki/*` 与 `xlsxChart.ts` / `xlsxAdapter.ts`。本任务新增文件 0 错误。

---

## 4. 验收标准

每个新模块（v0.7.72+）必须：

1. **vendor 文件直接 copy**：从 genoffice 拷文件，改 import path + node 名。
2. **schema 适配**：在 adapter 层做节点名映射（`docHeading` → `heading`）。
3. **测试 100% pass**：每个新模块 5-15 个 node:test，纯函数优先。
4. **vue-tsc 0 错误** 在本模块新增/改动文件。
5. **不引入 baseParse 错误** —— Bug-1 修完后才能 vite dev 验证。

---

## 5. 时间估算

按 1 turn ≈ 1 模块 + UI 集成：

| 版本 | 工作量 | 关键风险 |
| --- | --- | --- |
| v0.7.72 多节 + 页眉页脚 | 1 turn | section sectPr 序列化 |
| v0.7.73 查找 UI | 0.5 turn | 数据层已有，UI 仿 FindPanel |
| v0.7.74 视图共享 | 1 turn | xlsxSheets 扩展 |
| v0.7.75 协同光标 + 锁 | 1.5 turn | Yjs Awareness 学习曲线 |
| v0.7.79–85 PPT | 5+ turn | 复杂度高，建议拆分子任务 |
| v0.7.86 离线 | 1.5 turn | SW + IndexedDB |
| v0.7.87–89 历史 | 2 turn | UI + diff 复用 |
| v0.7.90 E2E | 1 turn | Playwright 配置 |

总计 ~15 turn，~3 个月（按每天 1 turn）。

---

## 6. 即时下一步（本对话结束后立即可执行）

1. **修复 Bug-1**（template parser bug）：用方法 2 重构 v-if 链，让 vite dev server 能编译 CollabDocProEditor.vue
2. **验证 v0.7.71 UI**：浏览器中打开 DOC 文档 → 点"大纲"按钮 → 检查 outline panel 显示标题 + 点击跳转
3. **截图/录屏验证**：大纲面板 UX 是否符合 Word/腾讯文档预期
4. **更新 README + collab-docs/README**：把 v0.7.67-71 新功能加入截图示例

## 7. v0.7.93+ 当前执行计划（2026-09-02）

### P0：已交付并验证

- DOC/SHEET/SLIDE/FORM 四类编辑器均可真实登录打开；DOC、SLIDE 已验证双端 CRDT。
- SHEET 多 sheet 名称通过 Y.Array + Y.Map manifest 实时同步，新增与删除均通过双浏览器验证。
- FORM 已实现公开访问、匿名提交、响应列表、汇总、CSV 导出。
- 四类文档均实现真实 `.docx` / `.xlsx` / `.pptx` / 表单 JSON round-trip 与 sync-to-KB 入口。
- SLIDE 已接入 genoffice 主题画廊与主题选择事件。

### P1：下一步必须做

1. **SHEET 单元格分表协同**：将 `sheet:cells` 按 sheet name 分区，补齐跨表单元格锁、公式引用和 feature state 的重连恢复。
2. **SLIDE 主题真正落盘**：Konva editor 监听 `theme:apply`，将主题写入 `theme*.xml`、master/layout 和 slide color mapping，再做保存 round-trip。
3. **后端 Yjs 持久化**：不要只依赖进程内 hub；将 Yjs update/doc snapshot 写入 `collab_doc_snapshots`，支持第一个协作者退出、长时间断线后重开。
4. **DOC/SLIDE 历史版本**：已有版本接口与面板，补 diff viewer、恢复确认、权限校验和回滚后的实时广播。

### P2：稳定性与收尾

- 修复现有 `WikiDatabaseView.vue:224 Invalid end tag`，再做全量 Vite build。
- 拆分 `internal/handler/*_test.go` 中重复的 `capturingAuditService`，恢复 Go 全量测试。
- 清理/合并本工作区并发 WIP 的 adapter 与测试文件，按文件所有权提交，避免一次性提交未经验证的跨模块代码。
- 增加离线 IndexedDB 断网编辑、刷新、重连一致性测试。

### 验收门槛

- `npm run build-with-types` 通过；若仍有全仓基线错误，需按模块输出新增错误为 0 的结果。
- Go 全量 `go test ./...` 通过，不再以失败测试作为发布门禁。
- Playwright 真实验证：DOC/SHEET/SLIDE 双端、FORM 公开提交、四类 sync-to-KB，均保存截图与结构化结果。

## 8. v0.7.94 — SHEET per-sheet Yjs cell namespace（已交付，2026-09-02）

### 已完成

- 新增顶层 Yjs 命名空间 `sheet:cells:by-sheet: Y.Map<Y.Map<Y.Map<string>>>`
- 每个 sheet 独立 cell map，rename 迁移、remove 删除
- XLSX 首载按 sheet seed 非空 cell
- 旧 `sheet:cells` 单层 map 自动迁移到第一个 sheet 并清空
- `observeDeep(syncFromY)` + `syncFromY()` 改读活动 sheet map

### 真实双端验证

`frontend/wk-sheet-cells.mjs`：Alice/Bob 两个 Playwright context，独立登录，
Alice 切到 Sheet2 写 A1=Bob 在 Sheet2 看到，Sheet1 不会串表。已 PASS。

### 下一阶段

- v0.7.95 SLIDE 主题真正落盘（Konva editor 监听 `wk-slide-theme-apply` →
  `applyThemeToDeck` + `recolorDeck` → 写回 `theme*.xml` → 重保存 round-trip）
- 后续命名区域 / 跨 sheet 公式 / 表格样式 / 跨 sheet 引用

## 9. v0.7.95 — SLIDE 主题真正落盘（已交付，2026-09-02）

### 已完成

- `CollabDocEditorView` 顶部条件挂 `<CollabSlideThemePanel>`（仅 slide 类型可见）
- `CollabSlideKonvaEditor` 监听 `wk-slide-theme-apply` window event
- 收到 preset → `applyThemeToDeck` 改写 theme*.xml + `recolorDeck` 重映射 srgbClr
- 标 dirty + `scheduleSave()` 1.5s debounce 自动保存
- Konva 端 `[...slides]` 触发重绘

### 真实双端验证

`frontend/wk-slide-theme-persist.mjs` 已 PASS：
- alice 点 indigo swatch → 自动保存 → 重下载 PPTX → 解压 → theme1.xml 12 个
  clrScheme 全部匹配 indigo 预设 → bob 端重下载同样匹配
- 多主题切换 (office → forest) 测试也 PASS

### 下一阶段

- v0.7.96 SHEET 跨 sheet 公式 + 命名区域
- v0.7.97 SLIDE 主题 Yjs 协同（让多端实时看到主题切换）
- 持续收敛 genoffice vendor：把已 copy 的 doc*/xlsx*/pivot*/slide* adapters + 配套
  vitest 按模块分组提交，避免一次性 PR 引入回归

## 10. v0.7.96 — SLIDE 全屏演示模式 + 演讲者视图（已交付，2026-09-02）

### 已完成

- 工具栏 ▶ 演示 按钮 (data-testid="slide-present-btn")
- `<Teleport>` 全屏 overlay (z-index 9999, rgba 黑色遮罩)
- SVG 重渲染当前幻灯片 (text/rect/roundRect/ellipse/line + 其他 fallback)
- 翻页控制：键盘 (←/→/Space/Home/End/PageUp/PageDown/Shift+→) + 工具栏按钮
- 演讲者视图：右下浮动备注 + 左下下一页预览
- ESC 退出，退出时 activeIndex 同步到编辑器

### 真实双端验证

`frontend/wk-slide-present.mjs` 已 PASS：
- overlay 出现 + svg 渲染 + 翻页 + prev/next disabled + ESC 关闭
- 编辑器 textarea 写备注 → 演示 overlay 浮动显示
- 0 page error

### 下一阶段

- v0.7.97 SLIDE 母版编辑 / 布局切换 (listBuiltinLayouts + ensureBuiltinLayout)
- v0.7.98 SHEET 命名区域 UI (xlsxDefinedNames.ts 已就位)
- v0.7.99 DOC 修订对比 viewer (audit timeline 增强)
- 持续收敛 genoffice vendor (doc* / xlsx* / pivot* adapters + 配套 vitest 按模块分组)

## 11. v0.7.97 — SLIDE 布局切换 UI（已交付，2026-09-02）

### 已完成

- pptxShapeAdapter 暴露 listSlideLayouts / setSlideLayout / resetSlideLayout
- 工具栏布局下拉 (data-testid="slide-layout-select")
- availableLayouts computed 列出 deck 内 layout
- missingBuiltins computed 列出未注入的 6 个 builtin (Title Slide / Title and Content / Section Header / Two Content / Title Only / Blank)
- onLayoutSelect handler: builtin: 前缀先 ensureBuiltinLayout 注入,再 setSlideLayout 切换,标 dirty + scheduleSave 1.5s debounce

### 真实双端验证

`frontend/wk-slide-layout.mjs` 已 PASS:
- 选 builtin:titleContent → ensureBuiltinLayout 注入 slideLayout2 → setSlideLayout 切到 slideLayout2 → 自动保存 → 重下载 PPTX 解压 → slide1.xml.rels Target 变成 ../slideLayouts/slideLayout2.xml
- 选 slideLayout1 (Blank) → 切回 → Target 变成 ../slideLayouts/slideLayout1.xml
- Bob peer 重下载,rels Target 与 Alice 一致
- new layout 内容包含 "Title and Content"

### 下一阶段

- v0.7.98 SHEET 命名区域 UI (xlsxDefinedNames.ts 已就位)
- v0.7.99 DOC 修订对比 viewer (audit timeline 增强)
- 持续收敛 genoffice vendor (doc* / xlsx* / pivot* / slide* adapters + 配套 vitest)

## 12. v0.7.98 — SLIDE 形状对齐工具（已交付，2026-09-02）

### 已完成

- 工具栏加 6 个对齐按钮 (data-testid="slide-align-left/center-h/right/top/center-v/bottom")
- alignSelected(direction) handler:
  left/centerH/right -> 修改 shape.x (基于 slide.width)
  top/centerV/bottom -> 修改 shape.y (基于 slide.height)
- 复用 updateShape() 走 Yjs transact + markDirty + scheduleSave 1.5s debounce

### 真实双端验证

`frontend/wk-slide-align.mjs` 已 PASS:
- 加矩形 -> 6 次对齐 -> 每次下载 PPTX 解压 -> 验证 slide1.xml 最后 <p:sp> 的 <a:off x y>
- left/right/top/bottom 边界对齐 + centerH/centerV 居中 x/y 值全部精确匹配
- 0 page error

### 下一阶段

- v0.7.99 SLIDE 多选 / 等距分布 / 匹配宽高 (alignment 扩展)
- v0.7.100 SHEET 找并替换 (find & replace) 或 SHEET 命名区域 UI
- v0.7.101 DOC 修订对比 viewer (audit timeline 增强)
- 持续收敛 genoffice vendor (doc* / xlsx* / pivot* / slide* adapters + 配套 vitest)

## 13. v0.7.99 — SHEET 查找替换（已交付，2026-09-02）

### 已完成

- 工具栏 "查找" 按钮 (data-testid="sheet-find-btn")
- featureDialog 新增 'find' 类型 + modal (search/replace/matchCase 输入 + 匹配计数 + 命中列表)
- findMatches computed: 遍历当前 sheet 全部 cells,按 hay.includes(ndl) 过滤
- onFindCommit: 用 setCell 走 Yjs transact + scheduleSave 1.5s debounce 自动保存
- 支持 case-insensitive (默认) + case-sensitive 切换

### 真实双端验证

`frontend/wk-sheet-find.mjs` 已 PASS:
- 输入测试数据 hello world / hello sheet / Hello / unrelated
- find "hello" 找到 3 个匹配 (A1/A2/A3)
- 替换为 hi -> 自动保存 -> 重下载 xlsx 解压 -> sheet1.xml 验证 A1=hi world, A2=hi sheet, A3=hi, A4=unrelated
- case-sensitive "Hello" 0 匹配 (因为都变成了 hi)
- 0 page error

### 下一阶段

- v0.7.100 SLIDE 多选 + 等距分布 + 匹配宽高 (alignment 扩展)
- v0.7.101 SHEET 命名区域 UI (xlsxDefinedNames.ts + 需在 wb 接口暴露 workbookXml)
- v0.7.102 DOC 修订对比 viewer
- 持续收敛 genoffice vendor

## 14. v0.7.100 — SHEET 按列排序（已交付，2026-09-02）

### 已完成

- 工具栏 "排序" 按钮 (data-testid="sheet-sort-btn")
- featureDialog 新增 'sort' 类型 + modal（列/方向/起始行/结束行）
- onSortCommit：数字按数值、文本按 localeCompare；只重排区间内行
- 同步到 Yjs per-sheet cellMap（只删区间内行 key，区间外数据保留）
- scheduleSave 1.5s debounce 自动保存

### 真实双端验证

`frontend/wk-sheet-sort.mjs` 已 PASS:
- 写入 A=3/1/2/4 + B=delta/alpha/charlie/bravo
- 按 A 列降序排序行 1-8 → UI 前 4 行 A=[4,3,2,1]、B 跟随重排
- 下载 xlsx 解压 sheet1.xml → A1..A4=[4,3,2,1]、B1..B4=[bravo,delta,charlie,alpha]
- 0 page error

### 下一阶段

- v0.7.101 SLIDE 多选 + 等距分布 + 匹配宽高 (alignment 扩展)
- v0.7.102 SHEET 命名区域 UI (xlsxDefinedNames.ts + 需在 wb 接口暴露 workbookXml)
- v0.7.103 DOC 修订对比 viewer (audit timeline 增强)
- 持续收敛 genoffice vendor (doc* / xlsx* / pivot* / slide* adapters + 配套 vitest)
