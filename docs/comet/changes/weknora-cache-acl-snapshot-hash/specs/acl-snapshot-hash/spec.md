# Build #27 — Spec: acl_snapshot_hash lazy skip

> 配套 brief.md。本文档是验收蓝本,实现以本文为准。

## 1. 验收项(A1–A8)

每条都给出:**输入 / 期望 / 验证手段**。验证手段优先指向现有 harness 或新写的 Go 测试。

### A1. Identical PutAcl 跳过 cache wipe

- **输入**:page `(kb=k1, slug=s1)` 已有 `acl={mode=public, allow_user_ids=[], allow_group_ids=[], deny_inherited=false}`,revision=3。调用 `PutAcl(k1, s1, {mode=public, allow_user_ids=[], allow_group_ids=[], deny_inherited=false}, baseRevision=3)`。
- **期望**:
  - 函数返回新 revision=4 的 `WikiPageAcl`。
  - `wiki_page_acl_audit` 新增一行,`action="noop_match"`。
  - `wiki_backlinks_cache_invalidation_log` **不**新增任何行。
  - `metricCacheInvalidationsTotal{op="acl_change"}` **不**递增(读 counter 前后值)。
  - `metricAclChangeSkippedTotal{reason="hash_match"}` 递增 1。
- **验证**:`TestPutAcl_IdenticalPayload_SkipsWipe`(harness,SQLite in-memory)+ `scripts/smoke-wiki-acl-snapshot-hash.sh` dry-run 步骤。

### A2. Identical PutAcl 仍递增 revision + 写 audit row

- **输入**:同 A1。
- **期望**:`acl_revision` +1,审计表有一条 `noop_match` 行,`before_acl` 与 `after_acl` JSON 完全相同。
- **验证**:同一 harness test 内断言 revision 字段 + audit 行字段。

### A3. Different PutAcl 仍走完整 wipe(回归)

- **输入**:page 已有 `mode=public`,`PutAcl(... mode=private ...)`。
- **期望**:
  - audit 行 `action="set_private"`。
  - invalidation log 行存在,`affected_count >= 0`。
  - `metricCacheInvalidationsTotal{op="acl_change"}` 递增 1。
  - `metricAclChangeSkippedTotal` **不**递增。
- **验证**:`TestPutAcl_DifferentPayload_RunsWipe`(回归断言)。

### A4. acl_snapshot_hash 字段写入并读回

- **输入**:`PutAcl` 写入新 ACL 后,立刻 `GetAclBySlug(k1, s1)`。
- **期望**:返回的 `WikiPageAcl.SnapshotHash == HashAcl(mode, allowUserIDs, allowGroupIDs, denyInherited)`,与写入时计算的 hash 一致。
- **验证**:`TestPutAcl_HashPersistedAcrossReads`。

### A5. Legacy 行(`acl_snapshot_hash=""`)首次 PutAcl 走 wipe

- **输入**:迁移前已存在的 row,`acl_snapshot_hash=''`(默认值)。`PutAcl` 写入任何新 ACL。
- **期望**:即使写入的内容与旧内容 hash 一致(理论概率极低),`PutAcl` 仍触发 wipe,因为空字符串永远不会等于真实 hash。
- **验证**:`TestPutAcl_LegacyRow_AlwaysWipes`。

### A6. 新 counter `metric_acl_change_skipped_total{reason="hash_match"}`

- **输入**:任意触发 A1 的调用。
- **期望**:counter 在 Prom scrape 输出中可见,值递增正确。
- **验证**:`TestPutAcl_IdenticalPayload_SkipsWipe` 内 `prometheus/testutil.ToFloat64()` 读 counter 值。

### A7. 6 个 harness test 全部通过

- **输入**:`go test ./internal/application/service/... -run 'TestPutAcl|TestAclHash' -v`。
- **期望**:6 个 test 全绿,无 skip、无 flake。
- **验证**:本地 CI 命令同 Build #26。

### A8. Migration 000102 在新库 + 旧库都干净

- **输入**:
  - 新库:`migrate up` 至 head。
  - 旧库:迁移前存在 `wiki_pages` 行,`acl_snapshot_hash` 列不存在。
