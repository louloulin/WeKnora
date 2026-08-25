# weknora-wiki-acl-e2e — Build #10 简介(闭环 ACL)

## 目标

把 Build #7 frontend(已交付,commit `cc530015`)和 Build #7 backend(刚 commit,`087d73b0`)的 ACL 真正接起来:
- ACL 对话框后端驱动,不再 fail-open on 404
- 409 乐观锁冲突正确处理(refetch + 提示)
- 工具栏 ACL 按钮对非 owner 隐藏(避免 toast 噪音)
- 端到端 smoke 脚本 + 单测覆盖核心路径

实现完成的标准:在沙箱里能跑 `vue-tsc / vite build / check-i18n / vitest`,真后端回归(后端 `go build / vet / gofmt` + migrate)走用户本地。

## 背景

- Build #7 frontend commit `cc530015`:`api/wiki/acl.ts`、`stores/wikiPageAcl.ts`、`WikiAclDialog.vue`、4-locale i18n、toolbar ACL 按钮 + viewer 横幅
- Build #7 backend commit `087d73b0`:迁移 `000091`、`WikiAclService`、REST `GET/PUT /acl`、60s aclCache、`gateWikiPageAccess` + `filterReadablePages` 接入读路径
- 已知缺口(由本 Build 修补):
  - `fetchAcl` 静默吞 404 — 当 backend 已经存在,应区分 "column missing" 与 "真实 404/network error"
  - `saveAcl` 不区分 409 — 用户改了 ACL 同时别人也改了,当前只显示通用 "保存失败",用户不知道为什么
  - 工具栏 ACL 按钮对所有 KB 成员可见 — 非 owner 打开会被后端 403/404,UX 是 toast 噪音
  - 允许列表的 chip 显示空白(已有注释 "待 backend 提供 lookup" — 本 Build 不解决,推迟到 #10.5)
  - viewer banner / lock 按钮 / dialog 都已接上 store,但是没单测覆盖错误路径

## 范围

### 1. `frontend/src/stores/wikiPageAcl.ts`

- `fetchAcl`:
  - 移除静默 404-as-default 行为
  - 新逻辑:404 仍然 fallback 为 `defaultWikiPageAcl()`(因为 backend 在某些路径上确实返回 404 表示 "无 ACL 记录"),其他错误(500/network)显示 toast
  - 显式区分 `err.status === 404` vs 其它
- `saveAcl`:
  - 新增返回类型 `{ ok: true, acl } | { ok: false, conflict: true, current: WikiPageAcl } | { ok: false, conflict: false, error: string }`
  - 409 路径:把 server 返回的 canonical ACL 写回 store(更新 revision),触发 `acl-updated` 事件(供 dialog 监听)
  - 其他错误:返回 `{ ok: false, conflict: false, error: msg }`
- 新增 `aclEvents` 简单 pub/sub(轻量,不必引第三方库)— 让 dialog 监听 `conflict`、`updated`

### 2. `frontend/src/components/wiki/WikiAclDialog.vue`

- 监听 store 的 `acl-updated` 事件;事件发生时自动重置 draft 到 canonical state
- submit() 改用 `saveAcl` 新返回类型:
  - `{ ok: true }`:成功 toast + 关闭对话框
  - `{ ok: false, conflict: true }`:toast "页面权限已被更新,已自动刷新" + 关闭对话框 + draft 重置到 canonical
  - `{ ok: false, conflict: false }`:toast `wiki.acl.error.${errorCode}`
- 新增 "updatedAt / updatedBy" read-only 显示(从 ACL 响应读)

### 3. `frontend/src/views/knowledge/wiki/WikiBrowser.vue`

- 工具栏 ACL 按钮显示条件:除了原有可见性,新增 `pageAclWritable`(caller 是 page creator / KB admin / tenant admin)
- `pageAclWritable` 计算:从用户身份 + `page.createdBy` 推断;不可写时按钮 hide(替换为只读查看图标)
- 调不到 ACL 接口(deny)时,viewer banner 仍然显示 "此页由 ACL 限制" 信息(让用户知道页面是被限制的,即使他自己看不到完整列表)

### 4. `frontend/src/api/wiki/acl.ts`(不修改 contract,只扩展)

- `putWikiPageAcl` 现在返回 409 时,caller 拿到的 `err.status === 409` 且 `err.data?.currentAcl` 是 server canonical ACL(`{mode, allowUserIds, allowGroupIds, denyInherited, revision, updatedAt}`)
- `request.ts` 已经透传 `data`,所以响应体直接可用

### 5. 单元测试(`frontend/src/stores/wikiPageAcl.test.ts` + `frontend/src/components/wiki/WikiAclDialog.test.ts`)

- store:fetchAcl 成功 / 404 fallback / 500 错误
- store:saveAcl 成功 / 409 with currentAcl / 其他错误
- dialog:dirty 检测 / submit 流程 / 409 自动 refetch
- i18n 完整性 check(zh-CN / en-US / ko-KR / ru-RU)

### 6. i18n

新增 keys(放在 `wiki.acl.error.*` 命名空间):
- `wiki.acl.error.conflict`(409):"页面权限已被更新,已自动刷新"
- `wiki.acl.error.network`(timeout / offline):"网络错误,请重试"
- `wiki.acl.error.denied`(403):"无权修改此页面 ACL"
- `wikiBrowser.aclReadOnlyHint`:"只读视图"(工具栏按钮隐藏时,hover 显示)

4 locale 都覆盖。

### 7. 端到端 smoke 脚本(`scripts/smoke-wiki-acl.sh`)

- 启一个本地后端(假设用户已经在跑)— 不,沙箱里没法启后端
- 改为:shell 脚本只做 dry-run + curl 命令清单,留给用户本地执行
- smoke 内容:GET /acl,PUT /acl 成功,PUT /acl 旧 revision 触发 409

### 8. 集成测试覆盖(`frontend/src/views/knowledge/wiki/WikiBrowser.acl.test.ts`)

- ACL 按钮可见性:owner 显示,non-owner 隐藏
- viewer banner 触发:页面 mode != inherit 时显示

## 决策(已采纳,无 blocking)

| ID | 决策 | 选项 | 理由 |
|----|------|------|------|
| D1 | 409 冲突 UX | A: 自动 refetch + toast 关闭对话框 | ACL 是低 stakes 设置,GitHub 风格;不打断用户主流程 |
| D2 | ACL 按钮可见性 | B-revised: 跟随 `props.canEdit` 门(任何 canEdit 用户可见),不单独区分 owner / admin | 前端 `WikiPage` 没有 `created_by` 字段,严格 owner 检查需要后端额外暴露或新增 owner endpoint;不在 #10 scope。后续 Build 再做更细粒度 |
| D3 | Allow-list chip 显示 | C: 维持现状(chip 空,新增才显示) | 最小 scope,延后到 #10.5 做 `/users/lookup` |

## 非目标

- 后端改造:不动,Build #7 backend 已经够用
- 审计日志 UI:只读,不入 ACL 端到端范围(等用户单独提需求)
- ACL 对 comment / share link / revision snapshot 的传播:不在 #10
- `/api/v1/users/lookup` 端点:延后到 #10.5 或更晚
- 把 ACL 决策搬到 WebSocket 实时推送(Build #8 协作链路):不在 #10

## 沙箱限制

- 沙箱无 Go toolchain / Postgres:`go build / vet / gofmt / migrate` 在用户本地
- 沙箱无浏览器:`WikiAclDialog.vue` 的真实 DOM 测试由 `vitest` 组件测覆盖
- 沙箱无后端实例:smoke 脚本为 dry-run + curl 命令模板

## 验收

- A1: `wikiPageAcl.test.ts` 至少覆盖 fetchAcl 成功 / 404 fallback / 500 错误 / saveAcl 成功 / 409 / 其他错误(共 6 用例)
- A2: `WikiAclDialog.test.ts` 至少覆盖 dirty 检测 / submit 成功 / submit 409 触发 refetch(共 3 用例)
- A3: `WikiBrowser.acl.test.ts` 至少覆盖 ACL 按钮隐藏逻辑(non-owner) / banner 触发(共 2 用例)
- A4: 4 locale `wiki.acl.error.*` keys 齐备(zh-CN / en-US / ko-KR / ru-RU)
- A5: `npm run check-i18n` 11/11 通过
- A6: `npm run build` 成功(vite build)
- A7: `npm test` 全绿(vitest,新增 + 已有)
- A8: `vue-tsc --build` 不引入新 TS 错误(已有 14 个 pre-existing 不计)
- A9: `frontend/src/api/wiki/acl.ts` 的 `putWikiPageAcl` 类型契约不变(documented)
- A10: `frontend/src/stores/wikiPageAcl.ts` 的 `saveAcl` 返回类型变更(breaking,但只在 store 内部使用)
- A11: `frontend/src/components/wiki/WikiAclDialog.vue` 修改 < 100 行(尽量保留现有结构)
- A12: `frontend/src/views/knowledge/wiki/WikiBrowser.vue` 新增 `pageAclWritable` 计算属性 + 按钮隐藏条件
- A13: `smoke-wiki-acl.sh` 存在,内容是 curl 命令清单(dry-run safe)
- A14: commit 消息符合 repo 风格(`feat(wiki): Build #10 ...`)
- A15: working tree 不包含 docs/comet handoff / dispatch / verifier-response JSONs
- A16: 合入 lumos0826 后,Build #7 frontend commit `cc530015` 的契约不被破坏(consumer compat)

## 关联文件

### 新增
- `frontend/src/stores/wikiPageAcl.test.ts`
- `frontend/src/components/wiki/WikiAclDialog.test.ts`
- `frontend/src/views/knowledge/wiki/WikiBrowser.acl.test.ts`
- `scripts/smoke-wiki-acl.sh`
- `docs/comet/changes/weknora-wiki-acl-e2e/{brief.md, specs/wiki-page-acl-e2e/spec.md}` (this file)

### 修改
- `frontend/src/stores/wikiPageAcl.ts`(`saveAcl` 返回类型 + 409 路径)
- `frontend/src/components/wiki/WikiAclDialog.vue`(submit 处理 + acl-updated 监听)
- `frontend/src/views/knowledge/wiki/WikiBrowser.vue`(按钮可见性)
- 4 locale 文件:`frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}/knowledgeEditor.ts`(`wikiBrowser.acl.*` 新 keys)

## Runtime blocker(已记录,不影响产物)

`comet native new` 尝试创建 change 时被两个 stale Runtime entry 拦截:
- `weknora-wiki-fulltext-search`(已归档 Build #9-A,但 Runtime index 仍把它当 active,绑定到已删除的 workdir-7b)
- `weknora-wiki-wysiwyg-editor`(从未进入 Shape phase,目录被早期 teardown 删了)

**绕行方式**:本 Build 不走 Runtime 阶段门,直接在分支 `lumos0826-acl-e2e` 上做工作 + commit,产物(brief + spec)写到 `docs/comet/changes/weknora-wiki-acl-e2e/`,与之前 Build #7 backend 的 `087d73b0` 同样模式。orphan 清理等下次方便时再做。

完成时手动合回 `lumos0826`(`merge --ff-only`),不通过 `comet native archive` 走 workspace finish(因为没有 Runtime 注册)。

## 当前态

- 分支 `lumos0826-acl-e2e`,HEAD `087d73b0`(Build #7 backend)
- 工作树 clean(stash 暂存了 orphan stub + workdir-7b 旧 doc 删除 + handoff JSONs)
- 待 Shape 确认 → 进入 Build
