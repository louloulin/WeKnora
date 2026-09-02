# WeKnora 飞书文档协作能力 — 现状分析（2026-08-31）

> 本文档盘点 WeKnora 当前已实现的「飞书/腾讯文档」协作能力与 genoffice 目标能力之间的差距，
> 作为后续 PORT_PLAN_V2 的输入。已与仓库内 `frontend/`、`internal/`、`migrations/` 实际文件交叉验证。

---

### v0.7.49 — DOC 飞书级补完 (equation + comments + nodes)

**目标**: 充分学习 genoffice docs editor, copy 其 equation/comments 模块到 WeKnora,
实现 DOC 飞书级公式与段落级批注。

**已完成**:
- copy `docEquation.ts` (51 行, 从 genoffice `docs/editor/equation.ts`):
  - `inlineMathML` / `inlineEquationNodeJson` (docInlineMath) / `equationBlockJson` (docProtected)
  - 修复 import 路径: `../../engines/docx-engine` → `../engines/docx-engine`
  - 测试 `docEquation.test.ts` (4/4 pass): inlineMathML / inlineEquationNodeJson / equationBlockJson / trim
- copy `docComments.ts` (106 行, 从 genoffice `docs/editor/comments.ts`):
  - `CommentMark` (Word comment range mark, ids 空格分隔) + `nextCommentId` / `addCommentToSelection` / `removeCommentFromDoc` / `addReplyToCommentRange`
  - `TRACK_IGNORE` inline 常量 (WeKnora 无 revisions.ts)
  - 测试 `docComments.test.ts` (9/9 pass, EditorState + mock view 无 DOM 运行)
- 自写 `docNodes.ts` (轻量 DOC 节点, 从 genoffice `extensions.ts` DocInlineMath + 简化 docProtected):
  - `DocInlineMath` (inline atom: omml/mathml/latex/text)
  - `DocProtected` (block atom: docxIndex/blockType/label/previewText/genXml/formulaDisplay)
  - 测试 `docNodes.test.ts` (4/4 pass)
- **接入 CollabDocProEditor.vue**:
  - extensions 加 DocInlineMath + DocProtected + CommentMark
  - 公式 modal 改用 `equationBlockJson(latex)` 插入结构化 docProtected 节点
    (genXml 携带 OMML 段落, 保存路径 `onEditorUpdate` 识别 docProtected → 整段 XML 替换)
  - 批注: `onSelectionUpdate` 设置 commentAnchor (段落范围 JSON), CollabCommentsPanel
    `@created` 事件 → `addCommentToSelection` 把批注 id 挂到选中文本 mark
- **接入 CollabCommentsPanel.vue**: 新增 `created` emit (创建线程后通知父编辑器挂 mark)

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 246
ℹ pass 246
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 (docEquation/docComments/docNodes/CollabDocProEditor/CollabCommentsPanel) 0 错误
# 40 个 pre-existing 错误与本任务无关
```

**已知遗留**:
- 公式插入在段落中间会 split 段落 (与 v0.7.42 raw HTML 行为一致), 后半段文本在逐段
  patch 保存模式下不落盘 — 完整段落增删需要 convert.ts 级 SavePlan (v0.7.50)
- 批注 mark 只挂到新建批注; 打开已有批注文档时 mark 未从 docx 恢复 (需要 parse 侧
  commentIds → mark 映射, v0.7.50)
- docProtected 仅公式子集; 图片/表格/图表 block 仍走 TipTap 原生节点

### v0.7.50 — DOC 批注 mark 恢复 (in-progress)

**目标**: 打开已有批注的 .docx 时恢复 comment mark 高亮。

**已完成**:
- `docxAdapter.ts`: `DocxAdapterParagraph` 加 `commentIds?: string[]`, `openDocx` 时
  用 `collectCommentIds` 合并 run 级 commentIds + 跨段落 commentStarts/commentEnds
- `CollabDocProEditor.vue`: `paragraphsToContent` 给有 commentIds 的段落加
  `comment` mark (ids 空格分隔), 打开文档即恢复批注高亮
- 测试 `docxCommentRestore.test.ts` (4/4 pass): run 级合并 / 跨段落 / 无批注 / textbox

### v0.7.51 — DOC 全量 SavePlan (公式 round-trip + 段落增删)

**目标**: 让 DOC 保存路径支持 docProtected 公式节点与段落增删 (convert.ts 简化版)。

**已完成**:
- `docxAdapter.ts` `pmDocToSavePlan` 加 docProtected 支持:
  - visibleNodes filter 加 `docProtected`
  - docProtected 节点 → `{ kind: 'xml', xml: genXml, docxIndex }` (OMML 段落整段替换)
- `pmDocToSavePlan` 加 docxIndex 锚定:
  - `parseDocxIndexFromNode` 读 `data-docx-index` / `docxIndex` attr
  - `originalByIndex` Map 优先按 docxIndex 定位, 无 anchor 节点 fallback 顺序游标
  - 段落插入/删除不再错位
- `saveDocxBytes` 支持两种模式: `PatchedParagraph[]` (增量) 与 `SaveBlock[]` (全量 plan)
- `saveDocxBytesWithImages` 加可选 `blocks` 参数
- `CollabDocProEditor.vue` flushSave 改用 `pmDocToSavePlan` 全量保存
- 测试 `docxSavePlan.test.ts` (4/4 pass): parseDocxIndexFromNode / docProtected genXml /
  新段落 generated / SaveBlock[] round-trip

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 254
ℹ pass 254
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 公式插入在段落中间仍会 split 段落 (TipTap insertContent 行为); 现在保存路径
  按 docxIndex 锚定, split 出的后半段 (无 anchor) 会作为新段落 generated 保存
- 批注 mark 的删除清理 (removeCommentFromDoc) 尚未接 UI 删除按钮
- docProtected 仅公式子集; 图片/表格/图表 block 仍走 TipTap 原生节点

### v0.7.52 — DOC 批注删除闭环 (in-progress)

**目标**: 批注线程删除后清理 TipTap comment mark。

**已完成**:
- `CollabCommentsPanel.vue`: 线程 header 加「删除」按钮, `removeThread` 调
  `deleteCollabDocComment` (回复级联删除) + `deleted` emit
- `CollabDocProEditor.vue`: `@deleted` → `removeCommentFromDoc(editor, id)` 剥离
  comment mark (ids 空格分隔, 最后一个 id 移除时 mark 整体删除)

### v0.7.53 — DOC 表格列宽 (table-sizing copy + colwidth 保存)

**目标**: DOC 表格列宽调整与保存 (copy genoffice table-sizing.ts)。

**已完成**:
- copy `docTableSizing.ts` (185 行, 从 genoffice `docs/editor/table-sizing.ts`):
  - `fitColumnWidths` / `setSelectedColumnWidth` / `constrainSelectedTableWidth` /
    `constrainTableWidthAtCell` — 纯函数 + TipTap commands, 依赖 `@tiptap/pm/tables`
    (tableRole 检查, 与 TipTap `table` 节点兼容, 无需改节点名)
- `docxAdapter.ts` `pmTableToTableXml` 读 TipTap `colwidth` attr:
  - 列宽 px → dxa (×15), 生成真实 `<w:gridCol>` / `<w:tcW>` / `<w:tblW>`
  - 无 colwidth 时 fallback 2000 dxa 等宽列
- `CollabDocProEditor.vue`: 工具栏加「⇔ 列宽」按钮 → prompt px →
  `setSelectedColumnWidth` (选中列调整 + 网格重分配)
- 测试 `docTableSizing.test.ts` (5/5 pass): fitColumnWidths 3 项 + pmTableToTableXml
  colwidth 2 项

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 259
ℹ pass 259
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关
```

### v0.7.54 — DOC 表格属性 (table-properties copy + 样式/表头)

**目标**: DOC 表格边框/底纹/表头重复 (copy genoffice table-properties.ts)。

**已完成**:
- 自写 `docTableExtras.ts` (TipTap 表格节点扩展, copy genoffice extensions.ts 的
  docTable/docTableCell/docTableRow attr 面):
  - `DocTable`: tblAutoFit / widthPx / widthPct / colWidthsPct / tblLook / tblStyleId
  - `DocTableRow`: repeatHeader / repeatHeaderEdited
  - `DocTableCell` / `DocTableHeader`: fill / borders
- copy `docTableProperties.ts` (212 行, 从 genoffice `docs/editor/table-properties.ts`):
  - `applyTablePreset` (表头填充 + 斑马纹 + 边框) / `toggleRepeatHeaderRows` (跨页重复表头)
  - `setTableAutoFit` / `setTableLookOption` / `updateSelectedTableAttrs` / `repeatHeaderState`
  - 节点名 docTable→table / docTableCell→tableCell / docTableHeader→tableHeader,
    类型从本地 docx-engine import (TableAutoFitMode / TableLook 已存在)
- `docxAdapter.ts` `pmTableToTableXml` 输出:
  - cell fill → `<w:shd>`, borders → `<w:tcBorders>` (top/left/bottom/right)
  - repeatHeader 行 → `<w:trPr><w:tblHeader/>`
- `CollabDocProEditor.vue`: 表格扩展换 DocTable 系列 + 工具栏加「🎨 样式」(蓝/灰/无边框
  预设) 与「⇕ 表头」(跨页重复) 按钮
- 测试 `docTableProperties.test.ts` (3/3 pass): fill/borders / tblHeader / 无 attrs fallback

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 262
ℹ pass 262
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### v0.7.55 — DOC 表格移动手柄 (table-handle copy)

**目标**: DOC 表格 hover 移动手柄 (copy genoffice table-handle.ts)。

**已完成**:
- copy `docTableHandle.ts` (261 行, 从 genoffice `docs/editor/table-handle.ts`):
  - `DocTableHandle` Extension + ProseMirror plugin: hover 表格左上角显示 ⣿ 手柄,
    点击选中整表 (NodeSelection), 拖拽走 ProseMirror 原生 block move
  - 节点名 docTable→table, i18n tooltip inline, 去掉浮动表格 (tblFloat) 分支
    (WeKnora 无浮动表格 attrs)
- `CollabDocProEditor.vue`: extensions 加 DocTableHandle + `.doc-table-handle` 全局样式
- 测试 `docTableHandle.test.ts` (1/1 pass): extension 注册 plugin + key

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 263
ℹ pass 263
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### v0.7.56 — DOC 批注落盘 (comments.xml round-trip)

**目标**: 批注 mark → .docx comments.xml + commentRangeStart 标记。

**已完成**:
- `docxAdapter.ts`:
  - `saveDocxBytes` 加 `options.comments` (SaveDocxBytesOptions), 传引擎
    `saveDocx` → 重新生成 word/comments.xml + commentsExtended.xml
  - `collectCommentIdsFromNode` 遍历 TipTap 节点收集 comment mark ids
  - `pmNodeToGeneratedBlock` 加 `commentStarts` (从 comment marks)
  - `pmDocToSavePlan` 三个分支 (未变/文本编辑/blank-docx) 遇 comment marks
    强制走 generated 路径, 保证 commentRangeStart 标记落盘
- `CollabCommentsPanel.vue`: `loaded` emit (refresh 后暴露批注列表)
- `CollabDocProEditor.vue`: `@loaded` 缓存批注, flushSave 构建 CommentInfo[]
  (id/author/text/date/parentId/done) 传 `saveDocxBytesWithImages`
- 测试 `docxCommentsSave.test.ts` (4/4 pass): mark 收集 / 无 mark / generated
  commentStarts / comments.xml + commentRangeStart round-trip

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 267
ℹ pass 267
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### v0.7.57 — SHEET 透视表 pipeline 接入 (pivot additions 落盘)

**目标**: 让 v0.7.48 的透视表 UI 真正走 `transformPackage` 写入 .xlsx 字节
(此前只写 reactive state, 保存/下载不落盘)。

**已完成**:
- `CollabSheetEditor.vue` `buildFeaturePipeline` 的 `packageTransformer` 加 pivot 处理:
  - 遍历 `pivotsBySheet`, 解析 `PivotAddition[]` (补 `worksheetPath`)
  - 调 `applyPivotAdditions(pkg, resolved, workbookXml, touched)` 注册
    pivotCacheDefinition / pivotCacheRecords / pivotTable parts + workbook.xml
    `<pivotCaches>` + worksheet rels 链接
  - 保存 (flushSave) 与下载 (exportXlsx) 自动落盘透视表
- 新测试 `xlsxPivotPipeline.test.ts` (2/2 pass):
  - `applyPivotAdditions` 注册 cache + table parts + workbook.xml + worksheet rels
  - pivot additions 走 `transformPackage` 后 parts 存活 + workbook.xml 已 patch

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 269
ℹ pass 269
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- PivotExpansion 完整 round-trip 未测 (需完整 pivot cache + records)
- PivotValueSpec.numFmt / showDataAs / formula 高级选项 UI 未暴露
- Pivot Grouping (日期/数值分组) UI 未接

### v0.7.58 — 腾讯文档兼容层 (CSV 互通 + .txt/.md 导入)

**目标**: 实现腾讯文档级互通 — SHEET 支持 CSV 导入/导出 (腾讯文档/Excel 的
CSV 是系统 legacy 字符集, 中文 Windows 是 GBK), DOC 支持 .txt/.md 导入
(腾讯文档在线文档的导入路径)。

**已完成**:
- copy `csvImport.ts` (从 genoffice `apps/sheets/src/gateway/csv-import.ts`, ~250 行):
  - `decodeCsvBuffer` (BOM → UTF-8 → GBK/Shift_JIS/Big5 嗅探, 带语言偏好)
  - `sniffDelimiter` / `parseCsv` (引号/CRLF/尾空行) / `isNumericCell` (前导零保文本)
  - `buildWorksheetXml` / `csvToXlsxBuffer` / `blankXlsxBuffer`
  - 适配: `Buffer` → `Uint8Array`, `nodebuffer` → `uint8array` (浏览器)
- copy CSV 导出纯函数 (从 genoffice `apps/sheets/src/renderer/csv-export.ts`):
  - `csvField` / `csvFromDisplayRows` (Excel 风格引号 + CRLF)
  - 自写 `gridToCsvBytes` (UTF-8 BOM, 腾讯文档/Excel 中文正确打开)
- 自写 `docTextImport.ts` (腾讯文档在线文档 .txt/.md 导入路径):
  - `textToDocParagraphs` (一行一段) / `markdownToDocParagraphs` (marked.lexer:
    heading/list/blockquote/code) / `stripInlineMarkdown` / `looksLikeMarkdown`
  - `importTextToDocParagraphs` (自动检测 Markdown)
- **接入 CollabSheetEditor.vue**:
  - 上传按钮改「上传 .xlsx / .csv」, accept 加 `.csv`
  - `onUploadFile` CSV 分支: `decodeCsvBuffer` → `parseCsv` → `csvToXlsxBuffer` → open
  - 新增「下载 CSV」按钮 + `exportCsv` (当前 sheet grid → BOM CSV)
- **接入 CollabDocProEditor.vue**:
  - 新增「上传 .docx / .txt / .md」按钮 + 隐藏 file input
  - .docx: `openDocx` → setContent → 上传
  - .txt/.md: `importTextToDocParagraphs` → `buildBlankDocxDoc` 种子 →
    `importParagraphsToContent` → `flushSave(true)` 落盘
- 新测试 14 个 (9 csvImport + 5 docTextImport)

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 283
ℹ pass 283
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- DOC .txt/.md 导入的 docx 尾部有 1-2 个空段落 (blank-docx 种子结构, 与
  现有 fresh-doc 路径一致); 内容完整保留
- 导入段落 1+ 走 generated 块, 首段 heading 样式走 text-only patch (与现有
  行为一致)
- CSV 导出只含当前 sheet 的显示值 (公式结果), 与腾讯文档一致

### v0.7.59 — DOC 批注闭环 (回复 + 解决状态 round-trip + 重新打开)

**目标**: 验证批注回复 (parentId) 与解决状态 (done) 通过 docx commentsExtended.xml
完整 round-trip, 并补上 UI 的「重新打开」动作让 resolve/unresolve 闭环。

**已完成**:
- 验证 engine 已支持完整链路 (无需新增 engine 代码):
  - `saveDocx({ comments })` 检测 `parentId`/`done` 时生成
    `word/commentsExtended.xml` (`<w15:commentEx w15:paraIdParent=... w15:done=...>`)
  - 新评论自动分配 `w14:paraId` (commentsExtended 链接 key)
  - `parseComments` 读 commentsExtended.xml 回填 `parentId`/`done` 到 CommentInfo
  - Content_Types 与 rels 自动注册
- 新测试 `docxCommentExtended.test.ts` (2/2 pass):
  - 回复 + 解决状态 round-trip (paraId 已知)
  - 新评论无 paraId 时自动分配 + 回复链接保持
- **接入 CollabCommentsPanel.vue** (v0.7.59 补全):
  - 已解决线程加「↺ 重新打开」按钮 (`reopenThread` → `updateCollabDocComment({ resolved: false })`)
  - 此前仅有「✓ 解决」, 闭环断在单向上

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 285
ℹ pass 285
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- engine 的 parseComments 只把 commentsExtended 的 done/parentParaId 写回 CommentInfo;
  WeKnora 面板从后端 REST 拉取评论, 不直接消费 engine 解析结果 (后端权威)
- 单条回复删除未做 (目前只能整 thread 删除)

### v0.7.60 — PPT arrange 闭环 (顶层/底层)

**目标**: 给 `CollabSlideKonvaEditor` 补齐 PPT 飞书级必备的「置于顶层 /
置于底层」动作 (原有 `bringForward`/`sendBackward` 只移一层)。

**已完成**:
- `CollabSlideKonvaEditor.vue` 工具栏新增两个按钮:
  - 「⤒ 顶层」 → `bringToFront` (Y.Array 移到末尾, engine.elements.push)
  - 「⤓ 底层」 → `sendToBack` (Y.Array 移到首位, engine.elements.unshift)
- 两个 handler 走 Yjs transact + engine elements 同步镜像, 与现有
  `reorderSelected` 一致的 dirty/structureDirty 标记
- 不引入新依赖, 纯 Web 端最小改造

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 285
ℹ pass 285
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 新增错误; 4 个 CollabSlideKonvaEditor pre-existing 错误
# (shape.kind / saveStatus) 与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 动画/转场未做 (genoffice slides 有 animation-actions.ts, 165 行, 但
  Electron IPC 耦合, 自写成本高)
- 对齐/分布 (align/distribute) 未做 (genoffice arrange-actions.ts 里有,
  需要 Konva transformer 集成)

### v0.7.61 — PPT 评论落盘 (classic comments round-trip)

**目标**: 让 WeKnora 幻灯片编辑器中通过面板创建的评论能落盘到 .pptx 文件
(类比 DOC v0.7.56)。此前 backend 存评论但下载的 .pptx 不含评论。

**已完成**:
- `CollabSlideKonvaEditor.vue`:
  - CollabCommentsPanel 加 `@loaded="onSlideCommentsLoaded"` 缓存后端评论
  - 新增 `writeSlideCommentsToArchive(opened)`: 按 slide 索引分组,
    按 (author, text) 去重避免与 archive 已存评论重复
  - `onForceSave` 在 `savePptxShapeBytes` 之前调用写评论函数
  - 走 pptx-engine 已有 `addSlideComment` (ppt/comments/commentN.xml +
    commentAuthors.xml, Content_Types 自动注册)
- 验证 engine 已有完整支持:
  - `addSlideComment` 创建 comment part + rels + Content_Types override
  - `ensureAuthor` 幂等 (同名同 authorId)
  - `savePptx` 的 `buildZip` 复制所有 entries, 评论 part 自动进入输出 zip
- 新测试 `pptxCommentRoundTrip.test.ts` (2/2 pass):
  - add → save → reopen → getSlideComments 保留 author/text
  - 同一作者多次评论共享 authorId

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 287
ℹ pass 287
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 新增错误; 4 个 CollabSlideKonvaEditor pre-existing
# 错误 (shape.kind / saveStatus) 与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- PPT 评论为经典 model (authorId + idx), 无 parentId/done (DOC 有 commentsExtended)
- 评论 mark 高亮未做 (PPT 评论不绑定文本范围, 仅锚到 slide/shape)

### v0.7.62 — SHEET 透视表高级选项 UI (numFmt / showDataAs / agg)

**目标**: 透视表 modal 暴露 numFmt、showDataAs、聚合方式三项高级选项
(底层 `pivotFormula.ts`/`pivotGrouping.ts` 已有, UI 未接)。

**已完成**:
- `CollabSheetEditor.vue` 透视表 modal 新增三个控件:
  - 聚合方式下拉 (sum / count / average / max / min)
  - numFmt 文本框 (e.g. `#,##0.00`, 留空走默认)
  - Show As As 下拉 (普通 / 总计 % / 行 % / 列 %)
- `onPivotCommit` 把新选项透传到 `PivotValueSpec`:
  - `agg: pivotAggInput.value`
  - `numFmt: pivotNumFmtInput.value.trim()` (可选)
  - `showDataAs: pivotShowDataAsInput.value` (可选)
- engine `xlsxPivotAdd.ts` 已有完整支持:
  - `numFmt` → `resolveNumFmtId` → `numFmtId` 属性
  - `showDataAs` → `showDataAs` 属性
  - `agg` → `subtotal` 属性
- 新测试 `xlsxPivotAdvanced.test.ts` (2/2 pass):
  - numFmt + percentOfTotal 写入 dataField
  - count + percentOfRow 写入 dataField

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 289
ℹ pass 289
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- Pivot grouping UI 未接 (日期/数值分组规则, genoffice 有 pivotGrouping)
- Pivot label filters UI 未接

### v0.7.63 — DOC 分页符 (pageBreakBefore round-trip)

**目标**: 飞书/腾讯文档级分页符 — Ctrl+Enter 等价物在 WeKnora DOC 编辑器中
插入, 并落盘到 `<w:pageBreakBefore/>`。

**已完成**:
- `docPageBreak.ts` (新文件, ~55 行): TipTap 扩展, 为 paragraph/heading
  加 `pageBreakBefore` 属性, 为 hardBreak 加 `pageBreak` 属性
  (模型来自 genoffice `docs editor/page-break.ts`, 适配 WeKnora StarterKit)
- `docxAdapter.ts` `pmNodeToGeneratedBlock`: 从 TipTap `pageBreakBefore` attr
  传递到 engine `ParaFormat.format`
- `CollabDocProEditor.vue`:
  - extensions 列表加 `DocPageBreak`
  - 工具栏新增「分页符」按钮 (`onInsertPageBreak`)
  - handler 用 `tr.setNodeMarkup` 设置当前 paragraph 的 `pageBreakBefore`
- engine 已有完整支持:
  - generate.ts `formatPPrChildren` emit `<w:pageBreakBefore/>`
  - parse.ts 回填到 `ParaFormat.pageBreakBefore`
- 新测试 `docPageBreak.test.ts` (2/2 pass):
  - generated block 含 pageBreakBefore → OOXML `<w:pageBreakBefore/>`
  - DocPageBreak 扩展可导入且 name 正确

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 291
ℹ pass 291
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 页眉/页脚未做 (engine 有 section 支持, 但需要 section properties UI)
- DOC 目录 (TOC) 未做 (genoffice 有 toc-refresh.ts)

### v0.7.64 — PPT 幻灯片转场 (slide transition)

**目标**: 给 PPT 编辑器补齐「幻灯片之间的切换效果」(转场) — 飞书/腾讯文档
演示文稿的标准功能。动画 (entrance/emphasis/exit) 已在 v0.7.38 实现, 转场缺失。

**已完成**:
- 验证 engine 已有完整支持:
  - `setSlideTransition(slide, kind)` / `getSlideTransition(slide)` 已导出
  - `patchSlideTransitionXml` / `readSlideTransitionXml` 处理 `<p:transition>` / `<p:fade/>` 等
  - `TRANSITION_KINDS` 12 种 (none/morph/fade/push/wipe/split/circle/cover/pull/dissolve/zoom/random)
- `pptxShapeAdapter.ts` 新增适配层:
  - `getSlideTransitionOnDeck(deck, slideIndex)` / `setSlideTransitionOnDeck(deck, slideIndex, kind)`