- **期望**:
  - 新库:迁移后 `DESCRIBE wiki_pages`(或等价查询)可见 `acl_snapshot_hash VARCHAR(16) NOT NULL DEFAULT ''`。
  - 旧库:`ALTER TABLE wiki_pages ADD COLUMN acl_snapshot_hash VARCHAR(16) NOT NULL DEFAULT ''` 成功,所有现存行该列值为 `''`。
- **验证**:`scripts/smoke-wiki-acl-snapshot-hash.sh` 步骤 0(migration 检查)+ CI migrate up。

## 2. 设计选择(D1–D9)

| ID | 决策 | 取值 | 理由 / 反例 |
| --- | --- | --- | --- |
| D1 | Hash 存储位置 | `wiki_pages.acl_snapshot_hash`(与 `acl`, `acl_revision` 同表) | 与 ACL 一起读写,无 JOIN 开销。 |
| D2 | Hash 算法 | SHA-256 截断前 16 个 hex 字符(64 bit) | 生日界 2^32,远超 wiki 页面数量级;stdlib,无新依赖。 |
| D3 | 规范化 | 排序两个 ID slice → `json.Marshal` 固定 struct → SHA-256 | 确定性,跨进程跨方言一致;`TestAclHash_Deterministic` 覆盖。 |
| D4 | Backfill 行为 | 默认 `''` 不做 backfill | 老行首次 PutAcl 一定走 wipe,符合"宁可多 wipe 一次也不漏 wipe"原则;迁移无需扫表。 |
| D5 | 跳过时 audit row | 仍写 `action="noop_match"` | 保持"每次 ACL 写都有审计行"不变量;操作员可按 action 过滤。 |
| D6 | 跳过时 invalidation log row | 不写 | 这是 cache 层"是否真发生了 wipe"的痕迹,跳过 = 没有 wipe = 无痕迹。 |
| D7 | 跳过时 counter | 新 counter `metric_acl_change_skipped_total{reason="hash_match"}` 递增 | 仪表盘需要看见优化生效的频率;不递增的话等于黑盒。 |
| D8 | 迁移编号 | `000102`(在 `000101` 之后) | 无跨依赖,独立提交便于 bisect。 |
| D9 | 分支 / Worktree | 沿用 `lumos0826`,不开 Worktree | 与 B21–B26 一致;无合并仪式开销。 |

## 3. 关键调用链(实现后应有的形态)

```
HTTP PUT /api/v1/knowledgebase/:kb/wiki/pages/:slug/acl
  → WikiAclHandler.PutAcl
    → wikiAclService.PutAcl(ctx, kbID, slug, req, callerUserID, callerRole)
        ├─ beforeACL, getErr := s.repo.GetAclBySlug(ctx, kbID, slug)
        │     └─ SELECT acl, acl_revision, acl_snapshot_hash FROM wiki_pages
        ├─ newHash := HashAcl(req.Mode, req.AllowUserIDs, req.AllowGroupIDs, req.DenyInherited)
        ├─ noop := (getErr == nil && beforeACL != nil && beforeACL.SnapshotHash == newHash)
        ├─ updated, err := s.repo.UpdateAclWithRevision(ctx, kbID, slug,
        │       types.WikiPageAcl{Mode, AllowUserIDs, AllowGroupIDs, DenyInherited},
        │       req.BaseRevision, newHash,
        │       callerUserID, callerRole, actionForMode(mode))
        │     └─ UPDATE wiki_pages SET acl=?, acl_revision=acl_revision+1, acl_snapshot_hash=?
        │       + INSERT INTO wiki_page_acl_audit (action=...)
        ├─ s.cache.invalidatePrefix(kbID + "|" + slug + "|")  // 始终执行
        ├─ if !noop { s.invalidateBacklinksCacheOnAclChange(ctx, ..., noop=false) }
        └─ if noop { metricAclChangeSkippedTotal.WithLabelValues("hash_match").Inc() }
```

注:`s.cache.invalidatePrefix` 是 ACL permission cache 的本地内存失效,与 Build #24 的反向引用 wipe 是两条独立路径,都保留。

