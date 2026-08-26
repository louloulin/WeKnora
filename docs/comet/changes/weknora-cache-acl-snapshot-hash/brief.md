# Build #27 — acl_snapshot_hash lazy skip

> **Change ID**: `weknora-cache-acl-snapshot-hash`
> **Branch**: `lumos0826` (no Worktree — continue inline)
> **Predecessors**: Build #24 (ACL→cache invalidation hook), Build #26 (reverse-lookup inverted index, migration 000101)
> **Out of scope** (deferred to B28+): invalidator unitization, Prom multi-instance hit_ratio

## 1. Background

Build #24 wired `PutAcl` → `invalidateBacklinksCacheOnAclChange`. Every successful `PutAcl` triggers a cache wipe, even when the new ACL payload is byte-identical to the previous one (a real pattern: re-submitted forms, idempotent retries, double-clicks that hit the optimistic lock path). On a large KB the wipe cost dominates — even with Build #26's indexed reverse-lookup, a `CountByKB` + `DeleteByKB` (or `FindReferencingSlugs` + `Delete`) for a 100k-row KB still costs ~50ms of cache work plus a `LogInvalidation` audit row write.

Two visible signals in production already confirm this is wasted work:

1. **`metricCacheInvalidationsTotal{op="acl_change"}` counter** — when the operator looks at the dashboards, every "ACL change" click increments the counter even though the ACL didn't actually change.
2. **`wiki_backlinks_cache_invalidation_log`** — rows pile up where `affected_count=0` and `details.before_mode == details.after_mode`. Operators chasing cache-related incidents wade through these noise rows.

The fix: store a small per-row fingerprint of the ACL payload on the same row. Compare fingerprints in `PutAcl`; if equal, short-circuit — no wipe, no audit row, but still bump the revision by 1 (so the optimistic-lock round-trip has a real `next-revision` value to read back) and write the audit row tagged `action="noop_match"` so the "did anything actually change" question is answerable.

## 2. Goal (one sentence)

Add an `acl_snapshot_hash` column to `wiki_pages` and skip the Build #24 cache wipe + invalidation-log row when the new ACL payload's hash matches the stored hash, while still incrementing the revision and writing a `noop_match` audit row so the optimistic-lock + audit-trail invariants hold.

## 3. In scope

- **Schema**:
  - Migration `000102_wiki_pages_acl_snapshot_hash.{up,down}.sql`
  - New column on `wiki_pages`: `acl_snapshot_hash VARCHAR(16) NOT NULL DEFAULT ''` — first 16 hex chars (64 bits) of SHA-256 over the canonical ACL JSON. 16 hex chars is enough collision resistance for "did this row change?" (birthday-bound collision at 2^32 rows).
  - Backfill: existing rows get `''` (empty hash) which never matches a real hash, so the first `PutAcl` on a legacy row always runs the wipe path — that's the correct, safe default.
- **Repository**:
  - `wikiAclRepository.GetAclBySlug` adds `acl_snapshot_hash` to its SELECT projection and returns the value on the result. New helper `GetAclHashBySlug(kbID, slug)` if any caller needs just the hash.
  - `wikiAclRepository.UpdateAclWithRevision` writes the new hash in the same `UPDATE wiki_pages SET acl=?, acl_revision=?, acl_snapshot_hash=?` statement. Hash is computed at the call site (service layer) and passed in.
- **Service**:
  - `wikiAclService.PutAcl`:
    1. Reads the current ACL via `GetAclBySlug` (already does this for `beforeMode`).
    2. Computes `newHash = hashOfAcl(req.Mode, req.AllowUserIDs, req.AllowGroupIDs, req.DenyInherited)` — deterministic, sorted-slice canonicalization so `{a,b}` and `{b,a}` hash the same.
    3. If `currentHash == newHash`, sets a `noop=true` flag and passes it down. The cache wipe + invalidation log row are skipped; the audit row still gets written with `action="noop_match"`.
    4. Otherwise, normal flow: write the hash, run `invalidateBacklinksCacheOnAclChange`, log an `acl_change` invalidation-log row.
  - `wikiAclService.invalidateBacklinksCacheOnAclChange` becomes no-op (returns immediately, no metric increment, no log) when called with `noop=true`. The hook keeps its existing signature; `noop` is an extra parameter with default `false` so call sites without the flag continue working.
- **Canonical hash function** (new file `internal/application/service/wiki_acl_hash.go`):
  - Input: `mode`, `allowUserIDs []string`, `allowGroupIDs []string`, `denyInherited bool`.
  - Algorithm: sort both ID slices (stable, `strings.Sort`), then `json.Marshal` a canonical struct, then `sha256.Sum256`, then `hex.EncodeToString` truncated to 16 chars.
  - Deterministic across processes and dialects — the JSON encoder order is fixed by the canonical struct's field order.
