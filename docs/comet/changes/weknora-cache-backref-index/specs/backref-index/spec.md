# Build #26 — Reverse-lookup inverted index for ACL wipe

> Spec for `weknora-cache-backref-index`. Concrete acceptance matrix follows brief §8. Read brief first for context.

## 1. Schema (`migrations/versioned/000101_wiki_backlinks_cache_backref.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS wiki_backlinks_cache_backref (
    kb_id           VARCHAR(64)  NOT NULL,
    referenced_slug VARCHAR(512) NOT NULL,
    owning_slug     VARCHAR(512) NOT NULL,
    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (kb_id, referenced_slug, owning_slug)
);

CREATE INDEX IF NOT EXISTS idx_wbc_backref_kb_refslug
    ON wiki_backlinks_cache_backref (kb_id, referenced_slug);
```

Notes:
- Composite PK `(kb_id, referenced_slug, owning_slug)` is the only uniqueness invariant — two cache rows in the same KB can legitimately share a referenced slug (A links to C AND B links to C → two distinct backref rows for C).
- The `idx_wbc_backref_kb_refslug` is the only read pattern. No other indexes — writes already pay for the PK.
- Cross-dialect `TIMESTAMP` / `CURRENT_TIMESTAMP` / `INTEGER` plumbing lives in the per-dialect migration files (`000101_*.{pg,mysql,sqlite}.sql`) generated from this canonical source — same pattern as 000099.

## 2. Backfill (`migrations/versioned/000101_wiki_backlinks_cache_backfill.go`)

Runs in the migration runner's `Up()` step AFTER the table exists. Pseudocode:

```go
func backfillBackrefs(db *gorm.DB) error {
    const batchSize = 1000
    var lastKbID, lastSlug string
    for {
        var rows []WikiBacklinksCacheRow
        err := db.Where("kb_id > ? OR (kb_id = ? AND slug > ?)", lastKbID, lastKbID, lastSlug).
            Order("kb_id, slug").
            Limit(batchSize).
            Find(&rows).Error
        if errors.Is(err, gorm.ErrRecordNotFound) || len(rows) == 0 {
            return nil
        }
        if err != nil { return err }

        backrefs := make([]WikiBacklinksCacheBackrefRow, 0, len(rows)*10)
        for _, r := range rows {
            for _, slug := range uniqueReferencedSlugs(r) {
                backrefs = append(backrefs, WikiBacklinksCacheBackrefRow{
                    KbID: r.KbID, ReferencedSlug: slug, OwningSlug: r.Slug,
                    UpdatedAt: time.Now().UTC(),
                })
            }
        }
        if len(backrefs) == 0 {
            lastKbID, lastSlug = rows[len(rows)-1].KbID, rows[len(rows)-1].Slug
            continue
        }
        if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&backrefs).Error; err != nil {
            return err
        }
        lastKbID, lastSlug = rows[len(rows)-1].KbID, rows[len(rows)-1].Slug
    }
}
```

Idempotency comes from `ON CONFLICT DO NOTHING` on the PK. Re-running `000101.up` after a partial backfill is safe.

`uniqueReferencedSlugs` parses `direct_json + indirect_json + related_json` (each is `[]string`), unions, dedupes. Implemented in `internal/application/repository/parse_backref.go` — shared between the migration and the repo's Upsert path.

## 3. Repository changes (`internal/application/repository/wiki_backlinks_cache.go`)

### 3.1 Upsert — wrap in transaction, rewrite backref rows

```go
func (r *wikiBacklinksCacheRepository) Upsert(ctx context.Context, row *types.WikiBacklinksCacheRow) error {
    if row == nil { return errors.New("Upsert: nil row") }
    if row.KbID == "" || row.Slug == "" { return errors.New("Upsert: empty kb_id or slug") }
    now := time.Now().UTC()
    row.ComputedAt = now
    row.UpdatedAt = now

    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 1. Cache row upsert (Build #21 logic preserved).
        if err := tx.Clauses(clause.OnConflict{
            DoUpdates: clause.AssignmentColumns([]string{
                "direct_json", "indirect_json", "related_json", "broken_json",
                "stats_json", "source_event_id", "computed_at", "updated_at",
            }),
        }).Create(row).Error; err != nil { return err }

        // 2. Drop old backrefs for this (kb, owning_slug) — the referenced
        //    slug set may have shrunk.
        if err := tx.Where("kb_id = ? AND owning_slug = ?", row.KbID, row.Slug).
            Delete(&types.WikiBacklinksCacheBackrefRow{}).Error; err != nil { return err }

        // 3. Insert new backrefs.
        backrefs := backrefRowsFromPayload(row.KbID, row.Slug, now, row.DirectJSON, row.IndirectJSON, row.RelatedJSON)
        if len(backrefs) == 0 { return nil }
        return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&backrefs).Error
    })
}
```

`backrefRowsFromPayload` parses the three JSON arrays, unions + dedupes, and returns `[]WikiBacklinksCacheBackrefRow` (empty when the payload has no references, which is fine — the cache row still exists, just no backrefs).

### 3.2 Delete — wrap in transaction, drop backref rows

```go
func (r *wikiBacklinksCacheRepository) Delete(ctx context.Context, kbID string, slugs []string) (int64, error) {
    if kbID == "" || len(slugs) == 0 { return 0, nil }
    uniq := dedupSlugs(slugs)
    var affected int64
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("kb_id = ? AND owning_slug IN ?", kbID, uniq).
            Delete(&types.WikiBacklinksCacheBackrefRow{}).Error; err != nil { return err }
        res := tx.Where("kb_id = ? AND slug IN ?", kbID, uniq).
            Delete(&types.WikiBacklinksCacheRow{})
        if res.Error != nil { return res.Error }
        affected = res.RowsAffected
        return nil
    })
    return affected, err
}
```

### 3.3 DeleteByKB — wrap in transaction, drop all backref rows

```go
func (r *wikiBacklinksCacheRepository) DeleteByKB(ctx context.Context, kbID string) (int64, error) {
    if kbID == "" { return 0, nil }
    var affected int64
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("kb_id = ?", kbID).
            Delete(&types.WikiBacklinksCacheBackrefRow{}).Error; err != nil { return err }
        res := tx.Where("kb_id = ?", kbID).
            Delete(&types.WikiBacklinksCacheRow{})
        if res.Error != nil { return res.Error }
        affected = res.RowsAffected
        return nil
    })
    return affected, err
}
```

### 3.4 FindReferencingSlugs — single index SELECT

```go
func (r *wikiBacklinksCacheRepository) FindReferencingSlugs(ctx context.Context, kbID, slug string) ([]string, error) {
    if kbID == "" || slug == "" { return []string{}, nil }
    var rows []string
    err := r.db.WithContext(ctx).
        Raw(`SELECT owning_slug FROM wiki_backlinks_cache_backref
             WHERE kb_id = ? AND referenced_slug = ? GROUP BY owning_slug`,
            kbID, slug).
        Scan(&rows).Error
    if err != nil { return nil, err }
    if rows == nil { rows = []string{} }
    return rows, nil
}
```

The 100-line dialect switch in the old impl is gone. The read is a single index range scan on `(kb_id, referenced_slug)` → distinct owning_slug. Both PostgreSQL, MySQL, and SQLite handle the same SQL natively.

### 3.5 DeleteStale — extend to also drop backrefs

Build #22's `DeleteStale` deletes the cache rows; the new variant also drops the corresponding backref rows in the same transaction. Pure extension — Build #22's tests still pass.

## 4. Metric changes (`internal/application/service/metrics.go` + `wiki_backlinks_cache_observability.go`)

### 4.1 New gauge

```go
var metricCacheBackrefRows = prometheus.NewGauge(prometheus.GaugeOpts{
    Name: "wiki_backlinks_cache_backref_rows",
    Help: "Total number of rows in wiki_backlinks_cache_backref.",
})
```

Registered alongside `metricCacheRows` in the same `prometheus.MustRegister` block.

### 4.2 Hook into Upsert / Delete / DeleteByKB / DeleteStale

After a successful Upsert, increment by `len(backrefRowsFromPayload(...))`. After Delete, decrement by the rows affected × average backrefs/row (counted by a follow-up SELECT in the same transaction; or pre-delete snapshot). For simplicity, decrement by the same number of backref rows the Upsert would have created — `metricCacheBackrefRows.Inc()/Dec()` are paired with each cache `metricCacheRows.Inc()/Dec()`.

### 4.3 Wipe-strategy label

```go
// Old:
metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange)).Inc()
// New (in invalidateBacklinksCacheOnAclChange, when strategy == "reverse-lookup-indexed"):
metricCacheInvalidationsTotal.WithLabelValues(
    string(types.BacklinkCacheInvalidateAclChange), "reverse-lookup-indexed",
).Inc()
```

Prometheus counters accept variadic labels — existing dashboards that filter by the first label still see the union. New dashboards can split by strategy.

## 5. ACL hook change (`internal/application/service/wiki_acl.go`)

`invalidateBacklinksCacheOnAclChange` is otherwise unchanged. The strategy label changes from `"reverse-lookup"` (legacy) to `"reverse-lookup-indexed"` (new). The pre-existing metric histograms (`metricCacheAclChangeWipeDuration`) record the cost; B26 expects p99 to drop ≥ 10× for KBs > 10k rows.

## 6. Tests (`internal/application/service/wiki_backlinks_cache_backref_test.go`)

Eight harness cases (full names match §3 of brief):

| Test | Asserts |
| --- | --- |
| `TestWikiBacklinksCache_Upsert_PopulatesBackref` | After Upsert of `(kb=X, slug=A, direct=[a,b] indirect=[c] related=[c])`, three backref rows exist with `owning_slug=A`: `(X, a, A)`, `(X, b, A)`, `(X, c, A)`. Direction is dropped — only the slug matters. |
| `TestWikiBacklinksCache_Upsert_ReplacesBackref` | After Upsert of `(X, A, direct=[a,b] indirect=[c] related=[c])` followed by Upsert of `(X, A, direct=[d] indirect=[] related=[])`, backref set is `{(X, d, A)}` only — the (a,b,c)→A rows are gone. |
| `TestWikiBacklinksCache_Delete_RemovesBackref` | After Upsert + Delete, `SELECT COUNT(*) FROM backref WHERE owning_slug=A` is 0. Other rows in the same KB are unaffected. |
| `TestWikiBacklinksCache_DeleteByKB_RemovesAllBackrefs` | After Upsert 3 rows in KB X, `DeleteByKB(X)` leaves 0 backref rows. Other KB's backrefs untouched. |
| `TestWikiBacklinksCache_FindReferencingSlugs_IndexPath` | Seed two rows (X, A) referencing (P, Q, R) and (X, B) referencing (Q, S). `FindReferencingSlugs(X, "Q")` returns `{"A", "B"}`. (Hits both rows that reference Q.) |
| `TestWikiBacklinksCache_FindReferencingSlugs_KBIsolation` | Seed (X, A) referencing S and (Y, B) referencing S. `FindReferencingSlugs(X, "S")` returns `{"A"}` only — (Y, B) is in a different KB. |
| `TestWikiBacklinksCache_FindReferencingSlugs_EmptyHit` | `FindReferencingSlugs(X, "nobody-references-this")` returns `[]string{}` (not nil, not error). |
| `TestWikiBacklinksCache_Upsert_TransactionRollback` | Inject a fault on the backref INSERT step (test stub repo panics). Verify the cache row is also absent — no partial state. Uses an in-memory SQLite or a test repo that exposes a fault-injection hook. |

The first 7 tests run on the existing harness repo (`wiki_audit_harness_test.go`'s `stubBacklinksCacheRepo` was extended by Build #24 — needs a small extension here for the backref table). The 8th test uses a separate test-only repo (a new `faultInjectingCacheRepo` wrapping the real one) to keep the production stub free of fault-injection branches.

## 7. Migration order + branch

- Migrations: `000101_wiki_backlinks_cache_backref.up.sql` then `000101_wiki_backlinks_cache_backfill.go`. Same migration number, same versioned directory.
- Backfill is implemented in Go (per D5) — runs after the table is created, before the migration is marked complete. If backfill fails, the migration rolls back the table creation.
- Branch: `lumos0826` (D9 — no Worktree). Single commit `Build #26: reverse-lookup index for ACL wipe`.