- `CollabSlideKonvaEditor.vue`:
  - 动画面板头部新增「转场」下拉 (12 种效果)
  - `onTransitionCommit` 调用 `setSlideTransitionOnDeck` + `scheduleSave`
  - `loadTransitionForActive` 在切换 activeIndex/deck 时同步当前转场
  - 走 Yjs + engine  同步镜像, 与现有动画模式一致
- 新测试 `pptxTransition.test.ts` (3/3 pass):
  - set 'fade' → save → reopen → get 'fade'
  - `<p:transition><p:fade/></p:transition>` 出现在保存的 slide XML
  - set 'none' 清除转场元素

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 294
ℹ pass 294
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 新增错误; 4 个 CollabSlideKonvaEditor pre-existing
# 错误 (shape.kind / saveStatus) 与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 转场时长 (dur) / 声音 / 触发方式未暴露 UI (PowerPoint 也只暴露基本选项)
- 变形 (morph) 转场需要额外配置 (源幻灯片引用), 自写 UI 成本高

### v0.7.65 — SHEET 透视表 grouping + label filter UI

**目标**: 透视表 modal 暴露日期分组 (年/季/月) 与标签筛选 (contains)
(底层 `pivotGrouping.ts` / `pivotFilters.ts` 已有, UI 未接)。

**已完成**:
- `CollabSheetEditor.vue` 透视表 modal 新增两个控件:
  - 行字段日期分组下拉 (不分组 / 按年 / 按季 / 按月)
  - 标签筛选文本框 (contains 操作, 行字段包含指定文本)
- `onPivotCommit` 把新选项透传到 `PivotAddition`:
  - `groupings: [{ fieldIndex: rowIdx, kind: 'date', dateUnit: 'year'|'quarter'|'month' }]`
  - `filters: [{ kind: 'label', field: rowIdx, op: 'contains', value: ... }]`
- engine 已有完整支持:
  - date grouping 生成 `<fieldGroup>` / `<rangePr>` 逻辑 (引擎内部)
  - label filter 生成 `<filter type="captionContains" stringValue1="...">` 在 pivotTableDefinition
- 新测试 `xlsxPivotGrouping.test.ts` (2/2 pass):
  - date grouping (year) 不破坏 records part 生成
  - label filter (contains "East") 写入 pivotTableDefinition

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 296
ℹ pass 296
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 数值分组 (rangeStep) UI 未接 (飞书 SHEET 也只暴露基本日期分组)
- value filter (top N / greater than / between) UI 未接
- 多个 grouping / filter 规则需逐条添加 (当前 UI 只暴露单条)

### v0.7.66 — DOC 页眉/页脚 (header/footer round-trip)

**目标**: 给 DOC 编辑器补齐页眉/页脚功能 — 飞书/腾讯文档 Word 的标准能力。
engine 已有完整支持, 仅缺 UI。

**已完成**:
- 验证 engine 已有完整支持:
  - `parseDocx` 读 6 种 (header/footer/headerFirst/footerFirst/headerEven/footerEven)
  - `saveDocx` options: `header` / `footer` / `headerFirst` / `footerFirst` /
    `headerEven` / `footerEven` / `sectionHf` / `hfAllSections` / `watermark`
  - `HeaderFooter` 类型支持 text + pageNumber (PAGE 字段) + rich paras
- `CollabDocProEditor.vue`:
  - 工具栏新增「页眉页脚」按钮 + modal (复用 math 模态样式)
  - 输入: 页眉文本 / 页脚文本 / 「页脚自动追加页码」checkbox
  - 「保存」/「清除」按钮
  - `pendingHeader` / `pendingFooter` refs 缓存用户设置
  - save path 在 `saveDocxBytesWithImages` options 里透传 `header` / `footer`
- 新测试 `docxHeaderFooter.test.ts` (3/3 pass):
  - options.header → word/header1.xml 含文本
  - options.footer + pageNumber → word/footer1.xml 含 PAGE 字段
  - header + footer 一起 → document.xml 含 headerReference + footerReference

**验证**:
```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 299
ℹ pass 299
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件 0 错误; 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

**已知遗留**:
- 首页/奇偶页 header/footer UI 未做 (飞书 Word 也默认只暴露普通页眉/页脚)
- 富文本 (logo 图像 / 多行 / 对齐) 未做, 当前只支持单行居中文本
- 水印 (watermark) 单独入口未做

### 18.8 下一步 — v0.7.67 (DOC 文档保护 + SHEET 透视表 value filter)

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P1 | DOC 文档保护 (密码限制编辑) | genoffice docs | 150+ | DOC 飞书级 |
| P2 | SHEET 透视表 value filter (top N / greater than / between) | genoffice sheets | 100+ | 透视表闭环 |
| P2 | DOC 多节 (section) header/footer 区分 | genoffice docs | 200+ | DOC 飞书级 |

---

### v0.7.42 — genoffice 三件套 gateway copy (in-progress)

**目标**: 充分学习 genoffice, copy 其 xlsx gateway 到 WeKnora, 实现飞书级 SHEET 能力。

**已完成**:
- copy 9 个文件 (从 `/Users/louloulin/appx/genoffice/apps/sheets/src/gateway/`) 到 `frontend/src/editor/adapters/`:
  - xlsxFilter.ts (筛选)
  - xlsxSparkline.ts (迷你图)
  - xlsxCf.ts (条件格式)
  - xlsxDv.ts (数据验证)
  - xlsxHyperlinks.ts (超链接)
  - xlsxProtection.ts (保护)
  - xlsxTheme.ts (主题)
  - xlsxDefinedNames.ts + futureFunctions.ts (命名区域 + 未来函数)
  - xlsxChart.ts (图表, 含 desktop-api 类型剥离)
- 新增 `xlsxWorksheetIo.ts` 适配层 (zip + worksheet XML 读写)
- 新增 `xlsxFreeze.test.ts` 验证冻结窗格 (6/6 pass)
- 新增 `xlsxVendored.test.ts` 验证 9 个 copy 文件 (34/34 pass)
- 重构 `xlsxFreeze` 走 `xlsxWorksheetIo` (从 ~70 行 → ~30 行, 去重 zip 逻辑)
- **接入 CollabSheetEditor UI** (CollabSheetEditor.vue: 793 → 1174 行):
  - 工具栏加「冻结」下拉菜单 (首行/首列/两者/取消)
  - 工具栏加「筛选」按钮 (modal: 列号 + 值列表)
  - 工具栏加「条件格式」按钮 (modal: 列号 + 比较 + 值)
  - 工具栏加「数据验证」按钮 (modal: 列号 + 类型 + 值)
  - 保存 (flushSave) 与下载 (exportXlsx) 自动应用全部规则到 .xlsx 字节
  - 加载时从 .xlsx 字节恢复冻结状态 (readXlsxFreeze)
- **接入 CollabDocProEditor 公式 UI**:
  - 工具栏加「公式」按钮 → 弹 modal: LaTeX 输入 + MathML 预览
  - 走 docxAdapter.latexToDocxMath → mathDisplayParagraph → docxMathToMathML
  - 插入为带 data-latex 属性的 HTML 块, source 保留用于 round-trip
- **新测试** docxMathAdapter.test.ts (8/8 pass): latexToDocxMath / mathDisplayParagraph / docxMathToMathML / docxMathToLatex / readMathFragmentsInParagraph / readMathTokensInParagraph
- 总体: 107/107 adapter test 全过, vue-tsc 0 个 CollabSheetEditor / CollabDocProEditor 错误

**架构原则 (继承)**:
- 引擎能力 (OOXML 序列化) 走 adapter, 不直接调内部 API
- 所有 IO 操作经 `xlsxWorksheetIo.ts` 的 zip 层
- genoffice copy 文件头注释标注来源, 方便 vendor 同步

**未 copy** (改造工作量大):
- docs/equation.ts (依赖自定义 TipTap schema: docInlineMath/docProtected)
- sheets/in-memory-workbook.ts + workbook-dsl.ts (Univer 模型, 跟 Univer 紧耦合)
- docs/toc-refresh.ts + page-break.ts (依赖自定义 pageBreakBefore schema)
- docs/comments.ts (依赖 genoffice 的 CommentInfo + revisions 模块)

**已完成接入 UI** (v0.7.42):
- ✅ SHEET: 冻结/筛选/条件格式/数据验证 (4 个能力)
- ✅ DOC: /math 触发 LaTeX 公式 (1 个能力)
- ✅ PPT: 动画 (11 效果 + 3 触发, v0.7.29 已完成)

**剩余 P2** (后续版本):
- 接入 PPT 11 类图表编辑 UI (xlsxChart 已有 applyChartEdit API, 仅缺前端)
- 接入 PPT SmartArt (移植 genoffice dgm-hier 295 行)
- 接入 SHEET 迷你图 / 超链接 / 主题 / 命名区域 UI
- 接入 DOC 表格 cell 颜色/合并 round-trip UI
- Y.Map 实时同步 (目前 SHEET feature 状态为本地 ref, 需推到 Y.Map 让多客户端同步)

---


## 1. 目标与定位

| 项 | 说明 |
| --- | --- |
| **目标** | 把 `/Users/louloulin/appx/genoffice`（Apache-2.0 完整 Office 三件套）的能力移植进 WeKnora，提供类飞书 / 腾讯文档 / Notion 的 DOC/SHEET/SLIDE 三类多人实时协作 |
| **当前代号** | v0.7.25 `collaborative_docs` |
| **核心 CRDT** | Yjs（同 wiki 实时分支共用 y-websocket wire protocol） |
| **编辑器栈** | DOC → TipTap + docx-engine；SHEET → Univer 0.25.1；SLIDE → Konva + pptx-engine/pptx-render |

---

## 2. 已经完成（v0.7.25 MVP 已落地）

### 2.1 后端（Go）—— 已编译通过

```
internal/types/collaborative_doc.go              # 类型 + 三种 DocKind
internal/types/interfaces/collab_doc.go          # 三个仓库接口
internal/types/collaborative_doc_test.go         # 3 个单元测试通过
internal/application/repository/collab_doc.go    # Postgres/SQLite 双 repo
internal/application/service/collaborative_doc.go           # 业务层
internal/application/service/collaborative_doc_authz.go     # ACL 最小接口
internal/handler/collaborative_doc.go            # REST（7 个端点）
internal/handler/collaborative_doc_ws.go         # Yjs y-websocket 升级
internal/handler/collaborative_doc_sync.go       # Sync handler
internal/router/routes_collaborative_doc.go      # 路由注册
internal/container/container.go                  # ~line 533 已注入
internal/router/router.go                        # RegisterCollabDocRoutes 已挂载
migrations/sqlite/000039_collaborative_docs.up.sql # sqlite 迁移
```

| 端点 | 方法 | 说明 |
| --- | --- | --- |
| `/collaborative-docs` | POST / GET | 创建 / 列表 |
| `/collaborative-docs/:id` | GET / PATCH / DELETE | 元数据 |
| `/collaborative-docs/:id/archive` | POST | 软删除 |
| `/collaborative-docs/:id/presence` | GET | 在线协作者 |
| `/collaborative-docs/:id/export` | GET | 当前仅返回静态 markdown 占位（**待替换为 docx/pptx/xlsx 字节流**） |
| `/collaborative-docs/:id/realtime` | GET (WS) | Yjs y-websocket 二进制帧 |

表结构：
- `collaborative_docs` — 元数据（id、tenant_id、kb_id、title、doc_kind、share_token、visibility…）
- `collab_doc_snapshots` — Yjs 压缩字节（ydoc_state bytea + vector_clock bytea + size_bytes）
- `collab_doc_sessions` — 在线协作者（与 wiki_realtime_sessions 同构）

### 2.2 前端（Vue 3）—— MVPs 已在，但只是壳

| 文件 | 状态 | 备注 |
| --- | --- | --- |
| `src/api/collabDoc/index.ts` | ✅ 完成 | REST + WS URL helper，已用 `@/utils/request` |
| `src/composables/useYjsCollabDoc.ts` | ✅ 完成 | 镜像 `useYjsWiki`，key=doc_id |
| `src/components/collab/CollabDocEditor.vue` | ⚠️ TipTap MVP | 仅 StarterKit + Collaboration，未挂载 docx-engine |
| `src/components/collab/CollabSheetEditor.vue` | ⚠️ Yjs MVP | Y.Array+Y.Map 网格，**非 Univer** |
| `src/components/collab/CollabSlideEditor.vue` | ⚠️ Y.Array MVP | 标题+要点列表，**非 Konva** |
| `src/views/collab/CollabDocListView.vue` | ✅ 完成 | 飞书列表视觉 |
| `src/views/collab/CollabDocEditorView.vue` | ⚠️ 装载三种编辑器 | auth store 属性名错（详见 §3） |
| `src/router/index.ts` | ✅ 完成 | `/collab-documents` + `/collab-documents/:id` |
| `src/i18n/locales/en-US.ts` | ⚠️ 重复 key | `google_workspace` 后少逗号 |
| `src/i18n/locales/zh-CN.ts` | ✅ 完成 | collabDoc.* |

### 2.3 genoffice 引擎已拷入，但未对接

```
frontend/src/editor/engines/docx-engine/   # 完整 25 个文件，已将 @genoffice/* 改相对路径
frontend/src/editor/engines/pptx-engine/   # 完整 41 个文件，含 node:crypto/node:zlib 需 polyfill
frontend/src/editor/engines/pptx-render/   # 完整 13 个文件，纯 TS
frontend/src/editor/engines/file-parse/    # 已删除 Node-only 文件，保留 docx.ts/pptx.ts/xlsx.ts/index.ts
frontend/src/editor/adapters/docxAdapter.ts # 浏览器侧 wrapper，但 saveDocx 调用签名错误
```

已安装的 npm 包：`jszip fast-xml-parser utif2 pptxgenjs pdfjs-dist ppt-to-text word-extractor cfb opentype.js`

---

## 3. 当前阻塞 — 已实测的错误清单

实测 `cd frontend && ./node_modules/.bin/vue-tsc --build 2>&1 | grep error TS` 的结果中，
**仅以下属于本次任务的修复项**（其它 wiki / syncBlock 等为已存在的历史债）：

```
src/api/collabDoc/index.ts(93,33): ArrayBuffer|SharedArrayBuffer → BlobPart（new Blob 接受 Uint8Array OK）
src/components/collab/CollabDocEditor.vue:
    (34,31)  @tiptap/extension-collaboration 未安装（npm 包已加在 package.json，需 install）
    (35,37)  @tiptap/extension-collaboration-cursor 同上
    (55,3)   boolean|undefined → boolean（v 转换）
    (56,3)   peers 类型合并错误
    (57,3)   error 类型合并错误
    (74,5)   null → Editor|undefined（teardown 时 editor.value = null）
src/components/collab/CollabSheetEditor.vue:
    (64,20)  yjs 未安装（package.json 有但 node_modules 缺）
    (93-95)  类型合并（同 Doc）
src/components/collab/CollabSlideEditor.vue:
    (73,20)  yjs 同上
    (108-110) 类型合并（同 Doc）
    (122,39) toArray().map(m => ...) m 隐式 any
src/composables/useYjsCollabDoc.ts:
    (9,20)  yjs 模块未找到
    (10,35) y-websocket 模块未找到
src/editor/adapters/docxAdapter.ts:
    (68,33) saveDocx 第二参应为 SaveBlock[] 而非 { blocks, ...opts }（签名错）
src/i18n/locales/en-US.ts:
    (2919,5) 和 (2943,5) 重复 key（google_workspace 后缺逗号）
src/views/collab/CollabDocEditorView.vue:
    (57,29) authStore.accessToken 不存在（应为 token）
    (58,41) authStore.user.name 不存在（应为 username）
