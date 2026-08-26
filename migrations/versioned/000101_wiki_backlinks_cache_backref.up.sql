-- Migration 000101: wiki_backlinks_cache_backref
--
-- Build #26 — reverse-lookup inverted index for ACL→cache wipes.
--
-- The Build #24 ACL→cache hook calls FindReferencingSlugs on every
-- ACL change for KBs > 10k cache rows. The original implementation
-- scans the entire KB's cache table, parsing three JSON arrays per
-- row with JSON_CONTAINS (PG/MySQL) or three json_each joins (SQLite).
-- O(N) work, dominated by JSON element comparisons.
--
-- This migration adds a relational inverted index: every (cache row ×
-- referenced slug) pair becomes one row in wiki_backlinks_cache_backref.
-- FindReferencingSlugs becomes a single indexed range scan:
--
--   SELECT owning_slug FROM wiki_backlinks_cache_backref
--    WHERE kb_id = ? AND referenced_slug = ?
--    GROUP BY owning_slug
--
-- The repo maintains the index transactionally on every Upsert / Delete
-- (see internal/application/repository/wiki_backlinks_cache.go). This
-- migration only creates the schema and backfills existing rows.
--
-- Schema choice:
--   - Composite PK (kb_id, referenced_slug, owning_slug). Two cache
--     rows in the same KB can legitimately reference the same slug
--     (A links to C and B links to C → two distinct backref rows for C).
--   - The only read pattern is (kb_id, referenced_slug) → owning_slug,
--     so we add a covering index on those two columns. The PK already
--     provides this prefix scan, but a named index makes intent clear
--     and lets the planner pick it independently.
--   - VARCHAR(512) on slugs matches wiki_backlinks_cache.slug.
--   - No FK to wiki_backlinks_cache: a cache row may be deleted before
--     its backref rows (e.g. Build #22 sweep), and we don't want a
--     constraint to keep the cache row alive past its eviction. The
--     repo guarantees consistency; backrefs outliving their cache row
--     for a moment is harmless (FindReferencingSlugs returns owning
--     slugs whose cache row doesn't exist → caller dedupes against
--     current state).
--   - No direction column: the ACL hook doesn't care if the reference
--     was direct / indirect / related. Add later if a future Build
--     needs per-direction analytics.
--
-- Backfill: at migration time we walk the existing wiki_backlinks_cache
-- rows and emit one backref row per (owning_slug, referenced_slug) pair.
-- Idempotent via ON CONFLICT DO NOTHING on the composite PK.
--
-- Cross-dialect: this is the PostgreSQL canonical source. MySQL has
-- equivalent JSON_TABLE syntax (8.0+) — operators on MySQL adapt the
-- backfill section if their install is non-empty. SQLite uses json_each
-- (1.5+) — see the SQLite test migration path which doesn't yet run
-- past 000012.
--
-- The Go side keeps the index hot: every Upsert deletes its old backrefs
-- in the same transaction, then inserts the new set with ON CONFLICT
-- DO NOTHING. Delete / DeleteByKB drop the matching backref rows in the
-- same transaction. See wiki_backlinks_cache.go for the transactional
-- wiring.

CREATE TABLE IF NOT EXISTS wiki_backlinks_cache_backref (
    kb_id           VARCHAR(64)  NOT NULL,
    referenced_slug VARCHAR(512) NOT NULL,
    owning_slug     VARCHAR(512) NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (kb_id, referenced_slug, owning_slug)
);

CREATE INDEX IF NOT EXISTS idx_wbc_backref_kb_refslug
    ON wiki_backlinks_cache_backref (kb_id, referenced_slug);

-- Backfill from existing wiki_backlinks_cache rows. Uses LATERAL +
-- jsonb_array_elements_text to flatten the three payload arrays into a
-- stream of referenced slugs, paired with each row's owning slug. The
-- DISTINCT collapses duplicates from rows that reference the same slug
-- in multiple sections (direct + indirect + related). ON CONFLICT
-- DO NOTHING makes the backfill idempotent — re-running 000101 on a
-- partially-backfilled DB is a no-op.
--
-- Defensive filters: skip rows whose payload is not a JSON array (empty
-- string, malformed value from a pre-Build #21 row), and skip null /
-- empty slug elements. These are edge cases that never occur in
-- production but make the migration robust against hand-edited rows.
INSERT INTO wiki_backlinks_cache_backref
    (kb_id, referenced_slug, owning_slug, updated_at)
SELECT DISTINCT
    wbc.kb_id,
    ref.value        AS referenced_slug,
    wbc.slug         AS owning_slug,
    NOW()            AS updated_at
FROM wiki_backlinks_cache wbc
CROSS JOIN LATERAL (
    SELECT jsonb_array_elements_text(wbc.direct_json::jsonb)   AS value
      WHERE jsonb_typeof(wbc.direct_json::jsonb)   = 'array'
    UNION ALL
    SELECT jsonb_array_elements_text(wbc.indirect_json::jsonb) AS value
      WHERE jsonb_typeof(wbc.indirect_json::jsonb) = 'array'
    UNION ALL
    SELECT jsonb_array_elements_text(wbc.related_json::jsonb)  AS value
      WHERE jsonb_typeof(wbc.related_json::jsonb)  = 'array'
) ref
WHERE ref.value IS NOT NULL
  AND ref.value <> ''
ON CONFLICT (kb_id, referenced_slug, owning_slug) DO NOTHING;