## 8. Acceptance matrix (cross-ref to brief §8)

| Brief ID | Spec section | Verifier check |
| --- | --- | --- |
| A1 | §3.4 + §6 | `TestWikiBacklinksCache_FindReferencingSlugs_IndexPath` and `_EmptyHit` cover parity with the old JSON path on the test fixtures. Production parity is asserted by re-running the Build #24 `_harness` test suite unchanged — it calls `FindReferencingSlugs` and expects the same results. |
| A2 | §3.4 | `metricCacheAclChangeWipeDuration` histogram. Manual benchmark on a synthetic 100k-row KB. |
| A3 | §3.1 | Manual benchmark on a 50-ref payload. |
| A4 | §2 | `migrations/versioned/000101_*_backfill_test.go` (new) — applies migration twice against a fixture DB and asserts no duplicate backref rows. |
| A5 | §6 | All 8 tests pass on PostgreSQL, MySQL, SQLite (CI matrix). |
| A6 | §4.3 | Existing tests that read `metricCacheInvalidationsTotal` (Build #23 harness) still pass without modification. |
| A7 | §3 | Build #25 harness test `TestWikiAuditService_ListAuditEvents_CorrelationIDFilter_AllSources_HTTPRequest` still passes — the audit log write path is untouched. |
| A8 | §4.1 + §4.2 | The gauge is registered in the same `init()` block as `metricCacheRows`; CI smoke confirms both gauges start at 0. |