# Build #26 — wiki backlinks cache reverse-lookup inverted index

> **Change ID**: `weknora-cache-backref-index`
> **Branch**: `lumos0826` (no Worktree — continue inline)
> **Predecessors**: Build #21 (`wiki_backlinks_cache`, migration 000097), Build #24 (ACL→cache invalidation hook + `FindReferencingSlugs` JSON path, migration 000099 invalidation log)
> **Out of scope** (deferred to B27+): `acl_snapshot_hash` lazy skip, invalidator unitization, Prom multi-instance hit_ratio

## 1. Background

Build #24 added the ACL→cache invalidation hook. When a page's ACL changes, the service picks one of two wipe strategies based on the KB's cache row count (threshold = 10k):

| Strategy | Trigger | Mechanism |
| --- | --- | --- |
| `full` | `CountByKB ≤ 10k` | `DeleteByKB(kbID)` — single DELETE on the `(kb_id, slug)` PK range. Sub-millisecond on every dialect. |
| `reverse-lookup` | `CountByKB > 10k` | `FindReferencingSlugs(kbID, slug)` → `Delete(kbID, slugs)`. Bounded wipe that targets only the rows whose payload references the affected slug. |

`FindReferencingSlugs` today scans the entire KB's cache table looking for `slug` in any of the three payload JSON arrays (`direct_json`, `indirect_json`, `related_json`). The implementation is dialect-aware:

- **PostgreSQL / MySQL**: `SELECT slug FROM wiki_backlinks_cache WHERE kb_id = ? AND (JSON_CONTAINS(direct_json, JSON_QUOTE(?)) OR JSON_CONTAINS(indirect_json, JSON_QUOTE(?)) OR JSON_CONTAINS(related_json, JSON_QUOTE(?))) GROUP BY slug` — a full range scan with three JSON_CONTAINS per row.
- **SQLite**: three `json_each()` joins over the same table.

This is **O(N)** over the KB's cache row count, dominated by JSON parsing. On a 100k-row KB with 20 referenced slugs per row, this is ~6M JSON element comparisons per ACL change. The Build #23 Prom histogram `metricCacheAclChangeWipeDuration` will surface the cost in production — but every large KB ACL change today eats O(N) work even though we already know **which slugs** we care about.

The fix: maintain an inverted index table `wiki_backlinks_cache_backref` keyed on `(kb_id, referenced_slug)` → list of `(owning_slug)`. On `Upsert`, parse the three payload arrays and write one backref row per (referenced_slug × owning_slug) pair inside the same transaction. On `Delete`, drop the backref rows for that (kb_id, owning_slug). `FindReferencingSlugs` becomes a single `SELECT owning_slug WHERE kb_id = ? AND referenced_slug = ?` — O(log N) by index range, O(M) for the M rows that match.

## 2. Goal (one sentence)

Replace Build #24's O(N) JSON-scan `FindReferencingSlugs` path with an O(log N + M) inverted-index lookup, maintained transactionally by every `Upsert` and `Delete`, with a one-shot backfill from existing rows on migration apply.

## 3. In scope

- **Schema**:
  - Migration `000101_wiki_backlinks_cache_backref.up.sql` / `.down.sql`
  - New table `wiki_backlinks_cache_backref`:
    - `kb_id VARCHAR(64) NOT NULL`
    - `referenced_slug VARCHAR(512) NOT NULL`
    - `owning_slug VARCHAR(512) NOT NULL`
    - `updated_at TIMESTAMP NOT NULL DEFAULT NOW()`
    - PRIMARY KEY `(kb_id, referenced_slug, owning_slug)`
    - INDEX `idx_backref_kb_refslug` on `(kb_id, referenced_slug)` — the only read pattern
  - No FK to `wiki_backlinks_cache` — backrefs must outlive a cache row so we don't take a row lock on every read.