## 4. 行为差异总览

| 场景 | 旧行为(B26 末) | 新行为(B27) |
| --- | --- | --- |
| PutAcl 写入相同 payload | revision+1,写 audit,触发 cache wipe,写 invalidation log | revision+1,写 audit(`noop_match`),**跳过** cache wipe,**不写** invalidation log,递增 `metric_acl_change_skipped_total` |
| PutAcl 写入不同 payload | revision+1,写 audit,触发 cache wipe,写 invalidation log | 完全相同(回归) |
| GetAclBySlug 返回值 | `{ACL, Revision}` | `{ACL, Revision, SnapshotHash}`(新增字段) |
| UpdateAclWithRevision 签名 | `(ctx, kbID, slug, newAcl, expectedRevision, actorUserID, actorRole, action)` | 增加 `snapshotHash string` 参数 |
| Legacy 行(无 hash)首次 PutAcl | wipe | wipe(D4 默认行为) |
| Counter / gauge | `metricCacheInvalidationsTotal{op="acl_change"}` 总是递增 | 同 A3;no-skipped 时不递增 |

## 5. 性能影响(预估)

| 操作 | B26 末 | B27 后(同 payload) | B27 后(异 payload) |
| --- | --- | --- | --- |
| PutAcl 大 KB(100k cache rows) | ~50ms wipe + 1 audit + 1 invalidation log | <1ms(1 audit + 1 hash 比对) | 同 B26 |
| PutAcl 小 KB(<=10k cache rows) | <5ms wipe + 1 audit + 1 invalidation log | <1ms(同上) | <5ms(同上) |
| PutAcl legacy 行 | 同上 | 同 B26(wipe 必触发) | 同 B26 |

**净收益**:相同 payload 写入占实际工作负载的 10%–30%(取决于前端表单 UX),每次节省 5–50ms + 1 行 invalidation log 写。在高频 KB 上累计可观。

## 6. 已知限制 / 留给后续 Build

- Hash 截断到 64 bit,生日界 2^32;Wiki 页面总数远超此界,但碰撞后果是"一次本该 wipe 但没 wipe",下次读会 cache miss 自愈。可接受。
- 不区分 mode 变化的子原因(set_inherit vs set_private vs set_allow_list);若未来需要细分,改 `action` 枚举即可,不影响 B27 接口。
- 不支持 `?force=true` 类强制写入;YAGNI,日后真有需求再加。
- Frontend 没有 UX 提示"你的 ACL 没有变化"——优化是静默的,运维层面才看得见 counter。

## 7. 验收提交清单(对应 B27-B4)

- [ ] `migrations/versioned/000102_wiki_pages_acl_snapshot_hash.{up,down}.sql`
- [ ] `internal/application/service/wiki_acl_hash.go`(新文件,`HashAcl` 函数)
- [ ] `internal/application/service/wiki_acl.go`:`PutAcl` 计算 hash + `noop` 路径;`invalidateBacklinksCacheOnAclChange` 接受 `noop` 参数并短路;`logAclChange` 接受 `action` override
- [ ] `internal/application/repository/wiki_acl.go`:`aclColumnProjection` 加 `acl_snapshot_hash`;`aclRow` 加字段;`UpdateAclWithRevision` 写新列
- [ ] `internal/types/wiki_page.go` / `interfaces/wiki_page.go`:`WikiPageAcl.SnapshotHash` 字段 + `UpdateAclWithRevision` 签名变化
- [ ] `internal/application/service/metrics.go`:`metricAclChangeSkippedTotal` 新 counter
- [ ] `internal/application/service/wiki_acl_test.go` + `wiki_audit_harness_test.go`:`stubWikiAclRepo` 等 fakes 同步新签名 + 6 个新 test
- [ ] `scripts/smoke-wiki-acl-snapshot-hash.sh`(dry-run 安全)
- [ ] `gofmt -l` 通过;`go vet ./...` 无新增警告(本地不可用,CI 跑)
- [ ] 8 条验收项 A1–A8 全部通过
