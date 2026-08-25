# Build #10 — ACL 端到端闭环 验收矩阵

> 本 spec.md 与 Runtime 验收矩阵同构,扁平 A1–A16 编号,每条都是可独立验证的项。

### A1 — `wikiPageAcl.test.ts` 至少覆盖 6 个 store 用例

`frontend/src/stores/wikiPageAcl.test.ts` 包含,文件存在且包含至少 6 个 `it()` 用例覆盖 `fetchAcl` + `saveAcl` 错误路径。

### A2 — `WikiAclDialog.test.ts` 覆盖 3 个 dialog 用例

`frontend/src/components/wiki/WikiAclDialog.test.ts` 存在,至少包含 dirty 检测 / submit 成功 / submit 409 触发 refetch 三个用例。

### A3 — `WikiBrowser.acl.test.ts` 覆盖 2 个 integration 用例

`frontend/src/views/knowledge/wiki/WikiBrowser.acl.test.ts` 存在,至少包含 ACL 按钮隐藏(non-owner) / banner 触发两个用例。

### A4 — 4 locale `wiki.acl.error.*` keys 齐备

zh-CN / en-US / ko-KR / ru-RU 四个 locale 文件均新增 `wiki.acl.error.conflict`、`wiki.acl.error.network`、`wiki.acl.error.denied`、`wikiBrowser.aclReadOnlyHint` 四个 key。

### A5 — `npm run check-i18n` 全通过

项目根目录跑 `npm run check-i18n` 返回 11/11(或当前 spec 数)绿。

### A6 — `npm run build` 成功

`vite build` 退出码 0,产出 dist/。

### A7 — `npm test` 全绿

`vitest run` 全部通过,新增 + 已有用例无 regression。

### A8 — `vue-tsc --build` 不引入新错误

`vue-tsc --build` 报错数 ≤ 14(已存在 pre-existing 错误数,主要来自 cc530015 Build #7 frontend 时代)。

### A9 — `putWikiPageAcl` 类型契约不变

`frontend/src/api/wiki/acl.ts` 中 `putWikiPageAcl` 的入参 `WikiAclSaveRequest` 和返回类型 `Promise<WikiPageAcl>` 不变(向后兼容);错误路径在 store 层处理,不在 API 层。

### A10 — `saveAcl` 返回类型变更(内部 breaking)

`frontend/src/stores/wikiPageAcl.ts` 中 `saveAcl` 返回类型从 `Promise<WikiPageAcl | null>` 改为判别联合 `{ ok: true; acl } | { ok: false; conflict: true; current } | { ok: false; conflict: false; error }`。仅 store 内部 + 调用方(dialog)需要适配。

### A11 — `WikiAclDialog.vue` 修改 < 100 行

变更行数 < 100,保留现有 template 结构(只新增事件监听 + 409 处理分支)。

### A12 — `WikiBrowser.vue` 新增 `pageAclWritable`

`frontend/src/views/knowledge/wiki/WikiBrowser.vue` 新增 `pageAclWritable` computed 属性,工具栏 ACL 按钮 `v-if` 条件中加入此判断。计算逻辑(`pageAclWritable = props.canEdit`):复用现有 canEdit 门,与 delete / comment 按钮保持一致;不做严格的 owner / admin 区分(`WikiPage` DTO 不含 `created_by`,需要后端额外暴露),留给后续 Build。

### A13 — `scripts/smoke-wiki-acl.sh` 存在,dry-run safe

`scripts/smoke-wiki-acl.sh` 文件存在,内容为可执行的 curl 命令清单;`bash scripts/smoke-wiki-acl.sh --dry-run` 打印请求样例不实际发出请求(除非带 `--live` 标志)。

### A14 — commit 消息符合 repo 风格

commit 标题以 `feat(wiki): Build #10` 起头,正文包含 bullet 列表 + Co-authored-by trailer,与 cc530015 / 087d73b0 风格一致。

### A15 — working tree 不包含 handoff JSONs

git status 不显示 `dispatch-*.json`、`verifier-response-*.json`、`handoff-*.json` 这些 workdir-only 临时文件(它们在 stash 里,不进 commit)。

### A16 — Build #7 frontend 契约不破坏

合入 lumos0826 后,cc530015 引入的 `WikiAclDialog`、`acl.ts`、`wikiPageAcl.ts`、`aclBanner*` 等 export / props / types 不被破坏;`frontend/src/api/wiki/acl.ts` 的 `getWikiPageAcl` / `putWikiPageAcl` / `searchWikiAclCandidates` 函数签名 / 入参 / 返回类型不变。