- **Observability**:
  - New counter `metric_acl_change_skipped_total{reason="hash_match"}` — operators can spot how often the skip fires.
  - Existing `metricCacheInvalidationsTotal{op="acl_change"}` counter is NOT incremented on skip — that's the point.
- **Tests** (B27-B4):
  - `TestPutAcl_IdenticalPayload_SkipsWipe`: seed a page with `mode=public`; call `PutAcl` with the same payload; assert (a) revision incremented by 1, (b) audit row has `action="noop_match"`, (c) no row added to `wiki_backlinks_cache_invalidation_log`, (d) `metricCacheInvalidationsTotal{op="acl_change"}` unchanged.
  - `TestPutAcl_DifferentPayload_RunsWipe`: regression — different mode triggers full wipe (Build #24 behavior intact).
  - `TestPutAcl_ReorderedAllowList_HashMatches`: seed `allow_user_ids=[a,b]`; submit `allow_user_ids=[b,a]`; assert hash equal, skip fires. Tests the sort canonicalization.
  - `TestPutAcl_LegacyRow_AlwaysWipes`: pre-000102 row with `acl_snapshot_hash=""`; any `PutAcl` triggers wipe (because empty hash never matches). Tests the safe-default backfill behavior.
  - `TestPutAcl_HashPersistedAcrossReads`: write a new ACL; immediately read it back; assert `acl_snapshot_hash` round-trips. Tests the column wiring in `UpdateAclWithRevision`.
  - `TestAclHash_Deterministic`: hash the same ACL 1000 times in a tight loop; all hashes equal. Tests for accidental non-determinism (e.g. map iteration order).

## 4. Out of scope (deferred)

- **Hash mismatch audit reason** (Build #28+): when the wipe runs, log `details.reason="changed_mode"` vs `details.reason="changed_allow_list"` vs `details.reason="changed_deny_inherited"` so operators can see *what* changed, not just *that* something changed. Skipped — current Details JSON already captures `before_mode`/`after_mode`, and the more granular reason is a UX sugar.
- **Skip invalidation-log on noop** (decision: keep the row): some operators want the "you tried to change ACL but it was the same" trail. Audit table is cheap; readers prefer the noise.
- **Force-write flag** (e.g. `?force=true`): lets a paranoid caller bypass the hash check. Skipped — YAGNI; if the hash is wrong the worst case is one wiped cache row, which self-heals on next read.
- **Hash for shared-link tokens, page templates, batch-jobs**: same pattern would apply but those have their own (smaller) write paths. Defer until a profiler points there.
- **PostgreSQL `pgcrypto` hash** (could use `digest()` from `pgcrypto` instead of Go-side SHA-256): the Go-side approach is dialect-neutral, matches the Build #24/26 style, and avoids a `CREATE EXTENSION` privilege ask.

## 5. Decision matrix

| ID | Decision | Recommended | Alternative |
| --- | --- | **A** | B |
| D1 | Hash storage location | **`acl_snapshot_hash` column on `wiki_pages`** (sibling of `acl`, `acl_revision`). One row read = one hash read. | Separate `wiki_page_acl_hashes` table — extra JOIN per PutAcl, no benefit. |
| D2 | Hash algorithm | **SHA-256 truncated to 16 hex chars** (64 bits). Computed Go-side from canonical JSON. | (B) xxHash — faster but no stdlib, requires new dep; the perf delta is microseconds, irrelevant next to a 50ms cache wipe. |
| D3 | Canonicalization | **Sort both ID slices** then `json.Marshal` a fixed struct. Deterministic, dialect-neutral. | (B) Store sorted JSON in the `acl` column itself — bigger change, breaks Build #24 audit row JSON shape. |
| D4 | Backfill behavior | **Default `''` (empty hash) for legacy rows** — first PutAcl on a legacy row always wipes. Safe, no migration backfill needed. | (B) Compute-and-write hashes for all existing rows on migration apply — slow, no benefit (the first PutAcl on a legacy row will compute the same hash anyway). |
| D5 | Audit row on skip | **Still write the audit row, tagged `action="noop_match"`**. Maintains the "every ACL write has an audit row" invariant. | (B) Skip the audit row too — would leave gaps in the audit timeline, harder to debug. |
| D6 | Invalidation-log row on skip | **Skip the row** (it's the cache-layer's "did we actually wipe" trail — nothing happened, no row). | (B) Write a row tagged `skipped=true` — operators already complain about log noise. |
| D7 | Counter on skip | **Increment `metric_acl_change_skipped_total{reason="hash_match"}`** — operators want visibility. | (B) Silent — bad for dashboards, hides the optimization working. |
| D8 | Migration number | **`000102`** — after `000101`. No cross-dep with prior migrations. | Bundle with another Build — harder to bisect. |
| D9 | Branch / Worktree | **No Worktree** — continue on `lumos0826`. | New branch — adds merge ceremony for no benefit. |

## 6. Deliverables

- **Backend**:
  - `migrations/versioned/000102_wiki_pages_acl_snapshot_hash.{up,down}.sql` — add/drop the column on `wiki_pages`.
  - `internal/types/wiki_page.go` — `WikiPageAcl` struct gains no new fields (hash is computed externally, not stored on the ACL value).
  - `internal/types/interfaces/wiki_page.go` — `WikiAclRepo.UpdateAclWithRevision` gains a `snapshotHash string` parameter. `WikiAclRepo.GetAclBySlug` return gains a `SnapshotHash string` field on a new return-shape — OR a parallel `GetAclHashBySlug(kbID, slug) (string, error)` method. (Decision: new field on the return, mirror `Revision`.)
  - `internal/application/repository/wiki_acl.go` — `aclColumnProjection` extended with `acl_snapshot_hash`; `aclRow` struct gains the field; `UpdateAclWithRevision` SETs the new column.
  - `internal/application/service/wiki_acl_hash.go` (NEW) — `HashAcl(mode, allowUserIDs, allowGroupIDs, denyInherited) string`.
  - `internal/application/service/wiki_acl.go` — `PutAcl` computes + compares hash, passes `noop` flag; `invalidateBacklinksCacheOnAclChange` accepts `noop` and short-circuits; `logAclChange` accepts the action override.
  - `internal/application/service/metrics.go` — new `metricAclChangeSkippedTotal` counter.
  - `internal/application/service/wiki_acl_test.go` + `wiki_audit_harness_test.go` — update stubs (new parameter, new return field) and add the 6 new tests.
- **No frontend changes** — Build #27 is a backend optimization. The frontend already sends the full ACL on every PUT; from its perspective the behavior is identical (revision goes up, dialog closes).
- **No new API endpoints** — the wipe is internal to the ACL→cache hook.
- **No new test framework** — reuse existing harness patterns.

## 7. Risk + mitigation

| Risk | Mitigation |
| --- | --- |
| Hash collision (two different ACLs hash to the same 16 hex chars) | 64-bit truncation has 2^32 birthday bound. For the cache wipe use case, a collision means one skipped wipe — the next read recomputes on cache miss and self-heals. Acceptable. |
| Canonicalization bug (non-deterministic hash → skip never fires) | `TestAclHash_Deterministic` covers it. 1000-iteration loop. |
| Legacy row edge case (pre-000102 row with `''` hash) | Documented in D4; safe default is "always wipe" until the row's first write. |
| Audit row noise (operators already complain about `affected_count=0` rows) | The `action="noop_match"` tag lets dashboards filter them. The cache invalidation log row is what we're skipping — that's the high-volume one. |
| Performance: hash computation on every PutAcl | SHA-256 over <1KB JSON is <10µs. Negligible next to the cache wipe it skips. |
| Frontend sends the same payload twice for a reason (e.g. retry after timeout) | The skip is correct behavior — the second call IS a no-op. The revision still bumps by 1 so the optimistic-lock invariant holds. |

## 8. Acceptance criteria (high level)

- A1. `PutAcl` with an identical payload (modulo slice ordering) skips the cache wipe entirely — no row in `wiki_backlinks_cache_invalidation_log`, no increment on `metricCacheInvalidationsTotal{op="acl_change"}`.
- A2. `PutAcl` with an identical payload still increments `acl_revision` by 1 and writes a `wiki_page_acl_audit` row tagged `action="noop_match"`.
- A3. `PutAcl` with a different payload runs the full Build #24 wipe path (regression — no behavior change for the non-noop case).
- A4. `acl_snapshot_hash` round-trips: written on `UpdateAclWithRevision`, read back by `GetAclBySlug`.
- A5. Legacy rows (`acl_snapshot_hash=""`) always run the wipe on first PutAcl — safe default backfill behavior.
- A6. New counter `metric_acl_change_skipped_total{reason="hash_match"}` increments on each skip.
- A7. All 6 harness tests pass on every supported dialect.
- A8. Migration 000102 applies cleanly on a fresh DB and on a DB with pre-existing `wiki_pages` rows.