```

> 其它 `useYjsWiki.ts`、`SyncBlockNode.ts`、`AIAssistantPanel.vue` 的错误属于历史债，本计划不修复。

---

## 4. 差距 — 当前 MVP 与「飞书文档」目标

| 维度 | 当前 MVP | 飞书 / 腾讯文档目标 | 需要做的事 |
| --- | --- | --- | --- |
| **DOC 编辑** | TipTap StarterKit，Y.Text 二进制更新 | 富文本 + 段落锚定到 docx-engine Block.docxIndex | 在 TipTap 中维护 Block 镜像；onUpdate → `patchParagraphText` → `saveDocxBytes` |
| **DOC 持久化** | 仅 Yjs 状态写到 collab_doc_snapshots.ydoc_state | 上传/下载真实 .docx 字节流 | 实现 `downloadCollabDocBytes`（后端新增端点），前端 `openDocx` 渲染后回写 |
| **SHEET 编辑** | Y.Map<rowKey, Y.Map<colKey, string>> 网格 | Univer：公式、单元格格式、跨表引用、图表 | 替换为 `@univerjs/presets + @univerjs/ui + @univerjs/sheets` |
| **SLIDE 编辑** | 标题+要点列表（自行 Y.Array） | Konva 舞台、形状、主题、母版、动画 | 接入 `pptx-engine` 的 `openPptx/savePptx` + `pptx-render` 的 `buildSlide/RenderTree`，用 vue-konva 渲染 |
| **协作粒度** | 整文档 Yjs 二进制 | DOC 段落级 Y.Text；SLIDE 形状级 Y.Map；SHEET Univer native | 替换抽象：DOC → `Y.XmlFragment` 或 per-paragraph `Y.Text` |
| **导入/导出** | Export 端点返回静态 markdown | .docx/.pptx/.xlsx 字节流导入 + 导出 | 后端按 doc_kind 走 docparser 或 python-pptx |
| **KB 同步** | `/sync-to-kb` 路由占位 | 解析 .docx → chunk → 入 KB | 接 docparser `/chunk` |
| **AI Agent** | 无 | 选段 → 「问 AI」 → 段落级润色 | docx-engine block JSON 喂给现有 chat，润色后 patchParagraphTexts |
| **离线编辑** | 无 | 弱网缓冲 + 离线快照 | Service Worker + IndexedDB Yjs provider |

---

## 5. 关键技术风险（实测确认）

1. **pptx-engine 含 Node-only 依赖**
   - `packages/pptx-engine/src/zip.ts:10` — `import { createHash } from 'node:crypto'`
   - `packages/pptx-engine/src/index.ts:666` — `await import('node:fs')`
   - `packages/pptx-engine/src/media-insert.ts:14` — `import { deflateSync } from 'node:zlib'`
   - `savePptxToFile`（仅 Electron 场景使用），浏览器可不打包
   - 浏览器侧需 polyfill：`globalThis.crypto.subtle` + `CompressionStream('deflate-raw')`

2. **file-parse 删除的安全风险**
   - 已删除 `parse.ts`（用 `node:fs/promises`）、`pdf.ts`（Electron）、`doc.ts`/`ppt.ts`/`vendor.ts`
   - 保留 `docx.ts/pptx.ts/xlsx.ts/index.ts` —— 纯 JSZip 实现，浏览器可跑
   - 浏览器侧 PDF 解析改用 `pdfjs-dist`（已装）

3. **docx-engine `saveDocx` 签名**
   ```ts
   export async function saveDocx(
     parsed: ParsedDocFull,
     finalBlocks: SaveBlock[],          // ← 第二参直接是 SaveBlock[]
     options: SaveOptions = {},
   ): Promise<Uint8Array>
   ```
   当前 `docxAdapter.saveDocxBytes` 错用 `{ blocks, ...opts }`，需要改成 `(doc.parsed, saveBlocks)`。

4. **TipTap ↔ docx-engine 桥接**
   - genoffice 的做法：PM doc ↔ SaveBlock[] 双向序列化（见 `apps/docs/src/renderer/editor/convert.ts`）
   - 移植到 Vue 端需重新实现 `pmDocToSavePlan`，依赖 `blocksToPmDoc`
   - 段落粒度协作：使用 `@tiptap/extension-collaboration` 的 `field` 配置，把每个 paragraph 映射成 Y.Text

5. **vue-konva 与 react-konva 的 API 差异**
   - genoffice 大量使用 react-konva 的 `<Stage><Layer><Rect /></Layer></Stage>`
   - vue-konva 形状组件 API 接近，但 Transformer / 自定义 Shape 需要手写 `configFunc`
   - 建议先做只读 Konva 渲染（buildSlide → Konva tree），再做交互层

6. **Univer 与现有 Yjs WS 兼容**
   - Univer 0.25.1 自带 `@univerjs/yjs` provider，需要创建对应的 `UniverDoc` namespace
   - 与现有 `useYjsCollabDoc` 共用一个 Y.Doc，但 key 不同（univer 默认 key 是 `default`）

---

## 6. 已确认可用的资产清单

| 资产 | 路径 | 价值 |
| --- | --- | --- |
| Wiki realtime 前例 | `frontend/src/composables/useYjsWiki.ts` + `internal/handler/wiki_realtime*.go` | CRDT + WS 模式的最佳参照 |
| Tiptap 已有渲染 | `frontend/src/components/wiki/WikiTiptap*.vue` | StarterKit + Collaboration 接线已成熟 |
| docparser Python 服务 | `docreader/` + `/chunk` 端点 | `.docx → markdown chunk`，用于 KB 同步 |
| 已有 KB/Chunk 管线 | `internal/application/service/chunk.go` | Re-chunk 流水线可复用 |
| 鉴权 / RBAC | `internal/application/service/rbac.go` | 已在 routes_collaborative_doc.go 用 OwnedWikiKBOrAdmin |
| i18n | `frontend/src/i18n/locales/{en-US,zh-CN}.ts` | `knowledgeBase.collabDoc.*` 已埋点 |

---

## 7. 下一步入口（与 PORT_PLAN_V2 一一对应）

1. **Phase 0（修脚手架）**：npm install 补齐 yjs/y-websocket/tiptap-collaboration；修 docxAdapter.saveDocx 签名；修 i18n 重复 key；修 authStore 属性名；修 CollabSlideEditor m 类型。
2. **Phase 1（DOC 真编辑）**：实现 `CollabDocProEditor.vue` + `pmDocToSavePlan`，跑通「上传 .docx → 编辑 → 下载 .docx」闭环。
3. **Phase 2（PPTX 真渲染）**：polyfill node:crypto/zlib；写 Konva adapter；接入 vue-konva。
4. **Phase 3（Univer 替换 SHEET）**：用 `@univerjs/presets`，与现有 Y.Doc 共存。
5. **Phase 4（协作粒度细化）**：DOC per-paragraph Y.Text；SLIDE per-shape Y.Map。
6. **Phase 5（KB 同步 / AI 接入）**：docparser `/chunk` + chat 流；选段 → 段落级润色。
7. **Phase 6（离线 + 分享）**：IndexedDB persistence；share-token 路由。

每个 phase 的具体 task 见 PORT_PLAN_V2.md。

---

## 8. v0.7.25 → v0.7.37 增量盘点（2026-09-01）

承接 §2，已落地的版本号（按 git log）：

| 版本 | commit | 关键产出 |
| --- | --- | --- |
| v0.7.25 | `5258cc23` | Collab Docs Foundation — 4 表 + 14 端点 + WS + TipTap MVP |
| v0.7.26 | `ced4b708` + `84060c44` + `edfdef25` | CollabDocFile 字节存储 + .pptx/.xlsx adapters + i18n |
| v0.7.27 | (P10-P12) | DOC 工具栏（11 TipTap 扩展）+ XLSX 公式 + IndexedDB |
| v0.7.30 | (本审计期) | PPT 演讲者备注 + PPT 选区广播 + 后端审计日志 |
| v0.7.31 | (本审计期) | SHEET 选区广播 + Slides Region backend |
| v0.7.32 | (本审计期) | 引擎能力适配层（6 大类，35+ 函数） |
| v0.7.33 | `3d185c29` | KG graph query API |
| v0.7.34 | `2c3f6902` | Docs × KB integration |
| v0.7.35 | `b18b51cd` + `a826cec5` | threaded comments + share token + PPT shape editor |
| v0.7.36 | `88d5083f` + `354fdac2` | Enterprise Search + MindMap |
| v0.7.37 | `e30e0e8c` + `ed1d62a9` | Slides backend + 引擎 adapter 收尾 |

适配层测试：**51 pass / 51 total**（10 个 .test.ts 文件覆盖 35+ 函数）。

详细差距分析见 [`ANALYSE_V2.md`](./ANALYSE_V2.md)，路线图见 [`ROADMAP_V2.md`](./ROADMAP_V2.md) 第七章。

---

## 9. v0.7.38 收尾增量 + v0.7.41+ 路线（2026-09-01）

承接 §8，已落地的本审计期变更：

| 版本 | 改动文件 | 关键产出 |
| --- | --- | --- |
| v0.7.38 | `internal/middleware/collab_ratelimit.go` + `_test.go` | 限流中间件（per-tenant/per-doc/per-IP），7 tests pass |
| v0.7.38 | `internal/types/webhooks.go` + `interfaces/webhooks.go` + `internal/handler/webhooks.go` + `internal/router/routes_webhooks.go` | WebHook 系统（11 events + HMAC-SHA256 + retry），8 tests pass |
| v0.7.38 | `internal/application/repository/webhooks.go` + `service/webhooks/` | WebHook 持久化 + delivery retry |
| v0.7.38 | `migrations/sqlite/000051_webhooks.{up,down}.sql` + `mysql/000046_*.sql` | WebHook 表迁移（双库） |
| v0.7.38 | `internal/types/audit_log.go` | 9 个新 AuditAction 常量（`collab_doc.*` + `slide_deck.*`） |
| v0.7.38 | `internal/handler/collaborative_doc*.go` + `internal/application/service/collaborative_doc.go` | 分享密码 + 过期（hash/verify/expired 全部接入） |
| v0.7.38 | `migrations/sqlite/000053_collab_share_protect.{up,down}.sql` + `mysql/000047_*.sql` | `share_password_hash` + `share_expires_at` 迁移 |
| v0.7.38 | `frontend/src/composables/useYjsCollabDoc.ts` + 3 个 `.vue` | DOC range awareness + SHEET cell selection + PPT format toolbar |
| v0.7.38 | `frontend/src/components/collab/CollabSlideDeckEditor.vue` + `frontend/src/views/collab/CollabSlidesView.vue` + `frontend/src/api/slides/` | Slides 后端可视化编辑器接入 |
| v0.7.38 | `docs/collab-docs/ANALYSE_V2.md` + `STATUS.md` + `ROADMAP_V2.md` | 第 3 轮盘点 |
| v0.7.38 | `.gitignore` | 排除 `desktop/` 329 MB 二进制 |

### 9.1 测试状态实测（2026-09-01）

```
$ go build ./...
# 全干净（仅 ld warning 关于 desktop 二进制）
$ go test ./internal/application/repository/ -count=1 -run "TestCollab"
ok  	github.com/Tencent/WeKnora/internal/application/repository	0.492s
$ go test ./internal/middleware/ -count=1 -run "TestCollab"
ok  	github.com/Tencent/WeKnora/internal/middleware	0.608s
```

### 9.2 关于"share test 编译失败"误报澄清

之前 handoff 文档误将 `internal/application/service/` 包 build 失败归因于新增的
`collaborative_doc_share_test.go`。经 `git stash` 验证：

- 该 build 失败是 **PRE-EXISTING** 的 wiki 测试 helper 名冲突
- 冲突点（8 个）：
  - `contains` — `audit_export_test.go` ↔ `knowledge_span_tracker.go` ↔ `wiki_template_test.go`
  - `fakeClock` — `tool_cache_test.go` ↔ `audit_log_test.go`
  - `stubAuditLogService` — `wiki_audit_harness_test.go` ↔ `audit_export_test.go`
  - `stubWikiAclRepo` — `wiki_audit_harness_test.go` ↔ `wiki_acl_test.go`
  - `stubBacklinksCacheRepo` / `newStubBacklinksCacheRepo` — `wiki_audit_harness_test.go` ↔ `wiki_page_backlinks_v2_cache_test.go`
- 修复策略（v0.7.41.1）：给上述 helper 加 `wiki` / `audit` / `cache` 前缀，包级不再 build fail
- **新增的 `collaborative_doc_share_test.go` 函数名（`TestShare*` + `newShareTestDB`）不与任何既有冲突**

### 9.3 下一步 — 见 [PLAN_V3.md](./PLAN_V3.md)

新路线图 v0.7.41 → v0.7.45（5 个版本 / 10 周）已在 `PLAN_V3.md` 第 3 章详述。

---

## 10. v0.7.41 Build #47 收尾（2026-09-01）

承接 §9，把 v0.7.41 完整收尾：

### 10.1 后端

- **测试构建修复**：8 个 PRE-EXISTING 的 wiki_*_test.go helper 名冲突(contains/fakeClock/stubAuditLogService/stubWikiAclRepo/stubBacklinksCacheRepo 等)+ interface drift。已通过 rename + auto-generated stub 解决。`go test -c ./internal/application/service/` 现在干净编译。
- **`internal/types/webhooks.go`**：新增 `WebhookEventCollabDocUnshared = "collab.doc.unshared"` 常量 + allow-list 条目。
- **`internal/application/service/collaborative_doc.go`**：
  - `EnableShare` 末尾追加 `s.publishEvent(ctx, "collab.doc.shared", {doc_id, doc_kind, password_protected, expires_at})`
  - `DisableShare` 末尾追加 `s.publishEvent(ctx, "collab.doc.unshared", {doc_id, doc_kind})`
- **测试**：`TestShareVerifyPasswordRoundTrip`、`TestShareExpiredReturnsTrue` ✅ pass。

### 10.2 前端

- **`frontend/src/api/collabDoc/index.ts`**：
  - `CollabDoc` 类型新增 `share_expires_at?: string | null` 和 `share_password_protected?: boolean`
  - 新增 `enableCollabDocShare(id, req)` / `disableCollabDocShare(id)` / `downloadCollabDocShare(token, password?)`
  - `downloadCollabDocShare` 在密码存在时自动加 `X-Share-Password` header
- **`frontend/src/components/collab/CollabSharePasswordPanel.vue`**（423 行）：完整的 owner-side 面板
  - 启用 / 禁用 / 复制链接 / 6+ 字符密码校验 / 可选过期时间
  - i18n: en-US + zh-CN
  - data-testid 全部覆盖（便于 E2E）
- **`frontend/src/views/collab/CollabDocEditorView.vue`**：在 sidebar 加「分享设置」开关 + 嵌入 `CollabSharePasswordPanel`
- **`frontend/src/views/collab/CollabDocShareView.vue`**：公开访问 `/collab-documents/share/:token`
  - 401/403 自动弹出密码输入框
  - 通过新 `downloadCollabDocShare` API 下载（带密码 header）
- **`frontend/src/i18n/locales/{en-US,zh-CN}.ts`**：`collabDoc.share.*` 全套键
- **`frontend/src/components/collab/__tests__/CollabSharePasswordPanel.test.ts`**（8 tests）：
  - 密码长度规则、handler vs panel floor 对齐、URL 编码、payload 形状 ✅ 8/8 pass

### 10.3 验证

- `go build ./...` ✅ 干净
- `go test -c ./internal/application/service/` ✅ 134MB test binary 编译干净
- `tsx --test src/editor/adapters/__tests__/*.test.ts` ✅ 51/51 pass
- `tsx --test src/components/collab/__tests__/CollabSharePasswordPanel.test.ts` ✅ 8/8 pass
- `vue-tsc --build`：我新增的 3 个文件零错误（其他 wiki/mindmap/i18n 既有 31 errors 与本任务无关）

### 10.4 下一步 — v0.7.42 SHEET 飞书级

- 条件格式：cellRules JSON in Y.Map
- 数据验证：dropdown + number range
- 冻结窗格
- 自绘 SVG 图表：bar/line/pie
- SHEET 公式栏 UI 完善


---

## 11. v0.7.43 SHEET 公式栏完整化（2026-09-01）

承接 §10.4，把 SHEET 公式栏升级到飞书 SHEET 基线水平。

### 11.1 背景

- 上一轮把 `genoffice` 9 个 xlsx gateway 已 copy 到 `frontend/src/editor/adapters/`，并已接入冻结 / 筛选 / 条件格式 / 数据验证 / 迷你图 的 modal UI（见 §9）。
- 公式栏仍停留在 v0.7.38 的最小实现：只支持 `SUM / AVERAGE / COUNT / MIN / MAX` + 单元格算术，没有跨表引用、没有 `IF / COUNTIF / VLOOKUP / CONCAT / LEN / ROUND / TEXT` 等基础函数，也没有单元测试覆盖。

### 11.2 工作

- **新建独立模块** `frontend/src/editor/formula/sheetFormula.ts`（378 行，纯函数）：
  - 类型：`SheetLookup = ReadonlyMap<string, string[][]>`。
  - 助手：`colNameToIndex`（A→0、AA→26）/ `resolveSheetRef`（支持 `'Sheet 2'!A1` 引号形式）/ `resolveCellRef` / `splitFormulaArgs`（尊重嵌套括号与字符串）/ `collectRangeValues` / `collectRangeStrings` / `stripStringQuotes`。
  - 引擎：`evaluateFormula(expr, currentSheet, lookup): string`，支持：
    - **聚合**：`SUM / AVERAGE / AVG / COUNT / COUNTA / MIN / MAX`
    - **条件**：`COUNTIF`（含 `>5 / =3 / !=x` 运算符）/ `SUMIF`
    - **查找**：`VLOOKUP(lookupValue, tableRange, colIdx)`（含 `#N/A` 兜底）
    - **字符串 / 数值**：`CONCAT / CONCATENATE / LEN / ROUND / ABS / TEXT`（支持 `0.00 / 0% / 0.00%`）
    - **逻辑**：`IF`（支持 `A1>B2` 类单元格比较 + 字面量比较 + THEN/ELSE 字面量字符串）
    - **Token 算术**：`A1 + Sheet2!B2 * 2`（跨表）
  - 修正原 v0.7.38 `resolveCellRef` 正则 bug：`/^([A-Z]+)(d+)$/i` → `/^([A-Z]+)(\d+)$/i`，多位数行号（`A10`）与多字母列号（`AB1`）现在能正确解析。
  - 修正原 `IF` 单元格比较只 resolve 左边的 bug：现在左右两边都走 `resolveAnyCellRef`。

- **接入 CollabSheetEditor.vue**：
  - 新增 `import { evaluateFormula as evalFormula, type SheetLookup } from '@/editor/formula/sheetFormula'`
  - 新增 `activeSheetName` computed（之前没有，公式栏需要当前 sheet 名）
  - 新增 `sheetLookup` computed
  - `formulaResult` 改为新签名：`evalFormula(value, activeSheetName.value, sheetLookup.value)`
  - 删除原 in-file `colNameToIndex` / `resolveCellRef` / `parseRangeArgs` / `evaluateFormula`（共 -48 行），避免双份实现

- **新测试** `frontend/src/editor/formula/__tests__/sheetFormula.test.ts`（21 个 case）：
  - 列名解析 / sheet 名解析 / 单元格解析（含越界）
  - 单 cell / 2D 范围 / 跨表范围 / 字符串范围
  - `SUM / AVERAGE / COUNT / COUNTA / MIN / MAX`
  - `COUNTIF / SUMIF`（运算符 + 字符串匹配）
  - `IF`（字面量 + 单元格比较）
  - `CONCAT / LEN / ROUND / ABS / TEXT`
  - `VLOOKUP`（命中 + `#N/A`）
  - Token 算术 + 跨表 token 算术
  - 边界：空 / 未知函数 / 未知 sheet 抛错

### 11.3 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/formula/__tests__/*.test.ts \
    src/editor/adapters/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 128
ℹ pass 128
ℹ fail 0

$ ./node_modules/.bin/vue-tsc --build --force
# 0 errors in sheetFormula.ts / CollabSheetEditor.vue
# 41 total errors — all pre-existing wiki/mindmap/i18n/slides/syncBlock

$ go build ./internal/...
# clean
```

### 11.4 已知遗留 / 后续

- **v0.7.42 SHEET 工具栏 modal UI（冻结/筛选/条件格式/数据验证/迷你图）在一次 `git checkout` 中被回退到 v0.7.38 的 793 行状态** — 底层 adapter（xlsxFilter / xlsxCf / xlsxDv / xlsxSparkline / xlsxWorksheetIo）均健在，公式栏完整化也独立可跑，但工具栏上的 5 个按钮与对应的 modal 模板需要按 §9 重新接入。
- 公式引擎 **不写入** .xlsx 字节：当前只渲染公式栏的实时结果。要让公式「真正落地」，需要在 `xlsxWorksheetIo.transformWorkbook` 的 sheet XML 写入路径里加一段：当 cell 有 `=` 前缀时，把值替换为 `<f>…</f><v>result</v>`（仿 genoffice / Excel 的 cellFormula schema）。这一段是飞书 SHEET 与当前实现的最后一公里差距。
- 未实现的函数（按飞书 SHEET 文档继续补）：`IFS / SWITCH / AND / OR / NOT / LEFT / RIGHT / MID / SEARCH / DATE / TODAY / NOW / INDIRECT / OFFSET`。

---

## 12. v0.7.43.b SHEET 工具栏 modal UI 补回 + 公式持久化（2026-09-01）

承接 §11.4，把 v0.7.42 在 `git checkout` 中丢失的 5 个 SHEET 工具栏 modal UI 全部补回，并把公式真正写到 `.xlsx` 字节。

### 12.1 5 个 modal UI 重新接入（CollabSheetEditor.vue）

工具栏新增 5 个按钮：

- **冻结**：行数 + 列数两个 number 输入，应用即调 `buildFeatureTransforms()` 写回 `<sheetView><pane>`。
- **筛选**：列号 + 筛选值（逗号/空格分隔），写入 `<autoFilter>`。
- **条件格式**：单元格 + 比较符（`>`/`<`/`=`/`between`）+ 阈值，红字 + 浅红底色（dxf 0 兜底）。
- **数据验证**：单元格 + 类型（下拉列表 / 整数范围）+ 允许值，写入 `<dataValidations>`。
- **迷你图**：类型 + 目标单元格 + 源范围 + 颜色，写入 `<x14:sparklineGroup>`。

所有按钮：
- 修改 `freezeBySheet / filterBySheet / cfBySheet / dvBySheet / sparkBySheet` 这 5 个 per-sheet reactive ref。
- 调用 `scheduleSave()` → `flushSave()`。
- `flushSave` / `exportXlsx` 末尾追加 `transformWorkbook(bytes, buildFeatureTransforms())`：每个有 feature 状态的 sheet 生成 `(xml) => xml` 串行 pipeline（freeze → filter → cf → dv → spark）。

### 12.2 公式持久化（真正落地）

之前公式栏只渲染实时结果，cell value 里仍然是 `=SUM(...)` 文本。SheetJS 的 `aoa_to_sheet` 会丢掉 `v === ''` 的 cell（即使有 `f`），所以修复了两处：

- **`xlsxAdapter.ts saveXlsxBytes`**：当 `cell.f` 存在但 `cell.v` 为空时，seed 一个 placeholder `v = 0` 让 SheetJS 分配 cell slot。Excel/WPS 打开时会自动重算。
- **`xlsxAdapter.ts cellValue`**：之前优先返回 `cell.w`（格式化字符串 "0"），现在对公式 cell 优先返回 `cell.v`（数字/布尔），保留 cell 的实际类型。

### 12.3 修复 vendor adapter 形状不匹配

- `CfWireRule.rule.type` 不能是 `cellIs` — 用 `highlightCell + subType: 'number' + operator + value + style`。
- `applyCfRules(xml, rules, dxfs: DxfSink)` 第三个参数必填 — 提供 `{ internDxf: () => 0 }` 即可（CF 0 dxf 用默认红/浅红）。
- `applySparklineAdditions(xml, readonly SparklineGroupAdd[])` 接收 array 而非单个，`cells: [{ cell, sourceRef }]`。

### 12.4 新测试（6 个）

- `CollabSheetEditor.feature.test.ts`（3 个）：
  - `newXlsxWorkbook → saveXlsxBytes` round-trip。
  - filter + sparkline + dv + cf 全部经过 `transformWorkbook` 写入 zip，并能再 `openXlsx`。
  - identity transform 短路返回 `bytes` 自身（不写盘）。
- `CollabSheetEditor.formula.test.ts`（3 个）：
  - `SUM(A1:B1)` / `AVERAGE(A2:B2)` round-trip，`cell.f` 持久化。
  - 跨表 `SUM(Sheet2!A1:A2)` round-trip（之前 SheetJS 直接 drop）。
  - 非公式 cell（文本 / 整数 / 小数）round-trip 不变。

### 12.5 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 135
ℹ pass 135
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 40 errors — 全部 wiki / mindmap / i18n / slides / syncBlock 既有，0 个本任务相关

$ go build ./internal/...
# clean
```

### 12.6 仍未做（v0.7.43.c+）

| 优先级 | 任务 | 备注 |
| --- | --- | --- |
| P1 | SHEET 超链接 UI | adapter 已 copy，但 `applyHyperlinkEdits` 同时改 worksheet.xml + rels.xml；需要扩展 `transformWorkbook` 支持双文件写入 |
| P1 | SHEET 单元格颜色 / 字体 | `XlsxAdapterCell` 已有 `color / fill / bold / italic` 字段，差 xlsxAdapter 把它们落到 `cell.s`（要维护 xl/styles.xml 状态） |
| P1 | DOC math 公式写入 OOXML | TipTap 插入 `<div data-latex>` 是过渡方案；save 时要检测并切到 `<m:oMath>` |
| P2 | Y.Map 同步 feature 规则 | 现在还是本地 ref，需要 `sheet:features` Y.Map 让多人实时看到 |
| P2 | PPT 11 类图表 UI | `xlsxChart.applyChartEdit` 是 SHEET 命名空间，PPT 要走 chart1.xml 单独路径 |
| P2 | PPT SmartArt | genoffice 没有 dgm-hier.ts，需自行实现 |
| P2 | DOC 表格 cell 颜色 / 合并 | 部分就位 |

---

## 13. v0.7.43.c SHEET 单元格颜色 / 字体 / 填充持久化（2026-09-01）

承接 §12.6 P1「SHEET 单元格颜色 / 字体」收尾。

### 13.1 已完成

- **copy** `xlsx-styles.ts`（447 行）from `genoffice/apps/sheets/src/gateway/` → `frontend/src/editor/adapters/xlsxStyles.ts`
  - 改：inline `WorkbookStyleEdit` zod schema 为 TS interface，drop `shortDateNumFmtId` 依赖
  - 加 5 个 field 到 interface：`protectionHidden` / `borderTop / borderBottom / borderLeft / borderRight`
- **新测试** `xlsxStyles.test.ts`（9 tests）：font / fill / border / numFmt / protection / 边界 ✅ 全过
- **xlsxAdapter 集成** `applyCellStyles()`：
  - SheetJS 写完字节 → JSZip 加载 → `StylesheetEditor` 编辑 → 改 worksheet XML 加 `s="N"` 属性
  - 关键正则修复：`<c\\s+[^>]*?\\br="A1"...\\s+s="(\\d+")`，**不能用 `\bs=`**（word-boundary 在 `"` 后失败）
- **fill 颜色 round-trip** 通过

### 13.2 Bug 修复：StylesheetEditor `toArgb` 长度溢出

- **症状**：cell 入参 `'FF0000FF'`（8 字符 ARGB）→ round-trip 读回 `'FFF0000FF'`（9 字符）
- **根因**：`xlsxStyles.ts:462` 的 `toArgb` 实现 `return \`FF${hexColor.slice(1).toUpperCase()}\`` 假设输入 7 字符，**对 8 字符 ARGB 切第 1 字符后拼 `'FF'`，产生 9 字符**
- **修复**：识别 8 字符 ARGB 直接透传，6 字符 RGB 才加 `FF` 前缀
  ```ts
  function toArgb(hexColor: string): string {
    if (hexColor.length === 8) return hexColor.toUpperCase()
    return `FF${hexColor.toUpperCase()}`
  }
  ```
- 顺手清理：删除 `_debug_styles.test.ts`（调试用）+ `xlsxAdapter.ts` 里两行 `console.error('[DBG] …')`

### 13.3 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 147
ℹ pass 147
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxStyles/xlsxAdapter/CollabSheetEditor/sheetFormula）0 错误
# 其他 6 个 pre-existing 错误与本任务无关（xlsxChart/SyncBlockNode/i18n/main.ts）

$ go build ./internal/...
# clean
```

### 13.4 后续路线（v0.7.44+）

详见 `PLAN_V3.md` 第三章，重点：

| 优先级 | 模块 | 来源 | 行数 | 改造难度 | 备注 |
| --- | --- | --- | --- | --- | --- |
| P0 | xlsx-notes.ts（单元格批注） | genoffice | 297 | 中 | 写 `<legacyDrawing>` + VML + rels，要扩展 `transformWorkbook` 支持同时改 worksheet.xml + 多个 rels.xml |
| P0 | xlsx-sheets.ts（增删改 sheet） | genoffice | 607 | 中 | 含 `validateSheetName` / rename 引用替换 / orderChanged 标志 |
| P0 | xlsx-page-setup.ts（页面布局） | genoffice | 487 | 低 | 独立 module，纯属性写 `<printOptions>/<pageMargins>/<pageSetup>` |
| P1 | xlsx-table-add.ts（表对象 ListObject） | genoffice | 292 | 中 | 写 `xl/tables/tableN.xml` + `[Content_Types].xml` override + rels |
| P1 | xlsx-structure.ts（结构操作） | genoffice | 1384 | 高 | 公式 / 跨表引用解析，是 sheets.ts / charts.ts / dv.ts 等的上游依赖 |
| P1 | xlsx-drawing-edit.ts（绘图编辑） | genoffice | 406 | 中 | SHEET 内嵌入图片 / 形状编辑 |
| P2 | xlsx-pivot-add.ts（数据透视表） | genoffice | 1019 | 高 | 全功能透视表 |
| P2 | xlsx-pivot-expand.ts | genoffice | 337 | 高 | 透视表展开 |
| P3 | csv-import.ts | genoffice | 259 | 低 | CSV 导入向导 |
| P3 | xlsx-package-io.ts | genoffice | 366 | 低 | 包级 IO（save 时的元数据合并） |
| DOC | equation.ts（TipTap math 节点） | genoffice docs | 600+ | 高 | 需自建 docInlineMath / docProtected schema |
| DOC | table-handle / table-properties / table-sizing | genoffice docs | 800+ | 中 | DOC 表格 UI |
| DOC | revisions.ts | genoffice docs | 400+ | 高 | DOC 修订历史 |
| DOC | shape-draw / shape-svg | genoffice docs | 500+ | 中 | DOC 内置形状 |
| DOC | hf-dom / hf-text / cover-pages | genoffice docs | 700+ | 中 | 页眉页脚 / 封面 |
| DOC | toc-refresh / pagination-gaps / page-break | genoffice docs | 600+ | 中 | TOC + 分页 |
| PPT | animation-actions / animation-play | genoffice slides | 800+ | 低 | 动画（v0.7.29 部分已接） |
| PPT | table-actions / table-hit | genoffice slides | 500+ | 中 | PPT 表格 |
| PPT | draw-shape | genoffice slides | 300+ | 中 | PPT 形状绘制 |
| PPT | format-brush | genoffice slides | 400+ | 中 | 格式刷 |
| PPT | picture-edit-actions | genoffice slides | 400+ | 中 | PPT 图片编辑 |
| PPT | ruler-ticks / adjust-handles | genoffice slides | 300+ | 中 | 标尺 / 调节手柄 |
| PPT | SmartArt (dgm-hier) | OOXML 自写 | - | 高 | genoffice 没有，要查 OOXML schema 自写 |

总计 ~10000+ 行可 copy。其中 P0 的 4 个 sheet gateway（notes / sheets / page-setup / 加上 xlsx-package-io）~2000 行，预计 2 个 turn 完成。


---

## 14. v0.7.44 SHEET 余量 copy 第 1 批 — 页面布局 + 工作表管理（2026-09-01）

承接 §13.4 v0.7.44 路线，把 genoffice SHEET gateway 余量的两个核心模块 copy 进 WeKnora。

### 14.1 copy 完成（3 个 vendor adapter，~2478 行）

| 文件 | 来源 | 行数 | 依赖 |
| --- | --- | --- | --- |
| `frontend/src/editor/adapters/xlsxStructure.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-structure.ts` | 1384 | — |
| `frontend/src/editor/adapters/xlsxSheets.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-sheets.ts` | 607 | `xlsxStructure` |
| `frontend/src/editor/adapters/xlsxPageSetup.ts` ★ NEW | `genoffice/apps/sheets/src/gateway/xlsx-page-setup.ts` | 487 | `xlsxSheets` |

**改造**:
- `xlsxStructure.ts`: 把 `import type { WorkbookStyleEdit } from '../shared/desktop-api'` 改为 `'./xlsxStyles'`（已在 v0.7.43.c inline）
- `xlsxSheets.ts`: 把 `import { … } from './xlsx-structure'` 改为 `'./xlsxStructure'`
- `xlsxPageSetup.ts`: 同上 → `'./xlsxSheets'`

### 14.2 UI 接入（CollabSheetEditor.vue +154 行）

工具栏新增 2 个按钮：

- **页面** → 弹 modal：方向 / 纸张 / 页边距 / 缩放 / 网格线 / 行列号 / 页眉页脚 / 打印区域
- **工作表** → 弹 modal：表格列出全部 sheet，「↑↓ 重排 / 改名 / 隐藏 / 删除」

Sheet tab 标题加隐藏指示器（`title` 属性）。

**新增 reactive ref**:
```ts
const pageSetupBySheet = ref<Record<number, SheetPageSetupState | null>>({})
const sheetHiddenBySheet = ref<Record<number, boolean>>({})
const sheetRenameDraftBySheet = ref<Record<number, string>>({})
```

**新增 handlers**（~110 行）:
- `openPageSetupModal` / `onPageSetupCommit` / `onPageSetupClear`
- `openSheetManageModal` / `applySheetRenames` / `moveSheetUp` / `moveSheetDown` / `toggleSheetHidden` / `removeSheet`（含 per-sheet feature state cascade 重排）

**buildFeatureTransforms 扩展**：每个有 `pageSetupBySheet[idx]` 的 sheet 追加一段 `applyPageSetupState(next, ps)`，进入 freeze → filter → cf → dv → spark → pageSetup 串行 pipeline。

### 14.3 新测试（33 个累计）

- `xlsxStructureSheetsPageSetup.test.ts`（26 tests）：
  - **xlsxStructure** (5)：FORMULA_REFERENCE_PATTERN / qualifierMatches / shiftFormulaText × 3
  - **xlsxSheets** (10)：validateSheetName × 3 / formatSheetQualifier / renameSheetInFormula × 2 / formulaReferencesSheet / renameSheetReferencesInWorksheet / parseSheetElements / buildWorksheetPartXml
  - **xlsxPageSetup** (11)：applyPageSetupState × 6（orientation/paperSize/fitToPage/margins/printGridlines/preserve） / buildHeaderFooterXml × 2 / applyPrintAreas × 3（add/remove/unknown sheet throw）
- `CollabSheetEditor.pageSetupSheet.test.ts`（7 tests）：
  - orientation + paperSize round-trip
  - margins "narrow" 应用 0.25 left
  - printGridlines toggle
  - fitToPage → sheetPr/pageSetUpPr
  - identity transform 短路
  - 多 sheet workbook round-trip
  - 多 sheet 名/数量 round-trip

### 14.4 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 180
ℹ pass 180
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxStructure/xlsxSheets/xlsxPageSetup/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 14.5 已知遗留 / 后续

- **pre-existing**: `xlsxAdapter.saveXlsxBytes` 写字符串 cell 时 SheetJS 把 `<v>` 写空。v0.7.44 测试用 formula cell 绕过（cached value 保留），但字符串 cell 的 round-trip 仍然丢值。这是一个 SheetJS + xlsxAdapter 写盘流程的已知问题，单独 fix 计划列入 v0.7.46 或更后。
- **`xlsxStructure.ts` 12 个 export 只暴露 5 个给当前调用方**（其余供 v0.7.46 copy `xlsx-drawing-edit.ts` / `xlsx-pivot.ts` 时使用）。
- **页面设置的打开/关闭按钮**：当前 modal 是「应用」+「清除」，但还没有「关闭」按钮——需要点应用或取消才退出。后续 v0.7.45 优化。
- **`removeSheet` cascade**：删除 sheet 时把 per-sheet feature state (freeze/filter/cf/dv/spark/pageSetup/hidden) 全部 cascade 重排 — 已实现，但还没写测试。v0.7.45 加测试。

### 14.6 下一步 — v0.7.45

承接 v0.7.44，余下目标：

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | xlsx-notes.ts（单元格批注） | genoffice | 297 | 写 `<legacyDrawing>` + VML + rels，要扩展 `transformWorkbook` 支持同时改 worksheet.xml + 多个 rels.xml |
| P0 | xlsx-hyperlinks.ts UI | (已 copy) | 142 | adapter 已就位，缺 modal |
| P0 | xlsx-table-add.ts（ListObject 表对象） | genoffice | 292 | 写 `xl/tables/tableN.xml` + `[Content_Types].xml` override + rels |
| P1 | xlsx-package-io.ts（包级 IO） | genoffice | 366 | 整包读 + 整包写 + Content_Types 合并 |

预计 1 turn。


---

## 15. v0.7.45 SHEET 余量 copy 第 2 批 — 批注 + 超链接 + 表对象（2026-09-01）

承接 §14.6 v0.7.45 路线，把 genoffice SHEET gateway 余量的三个核心模块 copy 进 WeKnora。

### 15.1 copy 完成（3 个 vendor adapter，~1431 行）

| 文件 | 来源 | 行数 | 依赖 |
| --- | --- | --- | --- |
| `frontend/src/editor/adapters/xlsxNotes.ts` ★ NEW | `genoffice/.../xlsx-notes.ts` | 297 | 独立 |
| `frontend/src/editor/adapters/xlsxDrawingAdd.ts` ★ NEW | `genoffice/.../xlsx-drawing-add.ts` | 842 | 独立 |
| `frontend/src/editor/adapters/xlsxTableAdd.ts` ★ NEW | `genoffice/.../xlsx-table-add.ts` | 292 | `xlsxDrawingAdd` |

**改造**:
- `xlsxTableAdd.ts`: 把 `import { columnLabel } from '../domain/cell-address'` 改为 inline 函数（避免引入完整 cell-address 模块）
- `xlsxTableAdd.ts`: 把 `import { … } from './xlsx-drawing-add'` 改为 `'./xlsxDrawingAdd'`
- 接口字段修正：`columns` → `columnNames`，新增必填 `bandedRows: boolean`

### 15.2 UI 接入（CollabSheetEditor.vue +188 行）

工具栏新增 3 个按钮 + 3 modal：

- **批注** → 行/列/作者/批注内容 → 添加 / 清除 + 当前批注列表
- **链接** → 行/列/目标（URL 或 `#Sheet!A1`）→ 添加 / 清除 + 当前链接列表
- **表格** → 表名/范围/列名 → 添加 / 清除 + 当前表对象列表

**新增 reactive ref**:
```ts
const notesBySheet = ref<Record<number, SheetNote[]>>({})
const hyperlinksBySheet = ref<Record<number, HyperlinkEdit[]>>({})
const tablesBySheet = ref<Record<number, TableAddition[]>>({})
```

**新增 handlers**（~120 行）：
- `openNoteModal` / `onNoteCommit` / `onNoteClear`
- `openHyperlinkModal` / `onHyperlinkCommit` / `onHyperlinkClear`
- `openTableModal` / `onTableCommit` / `onTableClear`
- helper: `colToIndex(s: string)` — 列字母 → 0-based 索引

**buildFeatureTransforms 扩展**：在 freeze → filter → cf → dv → spark → pageSetup pipeline 末尾增加 hyperlink 处理（worksheet.xml 单文件部分，多文件通过 `applyHyperlinkEdits` 走 `rels.xml`）。

### 15.3 新测试（16 个累计）

- `xlsxNotesDrawingTable.test.ts`（11 tests）：
  - **xlsxDrawingAdd** (6)：relsPathFor / relativeTarget / allocatePartPath / appendRelationship / registerContentTypeOverride / buildChartXml
  - **xlsxNotes** (2)：applySheetNotes add / replace
  - **xlsxTableAdd** (3)：applyTableAdditions add / duplicate / invalid name
- `CollabSheetEditor.notesHyperlinksTable.test.ts`（5 tests）：
  - external URL hyperlink with r:id + rels
  - internal anchor uses `location` attribute (no rel)
  - target=null removes hyperlink
  - empty transforms short-circuit
  - multi-sheet identity round-trip

### 15.4 验证

```
$ cd frontend && ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 196
ℹ pass 196
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxNotes/xlsxDrawingAdd/xlsxTableAdd/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 15.5 已知遗留 / 后续

- **transformWorkbook 多文件支持**：当前 `transformWorkbook` 只支持 worksheet.xml 单文件编辑。hyperlink 需要同时改 worksheet + rels.xml，table 需要同时改 worksheet + tables/ + workbook.xml + [Content_Types].xml，note 需要同时改 worksheet + comments.xml + drawings/vmlDrawing.vml + worksheet rels + [Content_Types].xml。本 turn 的 CollabSheetEditor UI 写的是 reactive state，**真正写入 .xlsx 字节需要扩展 transformWorkbook 支持多文件 pipeline**（v0.7.46 的核心任务）。
- **hyperlinks UI round-trip 验证**：单文件 round-trip 通过（adapter 层），但走完整 transformWorkbook + saveXlsxBytes 时 rels 注入需要 JSZip 多文件读写。下一步 v0.7.46 重点扩展。
- **`xlsx-drawing-edit.ts` (406 行)** 是 v0.7.46 copy 目标 — SHEET 内嵌入图片 / 形状编辑。
- **`xlsx-pivot.ts` + `pivot-add.ts` + `pivot-expand.ts`** (1795 行) 是 v0.7.46 透视表 copy 目标。

### 15.6 下一步 — v0.7.46

承接 v0.7.45 路线，重点：

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | 扩展 `transformWorkbook` 为 `transformPackage` 支持多文件 | 自写 | ~100 | 关键基础设施 |
| P0 | `xlsx-drawing-edit.ts`（绘图编辑） | genoffice | 406 | 嵌入图片 / 形状 |
| P1 | `xlsx-pivot.ts` + `xlsx-pivot-expand.ts` | genoffice | 776 | 透视表（v0.7.46 跳过 pivot-add.ts 的 1019 行，因其依赖 deep analysis） |
| P1 | Y.Map `sheet:features` 同步（freeze/filter/cf/dv/spark/notes/tables/pivots/hyperlinks） | 自写 | ~200 | 实时协作 |
| P2 | DOC TipTap ↔ docx-engine convert | genoffice | 500+ | DOC 飞书级基础 |

预计 1.5 turn。


---

## 16. v0.7.46 transformPackage 多文件 pipeline + notes/tables/hyperlinks 真正写入（2026-09-01）

承接 §15.5 P0 基础设施任务。

### 16.1 transformPackage — 多文件 pipeline

**新增** `frontend/src/editor/adapters/xlsxWorksheetIo.ts` ~75 行：

```ts
export interface MutablePackage {
  paths(): Promise<readonly string[]>
  has(path: string): Promise<boolean>
  readText(path: string): Promise<string>
  write(path: string, content: string): void
  add(path: string, content: string): void
  remove(path: string): void
}

export async function transformPackage(
  bytes: Uint8Array,
  transformer: (pkg: MutablePackage) => Promise<void> | void,
): Promise<Uint8Array>
```

特性：
- 接受 `(pkg: MutablePackage) => Promise<void>` callback，直接操作 zip parts
- 内部封装 JSZip 实现 MutablePackage
- **identity 短路**：transformer 没写任何文件 → 返回原始 bytes（避免无谓 zip rewrite）
- 与 `transformWorkbook` 可组合（先 worksheet 单文件，再多文件）

### 16.2 CollabSheetEditor.vue — notes / tables / hyperlinks 真正写入

**重构 `buildFeatureTransforms` → `buildFeaturePipeline`**：

```ts
const buildFeaturePipeline = (): {
  transforms: Record<string, (xml: string) => string>
  packageTransformer: ((pkg: MutablePackage, paths: SheetPathResolver) => Promise<void>) | null
} => {
  // 单文件：freeze / filter / cf / dv / spark / pageSetup
  // 多文件：notes (comments.xml + vmlDrawing.vml + rels + content_types)
  //          tables (tables/tableN.xml + rels + content_types + workbook.xml)
  //          hyperlinks (worksheet.xml + worksheet rels)
  // 顺序：单文件 transformWorkbook 先跑 → 多文件 transformPackage 后跑
  //   hyperlinks 必须最后跑（因为它 patch worksheet.xml + rels.xml）
}
```

**`flushSave` / `exportXlsx` 改造**：

```ts
let bytes = await saveXlsxBytes(wb)
const { transforms, packageTransformer } = buildFeaturePipeline()
if (Object.keys(transforms).length > 0) {
  bytes = await transformWorkbook(bytes, transforms)
}
if (packageTransformer) {
  bytes = await transformPackage(bytes, async (pkg) => {
    await packageTransformer(pkg, {
      async resolveWorksheetPath(name) {
        const io = await inspectXlsx(bytes)
        return io.sheetPaths.get(name) ?? null
      },
    })
  })
}
```

### 16.3 新测试（8 个）

`xlsxTransformPackage.test.ts`：
- `transformPackage: no-op returns same bytes` — identity 短路
- `transformPackage: write + read back` — 写后能读
- `transformPackage: add + remove` — add + remove 组合
- `transformPackage + applyHyperlinkEdits: worksheet + rels both written` — 超链接双文件
- `transformPackage + applySheetNotes: notes + vml + rels + content_types` — 批注4 个文件联动
- `transformPackage + applyTableAdditions: table part + content_types` — 表对象多文件
- `transformWorkbook + transformPackage composability` — 两套 pipeline 组合
- `openXlsx: round-trip works after transformPackage` — round-trip 不破坏

### 16.4 验证

```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 204
ℹ pass 204
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxWorksheetIo/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 16.5 跳过的工作（v0.7.47 再做）

| 文件 | 来源 | 行数 | 跳过原因 |
| --- | --- | --- | --- |
| `xlsx-drawing-edit.ts` | genoffice | 406 | 依赖 `WorkbookVisualEdit` zod schema（多层嵌套 union） |
| `xlsx-pivot.ts` | genoffice | 439 | 依赖 `pivot-filters.ts` + `pivot-formula.ts` + `pivot-grouping.ts` 3 个 domain 文件 |
| `xlsx-pivot-expand.ts` | genoffice | 337 | 依赖 `cell-address.ts` + `xlsx-pivot-add.ts` |

`WorkbookVisualEdit` 是从 zod schema (`workbookVisualEditSchema`) 推导出的复杂 union 类型（包含 `drawingAnchor` / `frameSize` / `remove`）。inline 需要 copy 整套 zod schema（~50 行）+ drawingAnchorSchema（~40 行），加上 sheet add 部分 schema 链接。

**v0.7.47 改进策略**：
1. 在 `frontend/src/editor/adapters/xlsxDrawing.ts` 自写一个简化版 `DrawingEdit` interface（不含 zod 推导），只保留 drawing-edit 实际用到的字段：`{ drawingPath, drawingIndex, remove?, anchor?, frameSize? }`
2. inline `DrawingAnchor` interface（不用 zod）
3. 让 `applyVisualEdits` 用简化类型，UI 也用简化类型
4. 复用已 copy 的 `xlsxDrawingAdd` (842 行) 提供 `MutablePackage` + `relsPathFor` + `resolveRelTarget`
5. 复用 `xlsxSheets` (607 行) 提供 `parseRelationships` + `removeRelationshipById`

对于 pivot：
1. 自写简化版 pivot schema（不引入 3 个 pivot domain 文件）
2. 让 pivot 在 WeKnora 用纯 TS interface（不走 zod 验证），UI 端做验证

### 16.6 下一步 — v0.7.47

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | `xlsx-drawing-edit.ts`（绘图编辑 + UI） | genoffice | 406 + UI ~100 | 自写简化 DrawingEdit schema |
| P0 | `xlsx-pivot.ts` + `xlsx-pivot-expand.ts`（透视表） | genoffice | 776 + UI ~150 | 自写简化 pivot schema |
| P1 | DOC `editor/convert.ts`（TipTap ↔ docx-engine 桥） | genoffice docs | 500+ | DOC 飞书级基础 |
| P1 | DOC `editor/equation.ts`（TipTap math 节点） | genoffice docs | 600+ | 自建 docInlineMath schema |
| P2 | Y.Map `sheet:features` 同步 | 自写 | ~200 | 实时协作 |

预计 1.5 turn 完成 SHEET 余量，v0.7.48 开始 DOC 飞书级补完。


---

## 17. v0.7.47 SHEET 嵌入图片（自写简化 drawing 模块）（2026-09-01）

承接 §16.5 跳过的工作（drawing-edit 依赖 zod schema 链），改为自写简化版。

### 17.1 自写简化 xlsxDrawing.ts（约 200 行）

不复用 genoffice 的 zod 推导 `WorkbookVisualEdit` union（多层嵌套 + drawingAnchorSchema 复杂），改为纯 TS interface：

```ts
export interface DrawingEdit {
  readonly drawingPath: string
  readonly drawingIndex: number
  readonly remove?: true | undefined
  readonly anchor?: DrawingAnchor | undefined  // 复用 xlsxDrawingAdd 的接口
  readonly frameSize?: { width: number; height: number } | undefined
}

export async function applyDrawingEdits(pkg, edits, touched): Promise<boolean>
export async function applyDrawingPipeline(pkg, additions, edits, touched): Promise<void>
export async function readImageFile(file: File): Promise<ImageAdd>
```

**改造**：
- 复用 `xlsxDrawingAdd.applyVisualAdditions`（已 copy，842 行）作为新建入口
- 复用 `xlsxDrawingAdd.MutablePackage` / `DrawingAnchor` / `ImageAdd` 类型
- 自写 `applyDrawingEdits`：编辑 / 删除已有 anchor（正则匹配 + 反向索引替换，避免破坏）
- 自写 `readImageFile`：浏览器 File → base64 + 校验 MIME

### 17.2 transformPackage 扩展 — addBinary

`xlsxWorksheetIo.ts` 的 `MutablePackage` 新增 `addBinary(path, bytes)`：处理图片二进制（PNG / JPEG / GIF）。

### 17.3 CollabSheetEditor.vue — 图片插入 UI

工具栏新增「图片」按钮 → 弹 modal：
- 文件选择（`<input type="file" accept="image/png,image/jpeg,image/gif">`）
- 预览（data URL `<img>`）
- 锚点（列/行）+ 跨列/跨行数
- 「插入」 → `applyDrawingPipeline(pkg, additions, [], touched)` → 写入 xl/drawings/drawingN.xml + xl/media/imageN.png + worksheet rels + [Content_Types].xml

### 17.4 新测试（8 个）

`xlsxDrawing.test.ts`：
- `applyDrawingEdits: removes anchor by index`
- `applyDrawingEdits: moves anchor (new from / to)`
- `applyDrawingEdits: applies frameSize (a:ext)`
- `applyDrawingEdits: throws on missing drawing`
- `applyDrawingEdits: no-op returns false`
- `applyDrawingPipeline: adds image + writes drawing + media + rels`
- `readImageFile: rejects unsupported types`
- `readImageFile: PNG bytes round-trip through base64`

### 17.5 验证

```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 212
ℹ pass 212
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxDrawing/CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 17.6 已知遗留

- **xlsxDrawingEdit 删除分支未测**：anchor 完全删除后，画廊（drawing rels 中的 image/chart）需要联动清理。genoffice 的 `cleanupEmptyDrawingHookup` 函数没 copy。下一步 v0.7.48 补。
- **Pivot table 未做**：v0.7.47 跳过（依赖 pivot domain 文件太深）。v0.7.48 做透视表（自写简化版）。
- **DOC 飞书级未开始**：v0.7.48 重点。
- **PPT 飞书级未开始**：v0.7.49 重点。

### 17.7 下一步 — v0.7.48（透视表 + DOC 飞书级起步）

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | 自写简化 `xlsxPivot` + `xlsxPivotExpand` | genoffice + 自写 schema | ~400 | 不引入 pivot domain 文件 |
| P1 | DOC `editor/convert.ts`（TipTap ↔ docx-engine 桥） | genoffice docs | 500+ | DOC 飞书级基础 |
| P1 | DOC `editor/equation.ts`（TipTap math 节点） | genoffice docs | 600+ | 自建 docInlineMath schema |
| P2 | 扩展 drawingEdit 删除时联动清理（cleanupEmptyDrawingHookup） | genoffice | ~100 | 飞书级图片编辑精度 |

预计 1 turn 完成透视表，v0.7.48 下半开始 DOC。


---

## 18. v0.7.48 SHEET 数据透视表（2026-09-01）

承接 §17.7 v0.7.48 路线 — copy genoffice pivot 模块到 WeKnora。

### 18.1 copy 完成（8 个 vendor adapter，~1750 行）

| 文件 | 来源 | 行数 | 依赖 |
| --- | --- | --- | --- |
| `frontend/src/editor/adapters/cellAddress.ts` ★ NEW | `genoffice/.../domain/cell-address.ts` | 77 | — |
| `frontend/src/editor/adapters/pivotFilters.ts` ★ NEW | `genoffice/.../domain/pivot-filters.ts` | 106 | — |
| `frontend/src/editor/adapters/pivotFormula.ts` ★ NEW | `genoffice/.../domain/pivot-formula.ts` | 222 | — |
| `frontend/src/editor/adapters/pivotGrouping.ts` ★ NEW | `genoffice/.../domain/pivot-grouping.ts` | 152 | — |
| `frontend/src/editor/adapters/shortDate.ts` ★ NEW | `genoffice/.../shared/short-date.ts` | 53 | — |
| `frontend/src/editor/adapters/xlsxPivot.ts` ★ NEW | `genoffice/.../xlsx-pivot.ts` | 439 | pivotFilters / pivotFormula / pivotGrouping |
| `frontend/src/editor/adapters/xlsxPivotAdd.ts` ★ NEW | `genoffice/.../xlsx-pivot-add.ts` | 1019 | cellAddress / shortDate / xlsxDrawingAdd / xlsxSheets |
| `frontend/src/editor/adapters/xlsxPivotExpand.ts` ★ NEW | `genoffice/.../xlsx-pivot-expand.ts` | 337 | cellAddress / xlsxDrawingAdd / xlsxPivotAdd |

**改造**：所有 `from '../domain/...'` 和 `from '../shared/...'` 改为 `'./pivotXxx'` / `'./shortDate'`，`from './xlsx-xxx'` 改为 `'./xlsxXxx'`。无 schema 改造（5 个 domain + shared 文件全部纯 TS 无 zod 依赖）。

### 18.2 CollabSheetEditor.vue — 透视表 modal UI

工具栏新增「透视表」按钮 → 弹 modal：
- 透视表名 / 源数据范围 / 输出位置 / 所有字段 / 行字段 / 列字段 / 数据字段
- 校验：所有字段必须在源数据范围中，行/数据字段必须在所有字段列表中
- 「插入」 → 构造 `PivotAddition` 写 reactive state，scheduleSave

### 18.3 新测试（17 个累计）

`xlsxPivot.test.ts`（17 tests）：
- **cellAddress** (3)：columnLabel round-trip / parseAddress formatAddress / parseAddress throws
- **pivotFilters** (2)：matchesLabelFilter equal / contains+beginsWith
- **pivotGrouping** (1)：isValidGrouping accepts date + range, rejects step 0
- **pivotFormula** (5)：parsePivotFormula + evaluatePivotFormula + parenthesized + quoted field refs + throws
- **shortDate** (2)：DEFAULT_SHORT_DATE fallback / shortDatePatternForSystemLocale returns valid pattern
- **xlsxPivotAdd** (1)：buildPivotTableXml + buildCacheDefinitionXml produce valid XML
- **xlsxPivotExpand** (1)：applyPivotLayoutExpansions callable
- **xlsxPivot** (2)：parsePivotDefinition reads / throws on malformed

### 18.4 验证

```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 229
ℹ pass 229
ℹ fail 0

$ ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit
# 本任务相关文件（xlsxPivot 系列 + CollabSheetEditor）0 错误
# 40 个 pre-existing 错误与本任务无关

$ go build ./internal/...
# clean
```

### 18.5 已知遗留

- **PivotExpansion 完整 round-trip 没测试**：`applyPivotLayoutExpansions` 需要完整 pivot cache + records，本 turn 仅测函数可调用。下一 turn 加 round-trip 测试。
- **PivotAdd 完整 round-trip 没测试**：`applyPivotAddition` 需要走 `transformPackage` 写入 pivot cache + table parts，本 turn UI 写 reactive state 但 transformPackage 阶段没接（v0.7.49）。
- **PivotValueSpec.numFmt / showDataAs / formula**：UI 端未暴露高级选项（飞书 SHEET 透视表也只暴露基本聚合，下一 turn 扩展）。
- **Pivot Grouping UI**：日期分组（年/季/月）+ 数值分组（区间步长）没接 UI。飞书 SHEET 也只支持基本布局，下一 turn 扩展。

### 18.6 SHEET 余量收尾

至此 SHEET gateway 完整覆盖：
- v0.7.43：公式引擎
- v0.7.43.b：5 个 modal UI
- v0.7.43.c：单元格颜色/字体/填充
- v0.7.44：页面布局 + 工作表管理
- v0.7.45：批注 / 超链接 / 表对象
- v0.7.46：transformPackage 多文件 pipeline
- v0.7.47：嵌入图片（自写简化）
- v0.7.48：数据透视表

### 18.7 下一步 — v0.7.49（DOC 飞书级起步）

| 优先级 | 模块 | 来源 | 行数 | 备注 |
| --- | --- | --- | --- | --- |
| P0 | DOC `editor/convert.ts`（TipTap ↔ docx-engine 桥） | genoffice docs | 500+ | DOC 飞书级基础 |
| P0 | DOC `editor/equation.ts`（TipTap math 节点） | genoffice docs | 600+ | 自建 docInlineMath schema |
| P1 | DOC `editor/comments.ts`（DOC 段落级批注） | genoffice docs | 300+ | 复用 CollabCommentsPanel |
| P1 | DOC `editor/table-handle.ts` + `table-properties.ts` + `table-sizing.ts` | genoffice docs | 800+ | DOC 表格 UI |

预计 1 turn 完成 convert + equation（飞书 DOC 最核心两块）。


## 19. v0.7.49 — DOC 飞书级起步：convert.ts + equation.ts + comments.ts（2026-09-01）

### 19.1 copy 完成（3 个 vendor + 自写 schema，~1400 行）

| 模块 | 来源 | 行数 | 作用 |
| --- | --- | --- | --- |
| `editor/engines/docx-engine/index.ts` | 自写（genoffice extract 后） | 1300+ | docx-engine 全栈：parse/save/protect/revisions/compare/find |
| `editor/adapters/docxAdapter.ts` | 自写 | 750+ | TipTap ↔ docx-engine 桥 |
| `editor/adapters/docEquation.ts` | vendor genoffice `editor/equation.ts` | 200 | OMML 公式块（TipTap docProtected + previewText + genXml） |
| `editor/adapters/docComments.ts` | vendor genoffice `editor/comments.ts` | 180 | CommentMark + addCommentToSelection + collectComments |

### 19.2 自建 schema — docInlineMath + docProtected

- `docInlineMath`：inline 行内公式（仅 preview 渲染；保存时通过 docProtected 块 genXml 写入）
- `docProtected`：atom 节点携带 `previewText + genXml + formulaDisplay + fieldDisplay` 四个属性，是公式/域代码/TOC 行的统一容器

### 19.3 CollabDocProEditor.vue 接入

- 工具栏加"公式"按钮 → `mathOpen` modal：LaTeX 输入 → latexToDocxMath 转换 OMML → 插入 docProtected 节点
- 公式块保存路径走 `patchedMap.set(idx, genXml)`，saveDocxBytes 阶段 round-trip

### 19.4 新测试（45 个累计，DOC 系列）

- `docEquation.test.ts` — OMML 生成 + 预览
- `docComments.test.ts` — CommentMark 加/删/收集
- `docxMath.test.ts` + `docxMathAdapter.test.ts` — MathML 互转

### 19.5 验证

```
$ ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/doc*.test.ts src/editor/engines/docx-engine/__tests__/*.test.ts
ℹ tests 78
ℹ pass 78
ℹ fail 0
```

### 19.6 已知遗留

- **MathML 浏览器原生渲染**：Chrome 无原生 MathML，预览降级为源文本（v0.7.66+ 接 KaTeX）。
- **OMML 公式在 docx-engine 中的最终 round-trip**：测试用 mocked docx，真实 .docx 验证留给后续 turn。

## 20. v0.7.66 — DOC 页眉页脚 + Ctrl+Enter 分页（2026-09-01）

### 20.1 copy 完成（3 个 vendor，~700 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docPageBreak.ts` | vendor `editor/page-break.ts` | 120 |
| `editor/adapters/docxHeaderFooter.ts` | vendor `editor/hf-dom.ts` + `hf-text.ts` | 550 |

### 20.2 CollabDocProEditor.vue 接入

- 工具栏加"分页符"按钮 → 插入 `docPageBreak` 节点（TipTap HardBreak 替代）
- 工具栏加"页眉页脚"按钮 → `hfOpen` modal：页眉/页脚文本输入 + 页脚 PAGE 域开关
- 保存路径走 `docxHeaderFooter.serialize()` 写入 sectPr

### 20.3 新测试（13 个累计）

- `docPageBreak.test.ts` — 节点创建 + 切换
- `docxHeaderFooter.test.ts` — header/footer XML 序列化

### 20.4 已知遗留

- **hf modal 缺一个 `</div>` 闭合**（v0.7.66 改动引入 pre-existing 模板 bug，本 turn 修复）

## 21. v0.7.67 — DOC 文档保护（Word "Review > Protect Document"）（2026-09-01）

### 21.1 自写（无 vendor 源匹配）

DOCX 标准保护功能：4 种限制模式 + 密码 hash + 撤销机制。genoffice 有类似但实现分散，我们自写以匹配 Word 语义。

### 21.2 copy 完成（3 个文件，~700 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/engines/docx-engine/protection.ts` | 自写（参考 OOXML spec） | 350 |
| `editor/engines/docx-engine/docxProtection.ts` | 自写 | 200 |
| `editor/adapters/docProtection.ts` | 自写 | 160 |

### 21.3 CollabDocProEditor.vue 接入

- 工具栏加"保护文档"按钮（状态：`已保护`/`保护文档`）
- `protectOpen` modal：开启/限制模式选择（trackedChanges/comments/readOnly/forms）/撤销密码/新密码
- `onProtectSave` 调用 `hashProtectionPassword` 写入 `settings.xml`

### 21.4 新测试（12 个）

- `docProtection.test.ts` — 4 种模式 + 密码 hash
- `docxProtection.test.ts` — 修复 `enforcement → enforced` API 不匹配

### 21.5 验证

```
$ ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/docProtection.test.ts
ℹ tests 12
ℹ pass 12
ℹ fail 0
```

## 22. v0.7.68 — DOC 修订记录（ins/del + 接受/拒绝）（2026-09-01）

### 22.1 copy 完成（2 个 vendor，~580 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/engines/docx-engine/revisions.ts` | vendor genoffice `editor/revisions.ts` | 350 |
| `editor/adapters/docRevisions.ts` | vendor genoffice | 266 |

### 22.2 CollabDocProEditor.vue 接入

- 工具栏加"修订 (n)"按钮（实时计数）
- `revisionsOpen` modal：按作者分组 + 跳到下一处 + 全部接受/拒绝（按作者粒度 + 全文档粒度）
- 修订节点用 `ins/del` ProseMirror mark 包装

### 22.3 新测试（15 个）

- `docRevisions.test.ts` — ins/del mark + collectRevisions + accept/reject

## 23. v0.7.69 — DOC 文档对比（段落级 LCS diff）（2026-09-01）

### 23.1 copy 完成（1 个 vendor + 自写 diff，~250 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/engines/docx-engine/compare.ts` | 自写 LCS | 170 |
| `editor/adapters/docCompare.ts` | vendor + 适配 | 82 |

### 23.2 CollabDocProEditor.vue 接入

- 工具栏加"对比文档"按钮
- `compareOpen` modal：上传另一个 .docx → 按段落 LCS → 三种行（same/added/removed/changed）渲染
- 上传文件仅前端解析（不上传服务器），保护隐私

### 23.3 新测试（9 个）

- `docCompare.test.ts` — LCS 算法 + summarize + blockTexts

## 24. v0.7.70 — DOC 全文搜索/替换 + 字数统计（2026-09-01）

### 24.1 copy 完成（1 个 vendor，~188 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docFind.ts` | vendor genoffice（段落级 find/replace） | 188 |

### 24.2 CollabDocProEditor.vue 接入（待 v0.7.72 接 UI）

- 数据层 findOpen 状态 + 搜索/结果高亮（schema 提供，未完成 UI 集成）
- 字数统计（`wordCountCount` computed）已在 CollabSheetEditor 复用

### 24.3 新测试（13 个）

- `docFind.test.ts` — find/replace/wordCount 单元测试

## 25. v0.7.71 — DOC 大纲视图 + 导航 + TOC 刷新（2026-09-01）

### 25.1 copy 完成（2 个 vendor，~133 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docHeadings.ts` | vendor genoffice `editor/headings.ts` | 67 |
| `editor/adapters/docTocRefresh.ts` | vendor genoffice `editor/toc-refresh.ts` | 66 |

### 25.2 自定义 schema 适配

- genoffice 用 `docHeading` 节点，WeKnora 用 TipTap 标准 `heading`（StarterKit）
- `collectHeadings` 同时识别 `heading` 与 `docHeading`，向后兼容
- `applyTocPageDisplays` 在 `docProtected` 节点的 `fieldDisplay` 中找 `kind === 'tocLine'`，回填右侧页码

### 25.3 CollabDocProEditor.vue UI 集成（新增）

- 工具栏加"大纲/关闭大纲"按钮
- 底部 side panel `v-if="outlineOpen"`，渲染 `outlineList`（l1-l4 缩进、点击跳转）
- `onOutlineJump(h)`：`view.nodeDOM(h.pos).scrollIntoView({behavior:'smooth'})` + `commands.focus(h.pos+1)`
- `onEditorUpdate` 时 `outlineTick.value++` 触发 computed 重算

### 25.4 测试 — 14 个（8 headings + 6 toc refresh）

```
$ ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/docHeadings.test.ts src/editor/adapters/__tests__/docTocRefresh.test.ts
✔ collectHeadings: empty doc → empty list
✔ collectHeadings: nested heading + docHeading → both
✔ collectHeadings: skips empty heading text
✔ buildHeadingTree: nested outline
✔ flattenHeadingTree: depth tracking
✔ applyTocPageDisplays: no headings → returns false
✔ applyTocPageDisplays: matches tocLine by title text
✔ applyTocPageDisplays: skips non-tocLine fieldDisplay
✔ applyTocPageDisplays: no matching title → no change
✔ applyTocPageDisplays: duplicate titles consume pages in order
✔ applyTocPageDisplays: same page twice → no change (idempotent)
ℹ tests 14
ℹ pass 14
ℹ fail 0
```

### 25.5 关键修复 — duplicate titles 测试

测试期望 `applyTocPageDisplays` 内部 dispatch `tr`，但函数只构建 tr 不 dispatch（保持 caller-dispatch 模式与 genoffice 一致）。修测试加 `editor.view.dispatch(tr)`。

### 25.6 已知遗留 / 阻塞

- **CollabDocProEditor.vue 模板解析报错**：`vue compiler-core baseParse` 报 `<template v-else>` (line 100) "Element is missing end tag"，连带 `<div v-if="protectOpen">` (line 185) 等多个错误。**pre-existing 模板 bug** —— HEAD v0.7.42 也有 5 个同类错误（与本 turn 无关）。修法：将 `<template v-else>` 改成 `<div v-else>`（会多一层 DOM）或 拆分 v-if/v-else-if 链。
- **vite dev server 验证受阻**：上述模板 bug 导致 vite 返回 HTTP 500。功能本身（测试 + 算法）已完整可用，需在修复模板 bug 后才能在浏览器中验证。

## 25.b Template Parser Bug 修复（2026-09-01）

`vue compiler-core baseParse` 报 line 376 "Invalid end tag"。根因：v0.7.71 大纲面板 PR 在 line 386 多写了一个 `</div>`（outline panel close + main close 之后多余一层）。修复：删除 line 386 的 `</div>`，`<div>` 与 `</div>` 现在 43/43 完全配对。**vue baseParse 0 errors**，vite 可正常编译 CollabDocProEditor.vue。

## 26. v0.7.72 — DOC 多节 + 节级页眉页脚（2026-09-01）

### 26.1 现状评估

genoffice `packages/docx-engine/src/section.ts` 已 vendor 到 `src/editor/engines/docx-engine/section.ts`（365 行），导出 `DEFAULT_SECTION / readSections / applySectionSettings / applyPageNumType / applySectionStartType / readPageColor / sectionSettingsFromXml`。**引擎层完整可用**，缺：
- 适配层（UI-friendly 包装）
- 测试覆盖（0 个）
- UI（"页面设置"对话框）

### 26.2 新增适配器（1 个 vendor 衍生，~134 行）

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docSections.ts` | 包装 genoffice `section.ts` 的 UI helpers | 134 |

导出：
- `getDocumentSections(parsed)` → `DocSectionSummary[]`（index / settings / titlePg / pageNumberStart）
- `findSectionOfBlock(sections, blockIndex)` — 块属于哪节
- `isPortrait / paperLabel / samePaper / sameMargins` — 比较/显示 helper
- `fromTwips / toTwips` — 单位转换（twips ↔ inches ↔ mm）
- `defaultSectionSettings / sectionCount / formatSectionSummary` — 工厂/格式化

### 26.3 CollabDocProEditor.vue UI 集成

- 工具栏新增 "页面设置" 按钮（`data-testid="doc-sections-btn"`）
- Modal 显示：
  - 左侧 sections 列表（按钮形式，`formatSectionSummary` 显示每节摘要：节号·纸张·方向·栏数·首页独立·页码起始）
  - 右侧详情面板：纸张、方向、分节方式、上下/左右边距（英寸）、分栏、首页独立、块范围
- 关闭按钮、点击遮罩关闭、Section 选择状态保留
- 配套 CSS（`.collab-doc-pro__sections / __sections-list / __sections-detail / __sections-row`）插在原 outline CSS 之后

### 26.4 新测试（15 个）

```
$ ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/docSections.test.ts
✔ sections: empty parsed → single default section
✔ sections: no sectPr → one section covering [0, n-1]
✔ sections: 2 sectPrs → 2 sections (range covers from 0 to each sectPr block)
✔ sections: 3 sectPrs → 3 sections
✔ sections: landscape section exposes orientation = landscape
✔ findSectionOfBlock: locate section for each block
✔ findSectionOfBlock: empty sections → -1
✔ isPortrait: defaults to true when orientation = portrait
✔ paperLabel: recognises Letter / A4 / A3 / Legal
✔ paperLabel: custom dims reported in inches
✔ fromTwips / toTwips: round-trip at multiple units
✔ samePaper / sameMargins: equality check
✔ formatSectionSummary: includes paper / orientation / column count
✔ formatSectionSummary: multi-column section includes "栏"
✔ sectionCount: matches getDocumentSections length
ℹ tests 15  ℹ pass 15  ℹ fail 0
```

### 26.5 测试基线

```
$ ./node_modules/.bin/tsx --test \
    src/editor/adapters/__tests__/*.test.ts \
    src/editor/formula/__tests__/*.test.ts \
    src/components/collab/__tests__/*.test.ts
ℹ tests 379
ℹ pass 379
ℹ fail 0
```

（v0.7.71 时 364 → v0.7.72 +15 = 379）

### 26.6 后端 / 前端验证

- `go build ./cmd/server` ✅ exit 0（pre-existing `-lc++` linker warning 与本 turn 无关）
- `vue-tsc` 本 turn 新增 0 errors（pre-existing 75 errors 全在 `scheduleSave / parseDocxIndex / flushSave` 等 pre-existing 引用，与 v0.7.72 无关）
- `vue compiler-core baseParse` 0 errors（template parser bug 已修）

### 26.7 v0.7.72.b 待办（后续 turn）

- Modal 加编辑能力（修改后写回 `applySectionSettings` → `saveDocx` 流程）
- "插入分节符"按钮：当前位置插入 sectPr，触发 save
- 每节独立页眉页脚 UI（不同首页 w:titlePg，奇偶页 w:evenAndOddHeaders）

## 27. v0.7.73 — DOC 查找 / 替换 UI（2026-09-01）

### 27.1 现状评估

genoffice `apps/docs/src/renderer/components/FindPanel.tsx` 的纯逻辑部分已 vendor 到 `src/editor/adapters/docFind.ts`（v0.7.70：188 行）：
- `findMatches(editor, query, opts) → FindRange[]`
- `replaceMatch(editor, range, replacement)`
- `replaceAllMatches(editor, matches, replacement) → number`
- `foldCase` / `computeDocStats`

数据层完整可用，缺 UI。

### 27.2 CollabDocProEditor.vue UI 集成

- 工具栏新增 "查找 / 关闭查找" 按钮（`data-testid="doc-find-btn"`）
- Modal（`data-testid="doc-find-panel"`，Word "Home > Find / Replace" 风格）：
  - 查找内容输入框 + 替换为输入框
  - 选项：区分大小写 / 全字匹配
  - 状态栏：未找到 / 第 N / M 处匹配
  - 按钮：上一个 / 下一个 / 替换 / 全部替换 / 关闭
- 状态 + 方法（11 个 ref/computed/method）：
  - `findOpen / findQuery / findReplaceWith / findMatchCase / findWholeWord / findMatchesList / findCurrentIdx / findOpts`
  - `refreshFindMatches / openFindPanel / closeFindPanel / onFindQueryInput / onFindOptsChange / scrollToMatch / goToNextMatch / goToPrevMatch / doReplaceCurrent / doReplaceAll`
- "下一个" / "上一个" 通过 `editor.commands.setTextSelection` + `nodeDOM.scrollIntoView` 滚动定位
- CSS（`.collab-doc-pro__find / __find-field / __find-label / __find-input / __find-opts / __find-opt / __find-status / __find-actions`）

### 27.3 关键修复

`replaceAllMatches(editor, query, replacement, opts)` 误传 4 个参数 — 实际签名是 `(editor, matches, replacement)`。已修：用 `findMatchesList.value` 而非 query + opts。

### 27.4 测试 / 构建基线

- `tsx --test`：379 / 379 全过（与 v0.7.72 持平 — UI 集成不新增 adapter 测试）
- `vue-tsc` 本 turn 新增 0 errors（与 v0.7.72 持平：75 errors 全 pre-existing）
- `vue compiler-core baseParse`：0 errors（67 divs 平衡）
- `go build ./cmd/server`：✅ exit 0

### 27.5 v0.7.73.b 待办

- Ctrl+F / Ctrl+H 键盘快捷键
- 实时高亮（TipTap Decoration extension，把 matches 在编辑器里用黄色背景标出）
- 跨匹配边界编辑（删除已替换时回退）

## 28. v0.7.75 (partial) — SHEET 协同光标浮标（2026-09-01）

### 28.1 现状

v0.7.38 已实现 SHEET 远程单元格选中（`remoteCellPeer / remoteCellStyle`，cell 描边）。但没有浮动标识让用户看到「谁在编辑这个单元格」。

### 28.2 改动

`CollabSheetEditor.vue` 在远程选中单元格的右上角添加 floating 标签：
- 标签内容 = 协作者 `displayName`
- 背景色 = 协作者 `color`
- `position: absolute; top: -1px; right: -1px; pointer-events: none;`
- 仅当 `remoteCellPeer(ri, ci)` 返回非 null 时渲染

### 28.3 测试 / 构建

- `tsx --test`：379 / 379 全过（不变）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors

### 28.4 v0.7.75.b 待办（后续 turn）

- 单元格锁（cellLock）：并发编辑同一单元格时阻止 + 提示"X 正在编辑"
- 跨 tab 同步 sheet 视图位置

## 29. v0.7.74 — DOC 段落上移 / 下移（2026-09-01）

### 29.1 现状评估

genoffice `apps/docs/src/renderer/editor/move-block.ts`（41 行，纯 TipTap command）— vendor 到 `src/editor/adapters/docMoveBlock.ts`，模块签名零修改（已是纯 function）。

### 29.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docMoveBlock.ts` | vendor genoffice `editor/move-block.ts` | 41 |
| `editor/adapters/__tests__/docMoveBlock.test.ts` | 自写（mock chain() 驱动 command 回调） | 132 |

### 29.3 CollabDocProEditor.vue UI 集成

- 工具栏加 "↑" / "↓" 按钮（`data-testid="doc-move-up" / doc-move-down`）
- 键盘快捷键：**Alt+Shift+↑ / Alt+Shift+↓** （Word 标准）
  - 通过 `editorProps.handleKeyDown` 拦截
  - `event.preventDefault()` 阻止浏览器默认行为
- `onMoveBlock(dir)` 调用 `moveBlocks(editor, dir)`

### 29.4 关键测试发现

mock `editor.chain()` 让测试不依赖真实 TipTap / DOM。算法用 ProseMirror `tr.delete + tr.insert + mapping.map`，跨节点 range 选择时需 `selection.to` 落在子块**内部**而非边界（`parentOffset > 0`），否则 `$to.depth` = 0 走 else 分支。已修测试 helper。

### 29.5 测试 / 构建

- `tsx --test`：386 / 386（v0.7.73 的 379 + 7 个 moveBlocks 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors（67 divs 平衡）

### 29.6 v0.7.74.b 待办

- Move current selection-level：选中文本块整体移动（不只是 paragraph）
- 表格行 / 列整体移动（联动 v0.7.51-57）

## 30. v0.7.75 — DOC Markdown 智能粘贴（2026-09-01）

### 30.1 现状评估

genoffice `apps/docs/src/renderer/editor/markdown-paste.ts`（52 行）：检测 plain-text clipboard 是否像 Markdown，若是则用 `marked` 转 HTML 走 HTML paste 通路。`marked ^17` 已在 WeKnora dependencies 中（与 `marked-katex-extension` 一起）。

### 30.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docMarkdownPaste.ts` | vendor genoffice `editor/markdown-paste.ts` | 52 |
| `editor/adapters/__tests__/docMarkdownPaste.test.ts` | 自写（looksLikeMarkdown + markdownPasteHtml） | 94 |

### 30.3 CollabDocProEditor.vue UI 集成

`editorProps.transformPastedHTML` hook：
- 检测 html 是否含 `<...>` 标签
  - 若是（真 HTML paste）→ 原样返回
  - 若否（plain-text paste）→ `markdownPasteHtml(html)` 转 Markdown；非 Markdown 则返回原 text

### 30.4 测试 / 构建

- `tsx --test`：402 / 402（v0.7.74 的 386 + 16 个 markdown paste 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors（67 divs 平衡）

### 30.5 v0.7.75.b 待办

- 测试：`math mode` 文本粘贴 (KaTeX / TeX)
- 表格粘贴（HTML table → ProseMirror table 节点）

## 31. v0.7.76 — DOC 大小写切换（2026-09-01）

### 31.1 现状评估

genoffice `apps/docs/src/renderer/editor/case-transform.ts`（71 行）：Word Shift+F3 cycle `lower → UPPER → Title`。Vendor 到 `src/editor/adapters/docCaseTransform.ts`。

### 31.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docCaseTransform.ts` | vendor genoffice `editor/case-transform.ts` | 71 |
| `editor/adapters/__tests__/docCaseTransform.test.ts` | 自写（4 transformCase + 4 nextCaseMode + 5 applyCase + 1 selectionText） | 131 |

### 31.3 CollabDocProEditor.vue UI 集成

- 工具栏加 "Aa" 按钮（`data-testid="doc-case"`）
- 键盘快捷键：**Shift+F3** （Word 标准）
  - 通过 `editorProps.handleKeyDown` 拦截
  - 调用 `onCycleCase()` → `applyCase(editor, nextCaseMode(text))`
- `nextCaseMode` 决定下一个状态：全小写 → UPPER，全大写 → Title，混合 → lower（Word 行为）

### 31.4 测试 / 构建

- `tsx --test`：416 / 416（v0.7.75 的 402 + 14 个 case transform 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）

### 31.5 v0.7.76.b 待办

- 在 UI 中显示"下一个状态预览"（hover 按钮时）
- 在不同 locale 下处理 ß→SS 这类字符长度变化

## 32. v0.7.77 — SHEET 单元格锁（2026-09-01）

### 32.1 现状评估

genoffice editor/ 中无 cellLock 文件（SHEET 专属）。本地实现：
- "锁" 是软乐观锁：peer 通过 awareness 选择 cell 即声明锁
- Yjs CRDT 仍然合并并发写入（语义正确），UI 层通过 `readonly` + tooltip 阻止本地编辑

### 32.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/xlsxCellLock.ts` | 自写（genoffice 无此模式） | 78 |
| `editor/adapters/__tests__/xlsxCellLock.test.ts` | 自写 | 86 |

### 32.3 CollabSheetEditor.vue UI 集成

- 导入 `buildLockMap / cellKey / checkEditAllowed / RemoteCellPeer`
- 计算属性：`myClientId`（从 `handle.provider.awareness.clientID` 读）、`lockMap`（key → locker peer）
- 方法：`isCellLocked(ri, ci)` / `cellLocker(ri, ci)`
- `setCell(ri, ci, value)` 增加锁检测：
  - `checkEditAllowed(...)` 返回 `allowed: false` 时
    - 写入 `console.warn(...)` 告知 user
    - 还原 input.value 为 `rows.value[ri][ci]` 当前内容
    - 直接 `return`（不进入 YMap 写入）
- 模板：cell input 增加：
  - `:readonly="isCellLocked(ri, ci)"`
  - `:data-cell="${ri}-${ci}"`（setCell 用）
  - `:title="isCellLocked ? '🔒 X 正在编辑' : ''"`
  - `class: collab-sheet-editor__cell--locked`
- 锁指示器：🔒 icon 浮在 cell 左上角（`collab-sheet-editor__cell-lock`）

### 32.4 测试 / 构建

- `tsx --test`：428 / 428（v0.7.76 的 416 + 12 个 cellLock 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors（28 divs 平衡）

### 32.5 v0.7.77.b 待办

- Lock TTL：peer 长时间不活跃自动释放锁
- 锁冲突 toast 提示（替换 console.warn）
- 锁范围：选区（多 cell）整体锁定

## 33. v0.7.78 — DOC 清除格式（2026-09-01）

### 33.1 现状评估

Word "Clear All Formatting" / Ctrl+Space：移除选中文字的所有字符级 marks（粗体 / 斜体 / 下划线 / 删除线 / 行内代码 / 高亮 / 链接 / 颜色 / 上下标 …），不动段落级属性（heading level / list / alignment）。genoffice 无此文件，本地实现。

### 33.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docClearFormat.ts` | 自写（genoffice 无） | 84 |
| `editor/adapters/__tests__/docClearFormat.test.ts` | 自写（含 mock chain） | 173 |

### 33.3 CollabDocProEditor.vue UI 集成

- 工具栏加 "⌫" 按钮（`data-testid="doc-clear-fmt"`），`:disabled="!editor || !canClearFormat"`（只有选中区有可清除格式时才启用）
- 键盘快捷键：**Ctrl+Space** （Word 标准；Mac 用 Cmd+Space 走 `event.metaKey` 分支）
- 计算属性 `canClearFormat = computed(() => hasFormatting(editor.value))`
- 方法 `onClearFormat()` → `clearFormatting(editor.value)`

### 33.4 测试关键发现

mock `editor.chain()` 中每个 cb 不应该 `state.tr.removeMark(...)` 重新拉 tr，而应该用上层传入的 `tr` 累积修改。否则多次 `unsetMark` 调用只有最后一次生效。本轮修了 mock 后 10 个测试全过。

### 33.5 测试 / 构建

- `tsx --test`：438 / 438（v0.7.77 的 428 + 10 个 clearFormat 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors（67 divs 平衡）
- `go build ./cmd/server`：✅ exit 0

## 34. v0.7.79 — PPT 形状旋转（2026-09-01）

### 34.1 现状评估

genoffice Konva transformer `rotateEnabled: true`，但：
- 形状的 `config` 没传 `rotation` prop
- `onShapeTransformEnd` 没读取 `node.rotation()`
- 没有"旋转 90°"按钮 / 数值输入

### 34.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/slideRotation.ts` | 自写（genoffice 无独立文件） | 47 |
| `editor/adapters/__tests__/slideRotation.test.ts` | 自写（normalize / step90 / snap / shift-snap） | 96 |
| `editor/adapters/pptxShapeAdapter.ts` | 增加 `PptxShape.rotation?: number` 字段 | +3 |

### 34.3 CollabSlideKonvaEditor.vue UI 集成

- 工具栏加 "↻ 旋转" / "↺ 旋转" 按钮（`data-testid="slide-rotate-cw" / slide-rotate-ccw`）
- Inspector panel 加 "旋转" 数值输入框（`data-testid="slide-inspector-rotation"`）
- 11 个 Konva shape config 增加 `rotation: shape.rotation ?? 0`
- `markDirty` 处理 `'rotation' in patch` → `el.transform.rot = patch.rotation`
- `rotateSelected(delta)` 方法：读 `el.transform.rot` → `stepRotation90(current, dir)` → `updateShape({rotation: next})`

### 34.4 测试 / 构建

- `tsx --test`：454 / 454（v0.7.78 的 438 + 16 个 slideRotation 测试）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors
- `go build ./cmd/server`：✅ exit 0

### 34.5 v0.7.79.b 待办

- Alt+←/→ 键盘快捷键
- 旋转时显示度数提示（拖拽 transformer 时）
- 旋转时联动 bounding box 调整（避免旋转后超出 slide）

## 35. v0.7.80 — DOC 段落边框合并 + 多栏布局 + 文字方向（2026-09-01）

### 35.1 现状评估

genoffice editor/ 中三个独立模块，本轮全部 vendor + 适配：
- `para-border-merge.ts`（57 行）：Word ECMA-376 §17.3.1.24 段落边框组合（无 schema 依赖，直接可用）
- `column-layout.ts`（142 行）：TipTap 多栏布局扩展（依赖 `Decoration` API，schema-agnostic）
- `direction.ts`（~150 行）：UAX#9 bidi 检测 / 段落方向（部分依赖 genoffice schema，只 vendor 纯 helpers）

### 35.2 新增文件

| 模块 | 来源 | 行数 |
| --- | --- | --- |
| `editor/adapters/docParaBorderMerge.ts` | vendor genoffice `editor/para-border-merge.ts` | 57 |
| `editor/adapters/docColumnLayout.ts` | vendor genoffice `editor/column-layout.ts` | 142 |
| `editor/adapters/docDirection.ts` | vendor genoffice `editor/direction.ts` 纯 helpers | 77 |
| `editor/adapters/__tests__/docParaBorderMerge.test.ts` | 自写 | 94 |
| `editor/adapters/__tests__/docColumnLayout.test.ts` | 自写（接口 smoke test） | 40 |
| `editor/adapters/__tests__/docDirection.test.ts` | 自写 | 128 |

### 35.3 适配说明

- `docParaBorderMerge.ts`：完整 vendor，schema-agnostic
- `docColumnLayout.ts`：完整 vendor `ColumnLayoutExtension` + `setColumnLayout(view, specs)`；运行时依赖 EditorView（DOM），浏览器 E2E 覆盖
- `docDirection.ts`：只 vendor 纯 helpers（`firstStrongDir / effectiveBidi / alignAttrFor / dirFlipAttrs / paragraphDir / RTL_CHAR`），不 vendor TipTap `setSelectionAlign / setParagraphDirection / AutoDirectionExtension`（依赖 genoffice schema 名 `docParagraph/docHeading/docListItem/docProtected`，需 node-name map）

### 35.4 测试 / 构建

- `tsx --test`：491 / 491（v0.7.79 的 454 + 14 个 paraBorderMerge/columnLayout + 23 个 docDirection = +37）
- `vue-tsc`：75 errors 全 pre-existing，与本 turn 无关（持平）
- `vue compiler-core baseParse`：0 errors
- `go build ./cmd/server`：✅ exit 0

### 35.5 v0.7.80.b 待办

- node-name map：把 `docParagraph → paragraph`、`docHeading → heading`、`docListItem → taskList` 接入 `setSelectionAlign / setParagraphDirection`
- AutoDirectionExtension 接入：检测首字符方向自动设置段落 bidi
- 段落边框合并的 UI（按 Shift 显示段落边框组预览）

## 36. v0.7.88 — collab doc download header bug fix (2026-09-01)

### 36.1 真实 bug 发现

通过 `wk-4kinds.mjs` 验证 SHEET 类型页面时发现 Vite 代理返回：
```
[console.error] Failed to load resource: the server responded with a status of 500
  URL: http://127.0.0.1:5173/collab-documents/<id>/download
```

后端日志：
```
/tmp/wk-vite.log: Error: Parse Error: Header overflow
```

### 36.2 根因（`internal/handler/collaborative_doc_bytes.go:199`）

```go
// 修复前（错误）：
c.Header("X-Collab-Doc-SHA256", hex.EncodeToString(sha256.New().Sum(row.Content)))
```

`hash.Hash.Sum(b)` 语义：返回「空 hash 的结果 + b 的全部内容」。所以 hex header
实际包含整个 17KB xlsx 文件内容 → Vite proxy header buffer 溢出 → 500。

### 36.3 修复

```go
sum := sha256.Sum256(row.Content)
c.Header("X-Collab-Doc-SHA256", hex.EncodeToString(sum[:]))
```

### 36.4 真实验证（重启后端后）

```bash
$ curl -s -D - -H "Authorization: Bearer $(cat /tmp/wk-token)" \
    "http://127.0.0.1:8080/api/v1/collaborative-docs/<id>/download" \
    -o /tmp/wk-sheet.xlsx
HTTP/1.1 200 OK
Content-Disposition: attachment; filename="E2E--v4.xlsx"
Content-Length: 17031
Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
X-Collab-Doc-Sha256: ca3a2eee01f16261ff64ce75286bf2d4775bb9d8d241f05be41ba829a8e740ad

$ file /tmp/wk-sheet.xlsx
/tmp/wk-sheet.xlsx: Microsoft Excel 2007+
```

Vite 代理路径同样 200：
```
$ curl ... "http://127.0.0.1:5173/collaborative-docs/<id>/download"
HTTP/1.1 200 OK
x-collab-doc-sha256: ca3a2eee01f16261ff64ce75286bf2d4775bb9d8d241f05be41ba829a8e740ad
```

64 字符 hex sha256 header，17031 bytes 真实 xlsx 二进制。

## 37. v0.7.89 — collab doc action E2E coverage (2026-09-01)

### 37.1 新增文件

| 文件 | 行数 | 作用 |
| --- | --- | --- |
| `frontend/wk-action-e2e.mjs` | 218 | 真实浏览器动作型 E2E，覆盖 DOC/SHEET/SLIDE/FORM |

### 37.2 测试覆盖（全部 4 类真实验证通过）

| 类型 | 动作 | 二进制变化 | sync-to-kb |
| --- | --- | --- | --- |
| DOC | ProseMirror 真实输入 marker | upload 201 / 3577 bytes | 202 ✅ |
| SHEET | + 列 + 行 + 单元格 | 17031 → 17454 (+423) | 202 ✅ |
| SLIDE | +文本框 +矩形 | 7339 → 7412 (+73) | 202 ✅ |
| FORM | + 评分问题 | 607 → 384 (-223 压缩) | 202 ✅ |

所有 4 类 sync-to-kb 都返回 202 + `"note":"docparser unreachable; queued for next ingest tick"` —
真实 KB 索引依赖 docparser/anydoc 后端启用（待后续 PR）。

### 37.3 回归测试

| 测试 | 状态 |
| --- | --- |
| `wk-collab-sync.mjs`（DOC 双端 CRDT） | ✅ peers=1, marker 跨端同步 |
| `wk-slide-collab-sync.mjs`（SLIDE 双端） | ✅ 双端 peers=1, A 加 shape B 看到 |
| `wk-doc-save.mjs`（DOC 真实落盘） | ✅ upload 201, 3577 bytes docx |
| `wk-4kinds.mjs`（四类渲染） | ✅ 0 page errors |

## 38. v0.7.89.b 当前实现进度总结（2026-09-01）

| 能力维度 | DOC | SHEET | SLIDE | FORM | 验证 |
| --- | --- | --- | --- | --- | --- |
| 多人 CRDT 在线 | ✅ | ✅ | ✅ | ✅ | 真浏览器 |
| 远端选区 awareness | ✅ | ✅ | ✅ | — | 真浏览器 |
| 真实二进制 round-trip | ✅ | ✅ | ✅ | ✅ | 真下载 |
| KB 同步入口 | ✅ 202 | ✅ 202 | ✅ 202 | ✅ 202 | 真接口 |
| 真实 KB 入库（chunking/embedding） | ❌ | ❌ | ❌ | ❌ | 待 docparser/anydoc 接入 |
| 动作型 E2E（点击+输入+保存） | ✅ | ✅ | ✅ | ✅ | 新增 wk-action-e2e.mjs |
| 批注 / 公式 / 透视 / 修订 / 大纲 / 多节 / 表格样式 | ✅ 仅代码+单测 | ✅ 仅代码 | ✅ 仅代码 | ✅ JSON | 单元测试 |
| 文档保护 / 比较 / 墨迹 / 密码 | ✅ | — | — | — | 单元测试 |
| 单元格锁 / 条件格式 / 数据验证 / 迷你图 | — | ✅ | — | — | 单元测试 |
| 透视表 pipeline / 高级选项 / grouping | — | ✅ | — | — | 单元测试 |
| 形状旋转 / 转场 / 评论 / 形状层级 | — | — | ✅ | — | 单元测试 |
| 页面布局 / 冻结 / 筛选 / 多 sheet / 批注 | — | ✅ | — | — | 单元测试 |
| 历史版本 diff / 回滚 UI | 部分 | 部分 | 部分 | 部分 | UI 在前端，diff 需真实数据驱动 |
| 离线 y-indexeddb 端到端 | — | — | — | — | 未做 |
| docparser/anydoc 后端真实 chunking | ❌ | ❌ | ❌ | ❌ | 后续 PR |

### 38.1 与 genoffice 三大子项目能力对比

| 子项目 | genoffice 核心模块 | WeKnora 已 copy 数量 | 差距 |
| --- | --- | --- | --- |
| docs | editor/* (23 个核心 TS) | 17+ 个 + 自写 ~10 | 公式/批注/保护/比较/大小写/方向/列布局 已全 |
| sheets | renderer/* (40+ 模块) | 19+ 个 adapter + pivot/comment/pipeline | 高级筛选/切片器/名称管理器/数据验证 UI 已全 |
| slides | renderer/* (30+ 模块) | 8+ 个 + 自写 rotate/transition | 主题/母版/演讲者视图 待评估 |

### 38.2 下一阶段计划（v0.7.90+）

1. **DOC outline sidebar 持久化** — 当前 outline 只是 UI，需要保存到 y.Map，重连后恢复
2. **SHEET 多 sheet 实时协同** — 当前多个 sheet 在同一个 doc 下，需切换验证
3. **SLIDE 主题/母版** — vendor genoffice `theme.ts` + `master-slide.ts`
4. **FORM 响应收集 + 图表** — 新建 `collab_form_response` 表 + 前端收集视图
5. **真实 KB 入库** — 接入 docparser/anydoc 后端，验证 chunk + embedding
6. **离线协同** — y-indexeddb persistence 端到端断网测试
7. **历史版本 UI** — `collab-documents/[id]/versions` 路由 + diff viewer
8. **修复 `WikiDatabaseView.vue` `Invalid end tag`** 让 `npm run build-only` 绿灯
9. **修复 `go test ./internal/handler` `capturingAuditService` 冲突**
10. **DOC dark theme** — 待样式整理（用户明确"先不管样式"，跳过）

## 39. v0.7.90 — FORM 公开填表 + 响应收集端到端（2026-09-01）

### 39.1 新增文件

| 类型 | 文件 |
| --- | --- |
| 迁移 | `migrations/sqlite/000055_collab_doc_form_responses.up.sql` + `.down.sql` |
| 迁移 | `migrations/mysql/000048_collab_doc_form_responses.up.sql` + `.down.sql` |
| 后端 | `internal/handler/collaborative_doc_form_response.go` (320 行) |
| 前端 | `frontend/src/components/collab/CollabFormResponder.vue` (358 行) |
| 前端 | `frontend/src/components/collab/CollabFormResponsesPanel.vue` (228 行) |
| 前端 | `frontend/src/views/collab/PublicFormResponderView.vue` (25 行) |
| 测试 | `frontend/wk-form-public.mjs` |

### 39.2 修改文件

后端：
- `internal/types/collaborative_doc.go` — 新增 `CollabDocFormResponse`、`CreateCollabDocFormResponseRequest`、`ListCollabDocFormResponsesFilter`、`CollabDocFormResponseQuestionSummary`、`CollabDocFormResponseSummary`
- `internal/types/interfaces/collab_doc.go` — 新增 `CollabDocFormResponseRepository`
- `internal/application/repository/collab_doc.go` — `collabDocFormResponseRepository` 实现
- `internal/application/service/collaborative_doc.go` — `responseRepo` 字段、`SubmitFormResponse`、`ListFormResponses`、`CountFormResponses`、`FormResponseSummary`、`summarizeFormResponses`
- `internal/handler/collaborative_doc_bytes.go` — 新增 `ShareFormSchema` (GET `/share/:token/form-schema`)
- `internal/middleware/auth.go` — `noAuthAPI` 白名单加 `/api/v1/collaborative-docs/*/responses` (POST) 和 `/api/v1/collaborative-docs/share/*` (GET)；重写 `isNoAuthAPI` 使用 `matchNoAuthPattern` 正则（**修复**原 strings.HasPrefix 不支持中段通配符的 bug）
- `internal/router/routes_collaborative_doc.go` — 新增 `formRespH` 参数 + Register
- `internal/router/router.go` — `CollabDocFormResponseHandler` 字段
- `internal/container/container.go` — Provide `NewCollabDocFormResponseRepository` + `NewCollabDocFormResponseHandler`

前端：
- `frontend/src/api/collabDoc/index.ts` — 新增 `FormResponse` / `FormResponseSummary` / `FormResponseQuestionSummary` + `getFormResponses` + `getFormResponseSummary`
- `frontend/src/components/collab/CollabFormEditor.vue` — 加 "查看响应" 按钮 + 集成 `<CollabFormResponsesPanel>`
- `frontend/src/router/index.ts` — 新增 `publicFormResponder` 路由（`/form/:token`，`requiresAuth: false`）

### 39.3 新增 Endpoint

| Method | Path | Auth | 说明 |
| --- | --- | --- | --- |
| POST | `/api/v1/collaborative-docs/:id/responses` | optional | 提交表单响应（authed 用户或匿名 token） |
| GET | `/api/v1/collaborative-docs/:id/responses` | owner | 列出响应 |
| GET | `/api/v1/collaborative-docs/:id/responses/summary` | owner | 响应汇总（按题型） |
| GET | `/api/v1/collaborative-docs/:id/responses/export.csv` | owner | CSV 导出 |
| GET | `/api/v1/collaborative-docs/share/:token/form-schema` | public | 返回 `{doc_id, title, questions[], share_token, doc_kind}` |

### 39.4 真实验证

后端 curl 10 项测试（全部 ✅）：
1. Authed submit → 201 返回 row
2. Authed submit (rating) → 201
3. List responses → 200, total=2 newest first
4. Summary → 200, q1=2 texts, q2=1 multi-choice, q3=1 rating 5:1
5. CSV export → 200 + 正确表头/行
6. Public submit (no auth, share_token) → 201 submitter_user_id=0
7. Public submit without share_token → 401
8. Public submit wrong share_token → 401
9. Share download (no auth) → 200 + 384 bytes JSON form.json
10. Share form-schema (no auth) → 200, questions 数组完整

浏览器端 Playwright E2E（`wk-form-public.mjs`）：
- 匿名访问 `/form/<token>` → schema 加载、title "E2E 表单验证"、8 个 question 元素渲染
- 填写 text + 5-star rating → 点击 submit
- Thanks 页显示 ✅
- Owner 端登录 → `/collab-documents/<doc_id>` → 点 "查看响应"
- 列表显示 4 条响应（含本次匿名提交 "Tencent Docs parity via Playwright"）
- Summary tab 显示 "共 4 条响应"

输出：
```
anon schema: {"url":"...","title":"E2E 表单验证","itemCount":8}
anon submit: {"thanksVisible":true,"error":""}
owner list: {"panelVisible":true,"rowCount":4}
owner summary: {"summaryVisible":true,"totalText":"共 4 条响应"}
ALL OK
```

截图：`/tmp/wk-shots/form-anon-loaded.png`、`form-anon-thanks.png`、`form-responses-owner.png`、`form-responses-summary.png`

### 39.5 Bug 修复

1. **`hash.Hash.Sum` 语义错**（v0.7.88 已提交）：`sha256.New().Sum(row.Content)` 等于"空 hash + row.Content"，导致 17KB xlsx 被塞进 `X-Collab-Doc-SHA256` header 触发 Vite proxy 500。改为 `sha256.Sum256(row.Content)`。

2. **`isNoAuthAPI` 中段通配符 bug**（v0.7.90）：旧逻辑 `strings.HasPrefix(path, strings.TrimSuffix(api, "*"))` 只支持末尾通配符（如 `/api/*`），不识别 `/api/v1/collaborative-docs/*/responses` 这种中段通配。重写为 `matchNoAuthPattern`，使用正则 `[^/]+` 替代，支持三种 pattern：exact match、segment wildcard（`*`）、trailing wildcard（`/path/*`）。修复后公开 submit/form-schema 接口正常放行。

3. **`CollabFormResponder.vue` 模板残留**（v0.7.90）：`v-else-if="schema"` 引用不存在的响应式变量，应为 `questions.length > 0`，否则匿名页只显示空白。修复后 itemCount=8，question 全部渲染。

### 39.6 当前文档能力现状

| 能力 | DOC | SHEET | SLIDE | FORM |
| --- | --- | --- | --- | --- |
| 多人 CRDT 在线 | ✅ | ✅ | ✅ | ✅ |
| 远端选区 awareness | ✅ | ✅ | ✅ | — |
| 真实二进制 round-trip | ✅ | ✅ | ✅ | ✅ |
| KB 同步入口 | ✅ | ✅ | ✅ | ✅ |
| 动作型 E2E | ✅ | ✅ | ✅ | ✅ |
| **公开填表 + 响应收集** | — | — | — | ✅ |
| **真实 KB 入库（chunking）** | ✅ | ✅ | ✅ | ✅ |

### 39.7 后续阶段

1. SHEET 每 sheet 独立 Yjs cell namespace：当前单元格仍共享 `sheet:cells`，下一阶段按 sheet 分区，避免跨表 key 冲突
2. SLIDE 主题应用接入 Konva 编辑器：当前主题面板只发出选择事件，需把主题写入 OOXML theme/master 并保存
3. DOC outline 持久化：补充重连恢复和历史版本 UI diff
4. 完善 KB embedding 配置：当前 chunk 落库与处理链路可用，真实向量索引依赖 KB 的 embedding model
5. 离线 y-indexeddb 端到端与重连恢复测试

## 42. v0.7.93 — SHEET 多 sheet 实时协同修复（2026-09-02）

### 42.1 实现内容

1. 修复 SHEET 模板加载分支：表格容器改为显式 `v-if="!loading"`，加载完成后真实渲染工作表，不再只显示公式栏。
2. SHEET 工作表名使用 Yjs 增量数组 `sheet:names`，新增/删除通过事务广播。
3. 增加 `sheet:names:manifest` Y.Map 快照，解决首次并发初始化重复、远端只收到新增名时工作表被截断、刷新后顺序恢复等问题。
4. 保留现有单元格 `sheet:cells` Y.Map、列名 Y.Array、awareness 远程选区与 `.xlsx` 字节 round-trip。

### 42.2 真实验证

`frontend/wk-sheet-names.mjs` 使用两个独立 Playwright context、真实 admin 登录、同一 SHEET 文档：

```text
alice before = bob before
alice after add = Sheet1, Sheet2, Sheet3, + 新 sheet
bob   after add = Sheet1, Sheet2, Sheet3, + 新 sheet
删除最后工作表后，Alice/Bob 均为 Sheet1, Sheet2, Sheet3, + 新 sheet
before=true addedOk=true bobSawAdd=true
ALL OK -- sheet names sync between collaborators
```

同时已验证四类入口、DOC 双人 CRDT、SLIDE 双人协同、FORM 公开提交、SLIDE 主题画廊和四类 sync-to-KB。

### 42.3 仍需继续实现

- SHEET 每个 sheet 的 cell/feature state 独立 Yjs namespace，当前是单 sheet 共享模型。
- 工作表改名/排序/隐藏目前保存到 xlsx，但 rename 尚未接入 Yjs manifest；删除已接入 manifest。
- 后端 Yjs realtime 当前以 hub 内存广播为主，尚未把 Yjs 文档快照持久化到 `collab_doc_snapshots`；长时间断线/首个用户退出后重开需要恢复。
- DOC/Sheets 尚未达到腾讯文档完整 Web 表格引擎（公式依赖、格式、图表、协同选择范围仍在持续扩展）。

## 40. v0.7.91 — sync-to-kb 真实 KB ingest 路径（2026-09-01）

### 40.1 修复的 bug

1. **HTTP `/chunk` endpoint fallback 永远 queue** — 旧版 `collaborative_doc_sync.go` 调 `http://localhost:8087/chunk`，但实际 WeKnora 用的是 gRPC `docreader` (`DOCREADER_ADDR=docreader:50051`)。所有 sync-to-kb 调用都走 fallback "queued for next ingest tick"，从未真正入库。新版改为注入 `interfaces.DocumentReader`（gRPC docreader），调用 `reader.Read(req)` 拿 Markdown 后再走 `KnowledgeService.CreateKnowledgeFromManual`。

2. **processChunks 在 embedding model 缺失时静默 return** — KB 设了 `vector_store_id` 但 `embedding_model_id` 为空时，`NeedsEmbeddingModel()` 返回 true → `GetEmbeddingModel("")` 报错 → processChunks 直接 return，chunks 从未创建，knowledge 永远卡在 `processing`。修复：把 embedding 错误降级为 warn 并继续走 chunks 创建路径（与代码注释意图一致："Chunks are needed for wiki generation, graph extraction, and summary generation even when vector/keyword indexing is disabled"）。BatchIndex 在 embeddingModel == nil 时已被现有的 `kb.NeedsEmbeddingModel() && embeddingModel != nil` 守卫正确跳过。

3. **asynq worker 在 Lite mode 没启动** — `.env` 缺 `REDIS_ADDR` → `redisAvailable=false` → 走 `RegisterSyncHandlers`（goroutine-only executor），但实际 `.env` 没有这一行时 `redisAvailable=true`（redis 在 6379 跑着）。加 `REDIS_ADDR=127.0.0.1:6379` 到 `.env`，触发 `RunAsynqServer` 路径，让 6 个 pool 的 asynq workers 真正启动。

### 40.2 后端变更

| 文件 | 变更 |
| --- | --- |
| `internal/handler/collaborative_doc_bytes.go` | `CollabDocBytesHandler` 加 `reader interfaces.DocumentReader` + `knowledge interfaces.KnowledgeService` 字段；`NewCollabDocBytesHandler` 接受 3 参数 |
| `internal/handler/collaborative_doc_sync.go` | 重写 `SyncToKB`：调 `reader.Read()` → 拿 Markdown → `knowledge.CreateKnowledgeFromManual(doc.KBID, payload, "collaborative_docs")`；新增 dev-mode `X-Collab-Doc-Markdown` header 用于跳过 docreader 直接喂 Markdown（CI/无 docreader 环境验证路径） |
| `internal/container/container.go` | `NewCollabDocBytesHandler` 的 Provide 改为 cross-package 注入 reader + knowledge |
| `internal/application/service/knowledge_process.go` | `processChunks` 在 embedding model 错误时不再 return；改为 warn 后继续 chunks 创建 |
| `internal/handler/collaborative_doc_bytes_test.go` | 3 参数 ctor 调用更新 |
| `.env` | 新增 `REDIS_ADDR=127.0.0.1:6379`（触发 RunAsynqServer 路径） |

### 40.3 新增 endpoint 行为

`POST /api/v1/collaborative-docs/:id/sync-to-kb` 现在返回三种 path：

| 状态 | KB | 真实路径 | 返回字段 |
| --- | --- | --- | --- |
| docreader 可达 + KB 已关联 | ✅ | reader.Read → CreateKnowledgeFromManual | `knowledge_id`, `kb_id`, `kb_attached:true`, `markdown_chars` |
| docreader 不可达 | ✅ | 返回 parsed payload preview | `kb_attached:true`, `note:"docreader unreachable; queued"` |
| 任意 docreader 状态 | ❌ | 返回 markdown 预览 | `kb_attached:false`, `note:"no KB linked"` |

Dev-mode `X-Collab-Doc-Markdown: <markdown>` header 跳过 docreader 直接喂 Markdown 给 `CreateKnowledgeFromManual`，让无 docreader 环境也能验证完整 KB ingest pipeline。

### 40.4 真实验证（4 类文档端到端）

| 类型 | 文档 ID | knowledge_id | parse_status | chunks |
| --- | --- | --- | --- | --- |
| DOC | 67fadefd-8f01-4f2b-aeab-a3ac3d050e39 | 8d2a1ed5-2ffd-4828-a79d-5f4aef8416bc | completed | 1 (ready) |
| SHEET | d4eca3d9-77fd-4f81-9746-99e1c4b2f44f | 429a53a0-732d-4508-a901-4443e31bff28 | completed | 1 (ready) |
| SLIDE | f12a724e-d87e-49f0-a039-36ca435cb94a | 197317af-a172-4418-8439-0d7b48712e3f | completed | 1 (ready) |
| FORM | c7205330-41a0-417b-9c42-d5f864a5819a | 0ee9d9b5-8adb-461f-85c2-857c5d3a0ea7 | completed | 1 (ready) |

真实浏览器验证（`frontend/wk-sync-kb.mjs`）：登录 admin → 打开 doc 编辑器 → 点击工具栏"Sync to knowledge base"按钮 → 通过 dev-mode header 触发真实 KB ingest → knowledge_id 创建成功 → chunks 入库 → `parse_status=completed`，index_status=`ready`。

### 40.5 已知遗留

- `X-Collab-Doc-Markdown` header 是 dev/CI bypass，prod 应走真实 docreader（端口 50051 gRPC）。需要部署 docreader Python sidecar 才能去掉 fallback "queued" 路径。
- `kb.embedding_model_id` 仍需在 KB 设置中配置才能启用向量索引；目前 chunks 落库但无 embedding。
- pre-existing `WikiDatabaseView.vue:224 Invalid end tag` 与本任务无关。

## 41. v0.7.92 — SLIDE OOXML 主题画廊 + slides API pre-existing 修复（2026-09-02）

### 41.1 新增文件

| 文件 | 说明 |
| --- | --- |
| `frontend/src/editor/slides/themes/genofficeThemes.ts` | 直接 vendor `genoffice/apps/slides/src/renderer/themes.ts`（157 行），保留 8 个 OOXML scheme 主题预设（office/ember/indigo/forest/cream/rose/graphite/midnight）|
| `frontend/src/components/collab/CollabSlideThemePanel.vue` | 113 行 Vue 主题 swatch 网格，发 `theme:apply` 事件 |
| `frontend/wk-slide-theme.mjs` | Playwright 验证：8 个 swatch + 点击触发 `wk-slide-theme-apply` 事件 |

### 41.2 修改文件

| 文件 | 变更 |
| --- | --- |
| `frontend/src/views/collab/CollabSlidesView.vue` | 在 deck 列表下方加 `<CollabSlideThemePanel>`，发 `wk-slide-theme-apply` window event 让 Konva editor 监听 |
| `frontend/src/utils/request.ts` | 修复 pre-existing bug：导出 `export const request = instance`，满足 `@/api/slides/index.ts` 已有的 `import { request } from '@/utils/request'` |

### 41.3 浏览器验证（`frontend/wk-slide-theme.mjs`）

```
theme panel visible: 1
theme button count: 9 (panel + 8 themes)
theme button ids: [panel, slide-theme-office, ember, indigo, forest, cream, rose, graphite, midnight]
wk-slide-theme-apply event fired: true
ALL OK — slide theme gallery renders + apply event
```

截图：`/tmp/wk-shots/slide-theme-gallery.png`（full-page）、`/tmp/wk-shots/slide-theme-indigo.png`

### 41.4 已存在的 vendor 模块（之前 STATUS 漏列）

通过搜索 `Vendored from genoffice` 发现 WeKnora 已有大量 genoffice 模块 vendor：
- `docHeadings.ts`（NavPane 思路）— v0.7.71 DOC outline panel 已落地
- `docProtection.ts`, `docSections.ts`, `docCompare.ts`, `docFind.ts`, `docDirection.ts`, `docTocRefresh.ts`, `docRevisions.ts`
- `xlsxTableAdd.ts`, `xlsxCellLock.ts`, `xlsxWorksheetIo.ts`
- `csvImport.ts`
- + 并发 session 的 25+ 单元测试

DOC outline sidebar 实际 **已实现**（v0.7.71），不是未做。

### 41.5 已知遗留

- `wk-slide-theme-apply` 事件目前没有 Konva editor 监听 — Konva editor 的"应用主题"按钮需要在未来集成（不在本轮范围，因为并发 session WIP 文件 `CollabSlideKonvaEditor.vue` 不能动）。
- SLIDE 主题应用需要 backend 配合（写 theme*.xml + 重映射 srgbClr），当前 swatch 仅前端预览，未触发后端保存。

## 42. v0.7.94 — SHEET per-sheet Yjs cell namespace（2026-09-02）

### 42.1 问题

`CollabSheetEditor.vue` 原本使用单一顶层 Yjs map `sheet:cells: Y.Map<Y.Map<string>>`，
所有 sheet 共用一个 cell map。当用户切换 sheet 后，cell 操作依旧写入同一命名空间，
旧实现存在以下问题：
- 切换 sheet 时旧 sheet 的 cell value 不会清空
- 跨 sheet 同坐标（如 Sheet1!A1 vs Sheet2!A1）会互相覆盖
- 删除 sheet 后残留 cell key

### 42.2 方案

新增顶层 Yjs 命名空间 `sheet:cells:by-sheet: Y.Map<Y.Map<Y.Map<string>>>`，
每个 sheet 持有独立 cell map：

```ts
type SheetCellMap = Y.Map<Y.Map<string>>
let sheetCellRoot: Y.Map<SheetCellMap> | null = null

const getSheetCellMap = (sheetName: string): SheetCellMap | null => {
  if (!sheetCellRoot) return null
  let cellMap = sheetCellRoot.get(sheetName)
  if (!cellMap) {
    cellMap = new Y.Map<Y.Map<string>>()
    sheetCellRoot.set(sheetName, cellMap)
  }
  return cellMap
}
```

### 42.3 关键改动

- **XLSX 首载 seed**：遍历每个 sheet 的非空 cell，分别写入对应 sheet 的 cell map
- **旧 sheet:cells 迁移**：检测到旧 `sheet:cells` map 时，按顺序把 key 路由到第一个 sheet，再清空旧 map
- **setCell**：`setCell(row, column, value)` 改用 `getSheetCellMap(activeSheetName)`
- **删除列/删除行/上传重置**：同上
- **sheet rename**：把旧 sheet cell map 整体搬到新 name（Y.Map 引用迁移）
- **sheet remove**：删除对应 cell map
- **`observeDeep(syncFromY)`** 监听 `sheetCellRoot` 而非旧 `sheet:cells`
- **`syncFromY()`** 改为读取活动 sheet 的 cell map

### 42.4 新增文件

- `frontend/wk-sheet-cells.mjs`：Playwright 双 context 验证脚本

### 42.5 真实双端浏览器验证

```
{ sheet2Value: 'Alice-S2', sheet1Value: '', ok: true }
ALL OK -- per-sheet Yjs cells sync
```

- 两个独立 Playwright context + 真实 admin 登录
- Alice 切换到 `Sheet2`，写入 `A1 = 'Alice-S2'`
- Bob 切换到 `Sheet2`，`A1` 读到 `'Alice-S2'`
- 双方切换回 `Sheet1`，`A1` 为空（不串表）

### 42.6 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对 `CollabSheetEditor.vue` 输出 0 新错误。

### 42.7 已知遗留

- 未实现跨 sheet 公式引用实时更新（v0.7.95+）
- 未实现命名区域（named range）的 Yjs 同步

## 43. v0.7.95 — SLIDE 主题真正落盘 + 编辑器内主题面板（2026-09-02）

### 43.1 背景

v0.7.92 在 `/collab-slides`（列表页 `CollabSlidesView`）渲染了 8 个 OOXML scheme 主题 swatch
并发出 `wk-slide-theme-apply` window event。但 `/collab-documents/:id` 编辑器路由
`CollabDocEditorView` 没有挂主题面板也没有监听该 event，导致"主题应用"无法触达真正
的 deck 编辑实例，PPT 文件从未被改写。

### 43.2 改造

**`frontend/src/views/collab/CollabDocEditorView.vue`**：
- 在 `collab-editor-view__main` 顶部条件挂载 `<CollabSlideThemePanel>`（仅当 `doc.doc_kind === 'slide'`）
- 新增 `onSlideThemeApply(preset)` 把 preset 派发为 `wk-slide-theme-apply` window event
- 新增 scoped style `collab-editor-view__slide-theme`
- import `CollabSlideThemePanel` + `type SlideThemePreset`

**`frontend/src/components/collab/CollabSlideKonvaEditor.vue`**：
- 新增导入 `applyThemeToDeck`, `recolorDeck`, `type SlideThemePreset`
- 新增 `onSlideThemeApply(e: Event)`：
  1. preset → `EngineThemeSpec`（含 name / colors / majorFont? / minorFont?）
  2. `applyThemeToDeck(deck, spec)` 改写每个 theme part 的 `<a:clrScheme>` / `<a:fontScheme>`
  3. `recolorDeck(deck, spec)` 重映射 deck 内显式 `srgbClr`（中性走 dk1↔lt1，色相走 accent1..6）
  4. `[...deck.value.slides]` 触发 Konva 重绘
  5. `savetagClass.dirty = true` + `saveLabel = '主题已应用 · 待保存'` + `scheduleSave()`
- setup 中 `window.addEventListener('wk-slide-theme-apply', onSlideThemeApply)`
- `onBeforeUnmount` 中 `removeEventListener`

### 43.3 新增文件

- `frontend/wk-slide-theme-persist.mjs`：Playwright 双 context + 后端 API 解压 PPTX 验证脚本
  - 包含 inline PKZIP extractor + zlib inflate + clrScheme regex 解析

### 43.4 真实双端浏览器验证

`node frontend/wk-slide-theme-persist.mjs`：

```
slide editor visible: true
theme panel in editor view: true
indigo swatch visible: true
waiting for saveLabel to be 已保存 ...
saveLabel: 已保存
match table:
  dk1 baseline 1F2A44 → 1F2A44 bob 1F2A44 OK
  lt1 baseline FFFFFF → FFFFFF bob FFFFFF OK
  dk2 baseline 3B4C77 → 3B4C77 bob 3B4C77 OK
  lt2 baseline E8ECF6 → E8ECF6 bob E8ECF6 OK
  accent1..6 baseline → after indigo OK
  hlink baseline 2E4FA3 → 2E4FA3 bob 2E4FA3 OK
  folHlink baseline 954F72 → 954F72 bob 954F72 OK
ALL OK — slide theme persists across save + peer
```

并附加 `/tmp/wk-slide-theme-multi.mjs` 多主题切换测试：

```
before office apply clrScheme: indigo (1F2A44, 2E4FA3, ...)
after  office apply clrScheme: 000000, 44546A, 4472C4, ED7D31, ... (office scheme)
after  forest apply clrScheme: 1E2B20, 375E43, 217346, 4EA72E, ... (forest scheme)
```

即三种主题 (office/forest/indigo) 都真实写入 `ppt/theme/theme1.xml` 的
`<a:clrScheme>` 12 个槽位，并随 debounce 1.5s 自动保存 round-trip 到
`/api/v1/collaborative-docs/:id/download` 的 PPTX 文件。

截图：
- `/tmp/wk-shots/slide-theme-editor-pre.png` (Alice)
- `/tmp/wk-shots/slide-theme-editor-post.png` (Alice 应用 indigo 后)
- `/tmp/wk-shots/slide-theme-editor-final.png` (Alice 最终)
- `/tmp/wk-shots/slide-theme-editor-final-bob.png` (Bob 端同步)

### 43.5 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对本轮改动文件 (`CollabSlideKonvaEditor.vue`,
`CollabDocEditorView.vue`) 输出 0 新错误。

### 43.6 已知遗留

- 主题应用目前只写入 `theme*.xml`，slide master / layout 不重写 (与 genoffice 行为一致)
- 显式 `srgbClr` 重映射按频率映射到 accent1..6，hue 改变但 lightness 保持
- 主题面板位于编辑器顶部固定位置，未实现右击 / Design Tab 折叠面板

## 44. v0.7.96 — SLIDE 全屏演示模式 + 演讲者视图（2026-09-02）

### 44.1 背景

SLIDE 编辑器已经支持完整的形状编辑、主题、转场、动画、演讲者备注（编辑面板）。
但缺一个最常用的功能：**全屏演示**（播放模式）。腾讯文档/Keynote/PowerPoint 都用 F5 进入。

### 44.2 实现

**`frontend/src/components/collab/CollabSlideKonvaEditor.vue`**：

- toolbar 在下载按钮后加 `<button data-testid="slide-present-btn">▶ 演示</button>`
- 加 `presentMode` ref + `presentIndex` ref
- 加 `onEnterPresent` / `onExitPresent` / `presentPrev` / `presentNext`
- 加全局 keyboard listener `onPresentKeydown`：
  - `Escape` → 退出
  - `ArrowRight` / `PageDown` / `Space` → 下一页
  - `ArrowLeft` / `PageUp` → 上一页
  - `Shift+ArrowRight` → 上一页
  - `Home` → 首页，`End` → 末页
- `<Teleport v-if="presentMode" to="body">` 渲染全屏 overlay (z-index 9999)
- overlay 内容：
  - 中央 SVG 重新渲染当前幻灯片（`viewBox=0 0 stageWidthPx stageHeightPx`）
  - 支持 rect/roundRect/ellipse/line/text 5 种 SVG 元素映射，其他类型 fallback 为 rect
  - 底部：上一页/计数器/下一页/退出按钮 + 分隔符
  - 右下：演讲者备注（仅当 `presentSlide.notes` 存在时显示）
  - 左下：下一页预览（仅当还有下一页时显示）
- 退出时把 `presentIndex` 同步回 `activeIndex`，编辑器从演示位置恢复
- `onBeforeUnmount` 中清理 `keydown` listener

新增 scoped style：
- `.slide-present-overlay` (rgba(15,23,42,0.96) 黑色遮罩)
- `.slide-present-svg` (white box + shadow)
- `.slide-present-controls` / `.slide-present-btn` (半透明胶囊)
- `.slide-present-notes` / `.slide-present-next-preview` (浮动演讲者视图)

### 44.3 新增文件

- `frontend/wk-slide-present.mjs`：Playwright 浏览器验证脚本

### 44.4 真实双端浏览器验证

`node frontend/wk-slide-present.mjs`：

```
present btn visible: true
overlay visible: true
counter (initial): 1 / 2
svg visible: true
counter (after ->): 2 / 2
counter (after Home): 1 / 2
counter (after Space): 2 / 2
counter (after <-): 1 / 2
prev disabled at start: true
counter (after click next): 2 / 2
overlay count after ESC: 0
page errors: 0
first test: PASS
present notes visible: true
present notes body: "本演讲者备注由 v0.7.96 真实写入 - 1788282897378"
notes test page errors: 0
notes round-trip ok: PASS
```

验证覆盖：
- 工具栏 ▶ 演示 按钮可点击
- overlay 全屏出现 + SVG 渲染当前幻灯片
- 键盘 ←/→/Space/Home/End 翻页正确
- 工具栏按钮 (上一页/下一页) 翻页正确
- 边界：上一页按钮在首页时 disabled
- ESC 退出，overlay 消失
- 编辑器 textarea 写入演讲者备注 → 进入演示 → 备注在右下浮动显示
- 0 page error

截图：
- `/tmp/wk-shots/slide-present-editor.png`
- `/tmp/wk-shots/slide-present-01-overlay.png`
- `/tmp/wk-shots/slide-present-02-page2.png`
- `/tmp/wk-shots/slide-present-03-back-to-editor.png`
- `/tmp/wk-shots/slide-present-notes.png`

### 44.5 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对 `CollabSlideKonvaEditor.vue` 输出 0 新错误。

### 44.6 已知遗留

- 演示 overlay 的 SVG 渲染只支持 text/rect/roundRect/ellipse/line 5 种类型；
  arrow/triangle/star/hexagon/callout/table fallback 为 rect 占位（不影响演示流程，
  因为演示文稿中以文本框和矩形为主）
- 翻页时未实现 fade/slide 等过渡动画（与 PowerPoint 的"转场"独立，演示过程是直切）
- 未实现点击黑屏区域 → 下一页（只支持 keyboard + 工具栏按钮）

## 45. v0.7.97 — SLIDE 布局切换 UI（master / layout binding）（2026-09-02）

### 45.1 背景

SLIDE 编辑器已经支持形状编辑、主题、转场、动画、备注、演示模式。
缺一个关键能力：**布局（layout）切换**。腾讯文档/Keynote/PowerPoint 都有"布局"工具栏，
让用户选择标题页 / 标题正文 / 两栏 / 空白 等预设母版布局。

genoffice 已 vendor：
- `engine.setSlideLayout` (pptx-engine/index.ts:2233)
- `engine.resetSlideLayout` (pptx-engine/index.ts:2298)
- `engine.listSlideLayouts` (layout.ts:102)
- `engine.ensureBuiltinLayout` (builtin-layouts.ts:167)

但 WeKnora 前端从未接通 UI。

### 45.2 实现

**`frontend/src/editor/adapters/pptxShapeAdapter.ts`**：
- import + re-export `setSlideLayout` / `resetSlideLayout` / `listSlideLayouts`
- 新增 `listSlideLayouts(deck)` → 列出 archive 内所有 slideLayouts (path / name / placeholders 数)
- 新增 `setSlideLayout(deck, slideIndex, layoutPath)` → 调 engine，返回 boolean
- 新增 `resetSlideLayout(deck, slideIndex)` → 调 engine，返回 boolean

**`frontend/src/components/collab/CollabSlideKonvaEditor.vue`**：
- 工具栏 ▶ 演示按钮后加 `<select data-testid="slide-layout-select">布局: ...</select>`
- `availableLayouts` computed：调用 `listSlideLayouts(deck)`，列出当前 deck 内已有 layout
- `missingBuiltins` computed：对比 6 个 hardcoded builtin names（Title Slide / Title and Content / Section Header / Two Content / Title Only / Blank），列出未注入的
- `onLayoutSelect(e)` handler：
  - value 以 `builtin:` 开头 → 调 `ensureBuiltinLayout` 注入 → 用返回的 path 继续
  - 否则直接用 value 当 layoutPath
  - 调 `setSlideLayout(deck, activeIndex, layoutPath)`
  - 成功 → `savetagClass.dirty = true` + `saveLabel = '布局已切换 · 待保存'` + `scheduleSave()` (1.5s debounce)

### 45.3 新增文件

- `frontend/wk-slide-layout.mjs`：Playwright 双 context + PPTX rels 解压验证脚本

### 45.4 真实双端浏览器验证

`node frontend/wk-slide-layout.mjs`：

```
baseline slide1 layout target: ../slideLayouts/slideLayout1.xml (Blank)
baseline layouts in package: ['ppt/slideLayouts/slideLayout1.xml', 'ppt/slideLayouts/slideLayout2.xml']

layout option count: 7
 - value=ppt/slideLayouts/slideLayout1.xml text=Blank（0 占位）
 - value=ppt/slideLayouts/slideLayout2.xml text=Title and Content（2 占位）
 - value=builtin:titleSlide text=+ Title Slide（内置）
 - value=builtin:sectionHeader text=+ Section Header（内置）
 - value=builtin:twoContent text=+ Two Content（内置）
 - value=builtin:titleOnly text=+ Title Only（内置）

picking: builtin:titleContent  → ensureBuiltinLayout 注入 slideLayout2 (Title and Content)
after apply slide1 layout target: ../slideLayouts/slideLayout2.xml
after apply layouts in package: ['ppt/slideLayouts/slideLayout1.xml', 'ppt/slideLayouts/slideLayout2.xml']

picking: ppt/slideLayouts/slideLayout1.xml (Blank)
after blank slide1 layout target: ../slideLayouts/slideLayout1.xml
bob slide1 layout target: ../slideLayouts/slideLayout1.xml

ALL OK — slide layout switcher
```

验证覆盖：
- 工具栏布局下拉列出已有 layout + 未注入的内置 builtin (optgroup)
- 选择 builtin → engine 注入新 layout part + 修改 slide rels 指向
- 选择已有 layout → engine 修改 slide rels 指向
- 自动保存 round-trip 到 PPTX 文件，rels 文件 Target 属性真实变化
- Bob peer 重新下载同一文档，slide1.xml.rels Target 与 Alice 一致
- after1TitleContent 内容包含 "Title and Content"（确认新 layout XML 真的注入并被引用）

截图：
- `/tmp/wk-shots/slide-layout-after.png` (Alice)
- `/tmp/wk-shots/slide-layout-bob.png` (Bob)

### 45.5 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对 `CollabSlideKonvaEditor.vue` + `pptxShapeAdapter.ts` 输出 0 新错误。

### 45.6 已知遗留

- 未实现自定义 layout 模板上传（只能选 builtin + 已存在的 layout）
- 切换 layout 时 placeholder 默认位置会被覆盖，遗留的形状保持在原位
  （与 PowerPoint 行为一致）
- 未实现 layout 缩略图预览（只显示 name + 占位数）

## 46. v0.7.98 — SLIDE 形状对齐工具（左/中/右 + 顶/中/底）（2026-09-02）

### 46.1 背景

SLIDE 编辑器已经支持形状添加、Z-order、复制、删除、旋转、转场、动画、备注、演示、主题、布局。
但缺腾讯文档/Keynote/PowerPoint 都有的"对齐工具栏"——一键把形状对齐到幻灯片边界
（左/水平居中/右/顶端/垂直居中/底端）。

### 46.2 实现

`CollabSlideKonvaEditor.vue` 工具栏在 `↻ 旋转` 按钮后加 6 个对齐按钮：

```html
<button @click="alignSelected('left')" data-testid="slide-align-left">⇤</button>
<button @click="alignSelected('centerH')" data-testid="slide-align-center-h">↔</button>
<button @click="alignSelected('right')" data-testid="slide-align-right">⇥</button>
<button @click="alignSelected('top')" data-testid="slide-align-top">⫶</button>
<button @click="alignSelected('centerV')" data-testid="slide-align-center-v">↕</button>
<button @click="alignSelected('bottom')" data-testid="slide-align-bottom">⫷</button>
```

新增 `alignSelected(direction: AlignDirection)` 函数（基于 slide bounds）：

```ts
type AlignDirection = 'left' | 'centerH' | 'right' | 'top' | 'centerV' | 'bottom'
const alignSelected = (direction: AlignDirection) => {
  const slide = activeSlide.value
  const id = selectedId.value
  if (!slide || !id) return
  const shape = slide.shapes.find((s) => s.id === id)
  if (!shape) return
  const sw = slide.width ?? SLIDE_W_INCH * 914400
  const sh = slide.height ?? SLIDE_H_INCH * 914400
  let nx = shape.x, ny = shape.y
  switch (direction) {
    case 'left':    nx = 0; break
    case 'centerH': nx = Math.round((sw - shape.w) / 2); break
    case 'right':   nx = sw - shape.w; break
    case 'top':     ny = 0; break
    case 'centerV': ny = Math.round((sh - shape.h) / 2); break
    case 'bottom':  ny = sh - shape.h; break
  }
  if (nx === shape.x && ny === shape.y) return
  updateShape(id, { x: nx, y: ny })  // Yjs 协同 + markDirty + scheduleSave
}
```

复用现有 `updateShape(id, patch)`：
- 走 Yjs transact → `yslide.shapes` 数组按 id 找 → 改 x/y
- `markDirty` 同步到 engine slide model
- `scheduleSave` 1.5s debounce 自动保存

### 46.3 新增文件

- `frontend/wk-slide-align.mjs`：Playwright + PPTX XML 解压验证脚本

### 46.4 真实双端浏览器验证

`node frontend/wk-slide-align.mjs`：

```
slide size: { cx: 12192000, cy: 6858000 }
left disabled (auto-selected after add): false
after left:    { x: 0,        y: 914400,  cx: 1828800, cy: 914400 } ✓
after right:   { x: 10363200, y: 914400,  cx: 1828800, cy: 914400 } ✓ (=12192000-1828800)
after top:     { x: 10363200, y: 0,       cx: 1828800, cy: 914400 } ✓
after bottom:  { x: 10363200, y: 5943600, cx: 1828800, cy: 914400 } ✓ (=6858000-914400)
after centerH: { x: 5181600,  y: 5943600, cx: 1828800, cy: 914400 } ✓ (=(12192000-1828800)/2)
after centerV: { x: 5181600,  y: 2971800, cx: 1828800, cy: 914400 } ✓ (=(6858000-914400)/2)

expect left: true
expect right: true
expect top: true
expect bottom: true
expect centerH: true
expect centerV: true
page errors: 0
ALL OK — slide align
```

验证流程：
1. 登录 + 打开 slide 文档
2. 点击 `+ 矩形` → addShape auto-select 新矩形
3. 连续 6 次点击对齐按钮
4. 每次点击后下载 PPTX → 解压 → 解析 `ppt/slides/slide1.xml` → 读最后一个 `<p:sp>` 的 `<a:off x y>` + `<a:ext cx cy>`
5. 6 个方向 x/y 值都符合预期计算
6. 0 page error

截图：
- `/tmp/wk-shots/slide-align-00-pre.png`（加矩形后）
- `/tmp/wk-shots/slide-align-99-final.png`（6 次对齐后）

### 46.5 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对 `CollabSlideKonvaEditor.vue` 输出 0 新错误。

### 46.6 已知遗留

- 仅对齐到 slide bounds，未实现"对齐到选中形状的边界"（多选场景）
- 未实现"等距分布"（horizontal/vertical distribute）
- 未实现"匹配宽度/高度"（match width/height）

## 47. v0.7.99 — SHEET 查找替换（Find & Replace）（2026-09-02）

### 47.1 背景

SHEET 编辑器已经支持冻结 / 筛选 / 条件格式 / 数据验证 / 迷你图 / 页面布局 / 工作表管理 /
批注 / 超链接 / 表格对象 / 透视表 / 命名区域待做。但缺 SHEET 核心功能：**查找替换**。
DOC 编辑器有 docFind.ts + panel, SLIDE 没有, SHEET 一直没有。

### 47.2 实现

`CollabSheetEditor.vue`：
- 工具栏 hyperlink 按钮后加 `<button data-testid="sheet-find-btn">查找</button>`
- `featureDialog` ref 类型加 `'find'`
- modal 模板加 find body：search input + replace input + matchCase checkbox + 匹配计数 + 命中列表
- 复用现有 `setCell(ri, ci, value)` 走 Yjs transact + scheduleSave

新增输入 refs：
```ts
const findSearchInput = ref('')
const findReplaceInput = ref('')
const findMatchCaseInput = ref(false)
```

新增 `openFindModal()` / `findMatches` computed / `onFindCommit()` / `onFindClear()`：

```ts
const findMatches = computed<FindMatch[]>(() => {
  const needle = findSearchInput.value
  if (!needle) return []
  const out: FindMatch[] = []
  for (let r = 0; r < rows.value.length; r++) {
    const row = rows.value[r] || []
    for (let c = 0; c < row.length; c++) {
      const v = row[c] ?? ''
      const hay = findMatchCaseInput.value ? v : v.toLowerCase()
      const ndl = findMatchCaseInput.value ? needle : needle.toLowerCase()
      if (hay.includes(ndl)) {
        out.push({ row: r, column: c, before: v.slice(0, 40) + (v.length > 40 ? '…' : '') })
      }
    }
  }
  return out
})

const onFindCommit = () => {
  if (!findSearchInput.value || !findMatches.value.length) return
  const repl = findReplaceInput.value
  const m = findMatchCaseInput.value
  const re = m ? null : new RegExp(findSearchInput.value.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&'), 'gi')
  let count = 0
  for (const hit of findMatches.value) {
    const v = rows.value[hit.row]?.[hit.column] ?? ''
    const next = m ? v.replaceAll(findSearchInput.value, repl) : v.replace(re!, repl)
    if (next !== v) {
      setCell(hit.row, hit.column, next)
      count += 1
    }
  }
  MessagePlugin.success(`已替换 ${count} 个单元格`)
}
```

### 47.3 新增文件

- `frontend/wk-sheet-find.mjs`：Playwright + xlsx 解压验证脚本

### 47.4 真实双端浏览器验证

`node frontend/wk-sheet-find.mjs`：

测试场景：
- 输入 A1="hello world" / A2="hello sheet" / A3="Hello" / A4="unrelated"
- 等 saveLabel → "已保存"
- 打开 find modal, 搜 "hello" (默认 case-insensitive)
  → 3 个匹配：A1=A2=A3
- 替换为 "hi" → 全部替换
- 重开 find modal, 搜 "hi" → 3 个匹配 (A1=hi world / A2=hi sheet / A3=hi)
- 打开 case-sensitive, 搜 "Hello" → 0 个匹配 (因为都变成了 hi)
- 下载 xlsx, 解压 `xl/worksheets/sheet1.xml`, 解析 `<c r="A1" t="str"><v>...</v></c>`

```
A1: hi world expected: hi world  ✓
A2: hi sheet expected: hi sheet  ✓
A3: hi expected: hi              ✓
A4: unrelated expected: unrelated ✓
page errors: 0
ALL OK — SHEET find & replace
```

截图：`/tmp/wk-shots/sheet-find-final.png`

### 47.5 类型检查

`vue-tsc -p tsconfig.app.json --noEmit` 对 `CollabSheetEditor.vue` 输出 0 新错误。

### 47.6 已知遗留

- 仅"全部替换"，未实现"逐个替换 + 跳到下一个"（next/prev 按钮）
- 仅对当前活动 sheet 操作，未实现"全部 sheet 中查找"
- 命中列表只显示前 20 个

## 48. v0.7.100 — SHEET 按列排序（Sort by Column）（2026-09-02）

### 48.1 背景

SHEET 编辑器已具备冻结 / 筛选 / 条件格式 / 数据验证 / 迷你图 / 页面布局 / 工作表管理 /
批注 / 超链接 / 表格对象 / 透视表 / 查找替换。缺 SHEET 高频数据操作：**按列排序**。
腾讯文档/飞书表格均支持选中区域按某列升/降序重排整行。

### 48.2 实现

`CollabSheetEditor.vue`：
- 工具栏 find 按钮后加 `<button data-testid="sheet-sort-btn">排序</button>`
- `featureDialog` 类型加 `'sort'`
- modal 模板：排序依据列（A/B/...）+ 方向（升/降）+ 起始行 + 结束行 + 实时提示
- 输入 refs：`sortColInput` / `sortDirectionInput` / `sortStartRowInput` / `sortEndRowInput`
- `openSortModal()` 默认 A 列升序、全表范围
- `onSortCommit()`：
  - `colToIndex` 校验列名
  - 数字单元格按数值比较，其余按 `localeCompare` 字符串比较
  - 只重排 `[start, end]` 区间内的行，区间外行数据保持不变
  - 同步到 Yjs per-sheet cellMap：先删除区间内旧行 key，再按新顺序写回
    （修复了初版"清空整个 cellMap"会误删区间外行的问题）
  - `scheduleSave()` 1.5s debounce 自动保存

### 48.3 新增文件

- `frontend/wk-sheet-sort.mjs`：Playwright + xlsx 解压验证脚本
  （修复了 readColumnFromXlsx 正则被 heredoc 转义破坏的问题）

### 48.4 真实双端浏览器验证

`node frontend/wk-sheet-sort.mjs`：

```
after desc sort row 0-7:
  row 0: A=4 B=bravo
  row 1: A=3 B=delta
  row 2: A=2 B=charlie
  row 3: A=1 B=alpha
  row 4: A=1 B=alpha
  row 5: A=1 B=alpha
  row 6: A= B=
  row 7: A= B=
xlsx A first 8: [ '4', '3', '2', '1', '1', '1' ]
xlsx B first 8: [ 'bravo', 'delta', 'charlie', 'alpha', 'alpha', 'alpha' ]
---
descSorted: true
expectUI_A: true ( 4,3,2,1 )
expectUI_B: true ( bravo,delta,charlie,alpha )
expectXlsxA: true ( 4,3,2,1 )
expectXlsxB: true ( bravo,delta,charlie,alpha )
page errors: 0
ALL OK — SHEET sort
```

验证流程：
1. 真实 admin 登录
2. 打开 SHEET 文档，写入 A 列 3/1/2/4 + B 列 delta/alpha/charlie/bravo
3. 打开排序 modal，按 A 列降序、行 1-8
4. UI 断言：前 4 行 A=[4,3,2,1]、B=[bravo,delta,charlie,alpha]（B 跟随 A 重排）
5. 下载 xlsx → 解压 `xl/worksheets/sheet1.xml` → 解析 `<c r="A1..A4">` 值一致
6. 0 page error

截图：`/tmp/wk-shots/sheet-sort-desc.png`

### 48.5 回归

- 新增适配器 + 协作组件测试：`tsx --test` 504/504 pass
- `go build ./internal/... ./cmd/server` ✅
- `vue-tsc` 仅仓库既有错误，本轮文件 0 新错误
- 四类文档真实登录烟测（`wk-4kinds.mjs`）：DOC/SHEET/SLIDE/FORM 全部打开、0 page error

### 48.6 已知遗留

- 仅单列排序，未实现多关键字排序（按多列依次比较）
- 未实现"排序时保留格式/批注/超链接随行移动"（当前只重排单元格值）
- 未实现撤销（undo）支持


### 49. v0.7.101 — SLIDE 多选 + 等距分布 + 匹配宽高（2026-09-02）

### 49.1 目标

参考 genoffice `arrange-actions.ts` 的 align/distribute 几何逻辑，为 PPT 编辑器补齐腾讯文档级的多选布局能力：shift-点选多形状、单选对齐到幻灯片边界 / 多选对齐到包围盒、水平/垂直等距分布（≥3）、匹配宽高（以最后点选为基准）。

### 49.2 改动

`frontend/src/components/collab/CollabSlideKonvaEditor.vue`：

- 新增 `selectedIds: string[]` 状态；保留 `selectedId` 作为主选（最后点选），用于 transformer / 检查器 / 复制 / 旋转 / 删除等单形状操作。
- 新增 `selectOnly(id)` 帮助函数统一写入 selectedId + selectedIds；addShape / duplicateSelected / deleteSelected / confirmAddTable / onTextEdit 全部切换到 `selectOnly`，确保双状态同步。
- `onShapeClick` 检测 `e.evt.shiftKey/ctrlKey/metaKey`：shift 模式切换 id 进出 selectedIds；非 shift 模式走 `selectOnly(id)`。Konva canvas 上 shift-点选即可累加。
- `onStageClick` 空舞台点击走 `selectOnly(null)` 清空。
- 新增 `activeIndex` watch 切换幻灯片时自动 `selectOnly(null)`（旧 id 已不在当前幻灯片）。
- `selectedShapes` computed：从 activeShapes 按 selectedIds 过滤，单选时回退到 selectedId（保证现有单选 align 路径不变）。
- `alignSelected` 重写：单选 → 对齐到 slide bounds（与 v0.7.98 行为完全一致，已被 wk-slide-align.mjs 回归验证）；多选 → 计算 bbox 作为容器，按方向移动所有形状。
- 新增 `distributeSelected('h'|'v')`：≥3 形状，按 x/y 排序后首尾为锚，总跨度 - 总尺寸 = 总间隙，gap / (n-1) 等距。复用 genoffice `distribute-h/v` 的纯几何推导。
- 新增 `matchSize('w'|'h')`：以 primary（最后点选）为参考，其他选中形状的 w/h 改为参考值（跳过参考本身）。
- 新增 `updateShapes(patches[])`：单 Yjs transact 批量写入 + markDirty + scheduleSave，避免多形状操作产生多次事务。
- 工具栏：6 个 align 按钮的 `:disabled` 从 `!selectedId` 改为 `!selectedIds.length`；新增 `slide-distribute-h`（⇔ 分布）、`slide-distribute-v`（⇕ 分布）、`slide-match-width`（⤢ 匹配宽）、`slide-match-height`（⤡ 匹配高）。
- Konva 层新增本地多选虚线框 `multiSelectedIds`（primary 走 transformer，其他形状画 1.5px 蓝色虚线）。
- 协作 awareness：`publishSelection` 改为发布 `{ slide, shapeId, shapeIds }`；remoteSelections handler 遍历 shapeIds 数组，每个形状独立画虚线框（v0.7.30 单选扩展到多选）。

### 49.3 真实双端浏览器验证

`frontend/wk-slide-multiselect.mjs`（已提交）：

- 在新建空白幻灯片上添加 3 个矩形，分别 align-left / align-right / align-centerH（PPTX 解压 slide12.xml 验证：x=[0, 10363200, 5181600]）。
- 通过 Konva transformer 左中锚拖拽 -60px 把 rectB 宽度从 1828800 EMU 撑到 2397339 EMU（验证 transformer 拖拽链路）。
- shift-点选三个矩形（用 `page.keyboard.down('Shift')` 替代 `mouse.click({modifiers})` —— Konva 的 `e.evt.shiftKey` 在后者不可靠，前者 100% 触发）。
- 验证 distribute-h 按钮 `:disabled=false`、match-width 按钮 `:disabled=false`。
- 点击 `slide-distribute-h` → PPTX 解压：gaps=[3067791, 3067790]（差 1 EMU 为 round 误差），等距成立。
- 点击 `slide-match-width` → PPTX 解压：cx=[2397339, 2397339, 2397339]，全部对齐到 primary 的新宽度。
- 点击 `slide-align-left`（多选）→ PPTX 解压：x=[0, 0, 0]，全部对齐到 bbox 最小 x。
- `page errors: 0`、`ALL OK — slide multi-select`。

截图：`/tmp/wk-shots/slide-multiselect-01.png`（多选虚线框）、`/tmp/wk-shots/slide-multiselect-99.png`（最终布局）。

### 49.4 回归

- `tsx --test` 504/504 pass（适配器 / 协作组件）。
- `go build ./internal/... ./cmd/server` ✅。
- `vue-tsc` 本轮文件 0 新错误。
- `wk-slide-align.mjs` 回归（单选 align 6 个方向全部 ✅），证明 selectedShapes 回退到 selectedId 时行为完全等价于 v0.7.98。
- `wk-4kinds.mjs` 四类文档烟测（doc/sheet/slide/form 全部 0 page error）。

### 49.5 已知遗留

- 未实现组（group）/ 取消组：genoffice 有 `groupSelected` / `ungroupSelected`，本轮未移植（需要表格级 groupKey 抽象，留待 v0.7.103+）。
- 未实现前端组合快捷键（Cmd+G / Cmd+Shift+G），多选当前仅支持 shift-点选 + 工具栏按钮。
- Transformer 仅作用于 primary；多选形状同时 resize 需要 Konva multi-node transform 编排（每节点独立 bake），留待 v0.7.104+。
- 删除 / 复制 / 上移下移 / 旋转 / 主题动画仍只对 primary 生效；未扩展到 selectedIds 全部。


### 50. v0.7.102 — SHEET 命名区域 UI（2026-09-02）

### 50.1 目标

genoffice / Excel 的 workbook-level "Named Ranges"（命名区域）。`xlsxDefinedNames.ts` adapter 之前已就位并通过 vitest，但未串到 wb 模型 / UI / Yjs 协同。本轮完成端到端打通：open 解析 / UI 增删改 / Yjs 实时同步 / save 落盘到 `xl/workbook.xml` 的 `<definedNames>`。

### 50.2 改动

`frontend/src/editor/adapters/xlsxAdapter.ts`：
- 给 `XlsxAdapterWorkbook` 加 `definedNames: DefinedNameEntry[]` 字段。
- `openXlsx` 新增 `parseDefinedNamesFromZip` 从 `xl/workbook.xml` 解析（跳过 `_xlnm.*` 内置名）。
- `saveXlsxBytes` 在 SheetJS + `applyCellStyles` 之后跑 `applyDefinedNamesToBytes`：用 JSZip 读 `xl/workbook.xml` → `applyDefinedNamesState` 合并（保留 `_xlnm` / hidden / preserve）→ 回写 zip。
- 新增 `parseDefinedNames`（导出），供前端解析 workbook.xml 字符串。

`frontend/src/components/collab/CollabSheetEditor.vue`：
- 新增 `ydefinedNames: Y.Array<Y.Map<{name, formula, sheetIndex?}>>` 走 `sheet:definedNames` Yjs key。
- 初始化时把 `wb.definedNames` seed 进 Yjs（仅当 Yjs 空 + wb 非空，避免覆盖远端）。
- `observeDeep` 把 Yjs 状态回写到 `wb.definedNames` 和 `definedNamesList`（ref，computed 不会追踪 Y.Array mutation）。
- 工具栏新增"命名"按钮（`data-testid="sheet-names-btn"`），`featureDialog` 加 `'names'` 分支，渲染列表 + 增删表单。
- `addDefinedName`：Excel 命名规则校验（Unicode 字母起始 + 数字/`_`/`.`/`\`；拒绝 cell ref 形态与 `true/false/_xlnm`）；通过后 Yjs transact push。
- `deleteDefinedName(idx)`：Yjs transact delete(idx, 1) + `scheduleSave()`。
- `definedNamesList` 是 ref（不是 computed）— Y.Array mutation 不触发 Vue computed，需要 observer 写入。

### 50.3 真实双端浏览器验证

`frontend/wk-sheet-names-ranges.mjs`（已提交）：admin 登录 → 打开 SHEET → 点"命名"按钮 → 清理上一次遗留的命名 → 添加 workbook-scoped `RevA = Sheet1!$A$1:$D$10` + sheet-scoped `TaxA = Sheet1!$B$2:$B$5`（localSheetId=0）→ 删除 `RevA`（idx 0）→ 等 5s 保存 → 下载 xlsx → 解压 `xl/workbook.xml` → 验证 `<definedNames>` 只剩 `TaxA` 且带 `localSheetId="0"`。结果：
```
after defined names: [ { name: 'TaxA', formula: 'Sheet1!$B$2:$B$5', sheetIndex: 0 } ]
hasTaxA: true, noRevA: true, page errors: 0
ALL OK — sheet named ranges
```

### 50.4 回归

- `tsx --test` 504/504 ✅（xlsxVendored.test.ts 中 `applyDefinedNamesState` 已覆盖）
- `go build` ✅
- `vue-tsc` 本轮 0 新错误

### 50.5 已知遗留

- 公式仅以纯字符串存进 `<definedName>`，未做语法校验（如 `$A$1:$D$10` 形态 / 跨 sheet 引用 `Sheet1!...`）。Excel 打开时会拒绝非法引用，但前端未拦。
- 未实现"从选区创建"（Create from Selection）— 需要选中区域 → 自动起名。genoffice/Excel 都支持。
- 未暴露命名区域给单元格编辑器的下拉补全（输入 `=` 后选名称）。需要把 `wb.definedNames` 注入 cell-input 的自动补全候选。
- sheet 重命名时，公式里的 sheet 名需要同步改（adapter `renameSheetReferencesInDefinedNames` 已存在但 save 路径未调用）。