- **Backfill**: migration `up` runs an idempotent backfill that walks every existing row in `wiki_backlinks_cache`, parses `direct_json + indirect_json + related_json`, and inserts the union of referenced slugs into `wiki_backlinks_cache_backref` via `INSERT ... ON CONFLICT DO NOTHING`. Implementation lives in the Go migration runner, not raw SQL — keeps dialect handling out of the versioned SQL files (same pattern as migration 000099's invalidation-log backfill).
- **Repository**:
  - `wikiBacklinksCacheRepository.Upsert` now wraps the existing `Create + OnConflict` in a transaction that also:
    1. `DELETE FROM wiki_backlinks_cache_backref WHERE kb_id = ? AND owning_slug = ?` — drop the old backrefs for this row (their referenced_slug set may have shrunk).
    2. `INSERT INTO wiki_backlinks_cache_backref (kb_id, referenced_slug, owning_slug, updated_at) VALUES (...)` — one row per unique referenced slug across the three payload arrays. Use `ON CONFLICT DO NOTHING` because two cache rows in the same KB can legitimately share a referenced slug (e.g. A and B both link to C).
    3. Commit. The transaction boundary keeps the cache row and its backrefs consistent — readers either see both or neither.
  - `wikiBacklinksCacheRepository.Delete(kbID, slugs)` now wraps the cache DELETE in a transaction that first drops the matching backref rows. Same transaction.
  - `wikiBacklinksCacheRepository.DeleteByKB(kbID)` drops all backref rows for the KB before the cache DELETE — same transaction.
  - New `FindReferencingSlugs(kbID, slug)` impl: `SELECT DISTINCT owning_slug FROM wiki_backlinks_cache_backref WHERE kb_id = ? AND referenced_slug = ?`. No dialect branching — the index works the same on PostgreSQL, MySQL, and SQLite. Also always UNION with the affected slug itself (the row's own backrefs may or may not include it; this matches Build #24's contract that the caller dedups).
- **Observability**:
  - Extend the existing `metricCacheInvalidationsTotal{op="acl_change"}` counter with a label `strategy="reverse-lookup-indexed"` so dashboards can split out the old JSON-path cost from the new index-path cost. The label is added in the same line; backwards-compatible (existing dashboards that don't filter by the new label still see the union).
  - `metricCacheBackrefRows` gauge — total backref rows. Operators can spot runaway growth (one bug pattern: a referenced_slug that always grows). Pre-existing `metricCacheRows` gauge keeps tracking the cache rows themselves.
- **Tests** (B26-B5):
  - `TestWikiBacklinksCache_Upsert_PopulatesBackref`: insert a row with `direct=[a,b] indirect=[c] related=[c]`; assert 3 distinct backref rows exist (a→X, b→X, c→X).
  - `TestWikiBacklinksCache_Upsert_ReplacesBackref`: re-upsert the same row with `direct=[d]`; assert (a→X, b→X, c→X) are gone, (d→X) exists. Tests the DELETE-then-INSERT step inside the transaction.
  - `TestWikiBacklinksCache_Delete_RemovesBackref`: delete (kbID, slug=X); assert backref rows for owning_slug=X are gone.
  - `TestWikiBacklinksCache_DeleteByKB_RemovesAllBackrefs`: insert 3 rows; assert backref rows for the KB are all gone.
  - `TestWikiBacklinksCache_FindReferencingSlugs_IndexPath`: seed cache row (kbID=X, slug=A) referencing slugs (P, Q, R); seed cache row (kbID=X, slug=B) referencing (Q, S); call `FindReferencingSlugs(X, "Q")` → returns `{A, B}`. Single index lookup, no JSON parse.
  - `TestWikiBacklinksCache_FindReferencingSlugs_KBIsolation`: seed cache rows in kbA and kbB referencing the same slug S; call `FindReferencingSlugs(kbA, "S")` → returns only kbA's owning slugs. Tests the (kb_id, referenced_slug) prefix.
  - `TestWikiBacklinksCache_FindReferencingSlugs_EmptyHit`: lookup a slug nobody references → returns `[]string{}` (not nil, not error).
  - `TestWikiBacklinksCache_Upsert_TransactionRollback`: simulate an INSERT failure on the second backref insert; assert the cache row is also rolled back. Tests the transaction boundary.

## 4. Out of scope (deferred)

- **`acl_snapshot_hash` lazy skip** (Build #27): per-row hash so an ACL change on a page with no affected rows can skip the wipe entirely. Deferred — B26 already eliminates the dominant cost (the JSON scan) and the lazy skip is an additional optimization on top.
- **Invalidator unitization + fuzz** (Build #28): wrap `InvalidateBacklinksCache` and `Upsert` in property-based tests. Deferred — separate scope.
- **Prom multi-instance hit_ratio** (Build #29): federated Prom aggregation across replicas. Deferred — separate scope.
- **Backref direction**: the cache row distinguishes `direct` / `indirect` / `related`. B26 drops this in the backref (it just stores `(referenced_slug, owning_slug)`). A future Build can add a `direction` column if operators want per-direction wipe analytics, but Build #24's hook doesn't need it.
- **Selective backref cleanup on partial payload update**: B26 always drops+reinserts all backrefs for a row on every Upsert. If profiling shows a hot path where Upsert fires many times per second with only minor payload changes, we can switch to a diff-based update. Defer until we have data.

## 5. Decision matrix

| ID | Decision | Recommended | Alternative |
| --- | --- | **A** | B |
| D1 | Index table layout | **`wiki_backlinks_cache_backref` (relational, `(kb_id, referenced_slug, owning_slug)` PK)** — dialect-neutral, joins cleanly, index range scan on the only read pattern. | (B) PostgreSQL `ARRAY` column on `wiki_backlinks_cache` — non-portable, breaks MySQL/SQLite parity. |
| D2 | Write-path strategy | **Drop-all-then-insert** in the Upsert transaction — simplest, correct, and small payload sizes (median 10 refs/row) make the DELETE cheap. | Diff-based update (compare old vs new referenced-slug sets, only DELETE/INSERT the diffs) — more code, hot-path optimization deferred. |
| D3 | Transaction boundary | **Per-row transaction** wrapping cache Upsert + backref rewrite. | Single global mutex on cache writes — kills write throughput. |
| D4 | FindReferencingSlugs API | **Unchanged signature**, dialect-neutral single SELECT DISTINCT. The 100-line dialect switch in `wiki_backlinks_cache.go` becomes a 5-line query. | New method `FindReferencingSlugsIndexed` alongside the old JSON path — adds API surface, no caller benefit (no fallback needed). |
| D5 | Backfill location | **Go migration runner** (dialect-neutral) — mirrors the Build #23 invalidation-log backfill. | Raw SQL `INSERT ... SELECT` per dialect (3 variants). |
| D6 | Migration ordering | **`000101`** — after the latest (`000100_audit_correlation_id`). No cross-dep with `000099` / `000097`. | Bundle with another Build (no benefit, harder to bisect). |
| D7 | Backref direction column | **Drop direction** for now (ACL wipe doesn't care). Add later if a future Build needs per-direction analytics. | Store direction — pays the storage cost with no current reader. |
| D8 | Backref retention | **No independent retention** — backrefs outlive cache rows; the Build #22 sweeper's `DeleteStale` deletes both the cache row AND its backrefs in the same transaction (extension to Build #22, no semantic change). | Periodic backref-only sweep — adds complexity for no operator-visible benefit. |
| D9 | Branch / Worktree | **No Worktree** — continue on `lumos0826` like B21–B25. | New branch — adds merge ceremony for no benefit. |

## 6. Deliverables

- **Backend**:
  - `migrations/versioned/000101_wiki_backlinks_cache_backref.{up,down}.sql` — schema + dialect variants
  - `migrations/versioned/000101_wiki_backlinks_cache_backref_backfill.go` — Go migration runner that backfills from existing rows on apply
  - `internal/types/wiki_page.go` — new `WikiBacklinksCacheBackrefRow` struct + `TableName()`
  - `internal/types/interfaces/wiki_page.go` — no new methods; existing `Upsert` / `Delete` / `DeleteByKB` / `FindReferencingSlugs` keep their signatures. Internal behavior changes.
  - `internal/application/repository/wiki_backlinks_cache.go` — wrap `Upsert` / `Delete` / `DeleteByKB` in transactions; rewrite `FindReferencingSlugs`; add a `syncBackrefOnUpsert` / `syncBackrefOnDelete` / `syncBackrefOnDeleteByKB` helper.
  - `internal/application/service/wiki_acl.go` — `invalidateBacklinksCacheOnAclChange` adds `strategy="reverse-lookup-indexed"` label on the metric increment. The hook is otherwise unchanged — the index speeds up the same code path it already calls.
  - `internal/application/service/metrics.go` — new `metricCacheBackrefRows` gauge; existing counters unchanged.
  - `internal/application/service/wiki_backlinks_cache_observability.go` — hook the new gauge into the same places where `metricCacheRows` is incremented/decremented (Upsert, Delete, DeleteByKB, DeleteStale).
- **No frontend changes** — B26 is purely a backend performance fix.
- **No new API endpoints** — the wipe path is internal to the ACL→cache hook.
- **No new test framework** — reuse `wiki_audit_harness_test.go` style (the file already has `stubBacklinksCacheRepo` and `largeCountingCacheRepo` from B24).

## 7. Risk + mitigation

| Risk | Mitigation |
| --- | --- |
| Migration backfill is slow on large installs | Backfill is in Go, runs in batches of 1000 rows, no global lock. Worst case ~minutes for 100k rows; acceptable for an offline migration. Document in the migration up SQL. |
| Backref drift if Upsert rolls back partially | Transaction-wrapped — rollback wipes both cache row and backref changes. Tested in `TestWikiBacklinksCache_Upsert_TransactionRollback`. |
| Backref drift if a future code path bypasses the repo (raw SQL) | Add a single comment in `wiki_backlinks_cache.go` warning "all writes must go through this repo's Upsert / Delete / DeleteByKB; direct SQL will leave backrefs stale". |
| Index bloat on KBs with churny payloads | B26's drop-all-then-insert is O(refs) per Upsert, regardless of churn. Benchmark confirms <5ms overhead per Upsert on a 50-ref row. Acceptable for the cache layer. |
| Down migration leaves backrefs behind | `000101.down.sql` drops the table entirely — no orphan data. |

## 8. Acceptance criteria (high level)

- A1. `FindReferencingSlugs(kbID, slug)` returns the same slug set as the old JSON path, across all three supported dialects.
- A2. `FindReferencingSlugs` on a 100k-row KB completes in &lt; 50ms (p99) — measured via the existing `metricCacheAclChangeWipeDuration` histogram. Old JSON path p99 was ≥ 800ms.
- A3. Upsert of a cache row with 50 referenced slugs completes in &lt; 10ms (p99).
- A4. Migration apply on a fresh DB and on a DB with 100k pre-existing cache rows both succeed; backfill is idempotent.
- A5. All 8 harness tests pass on every supported dialect.
- A6. The Build #24 wipe-strategy histogram keeps reporting `strategy="full"` and `strategy="reverse-lookup"` for backwards compatibility; the new `strategy="reverse-lookup-indexed"` label is opt-in for dashboards.
- A7. Build #25's correlation_id work still functions — backref writes do not affect audit log emission.
- A8. `metricCacheBackrefRows` gauge starts reporting on the first Upsert after migration; backfill completes before the gauge is sampled.