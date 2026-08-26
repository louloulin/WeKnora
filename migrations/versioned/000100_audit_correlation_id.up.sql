-- Migration 000100: audit_correlation_id
--
-- Build #25 — cross-source `source_event_id` correlation.
--
-- The B24 unified audit endpoint merges 4 sources (audit_logs +
-- wiki_batch_job_audit + wiki_backlinks_cache_invalidation_log +
-- wiki_page_acl_audit) into one envelope. Without a correlation_id
-- stamped on each row from the same request, the operator sees four
-- parallel timelines and has to manually join them by timestamp.
--
-- This migration adds a `correlation_id VARCHAR(64)` column to:
--   - audit_logs
--   - wiki_batch_job_audit
--   - wiki_page_acl_audit
-- `wiki_backlinks_cache_invalidation_log` already has `source_event_id
-- VARCHAR(64)` from migration 000099; we'll reuse that column rather
-- than rename (the column type is identical).
--
-- The value is `types.RequestIDFromContext(ctx)` — set by
-- middleware.RequestID() at the HTTP edge from the inbound X-Request-ID
-- header (or generated when absent). Background jobs (sweeper, batch
-- workers) stamp their own correlation_id with the prefix
-- `sweep:<uuid>` / `batch:<job_id>` so audit rows from background work
-- can be traced back to the originating job.
--
-- Schema choice:
--   - VARCHAR(64) — fits a UUIDv4 (36 chars) with 28 chars of headroom
--     for the `sweep:` / `batch:` prefixes.
--   - NULLABLE — historical rows (pre-migration) stay NULL; the
--     frontend shows "—" instead of a chip for those rows. No backfill.
--   - INDEX on each table — the only read pattern is "show me all rows
--     sharing this correlation_id" (one-off debug), so a single btree
--     on correlation_id is the right shape.
--
-- Forward-only contract: no destructive change. The down migration
-- drops the columns and indexes.

ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(64);

ALTER TABLE wiki_batch_job_audit
    ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(64);

ALTER TABLE wiki_page_acl_audit
    ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(64);

-- wiki_backlinks_cache_invalidation_log.source_event_id already exists
-- from 000099; rename it to correlation_id so the 4-table join key is
-- uniform across all sources. The column type matches (VARCHAR(64)).
-- IF NOT EXISTS rename is PG 9.6+ which WeKnora's PG dialect requires
-- (paradedb tests are pinned to PG 14+). MySQL/SQLite versions of this
-- migration use the dialect-specific rename syntax.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'wiki_backlinks_cache_invalidation_log'
          AND column_name = 'source_event_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'wiki_backlinks_cache_invalidation_log'
          AND column_name = 'correlation_id'
    ) THEN
        ALTER TABLE wiki_backlinks_cache_invalidation_log
            RENAME COLUMN source_event_id TO correlation_id;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_audit_logs_correlation
    ON audit_logs (correlation_id)
    WHERE correlation_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wbb_audit_correlation
    ON wiki_batch_job_audit (correlation_id)
    WHERE correlation_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wiki_page_acl_audit_correlation
    ON wiki_page_acl_audit (correlation_id)
    WHERE correlation_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wbc_invalidation_log_correlation
    ON wiki_backlinks_cache_invalidation_log (correlation_id)
    WHERE correlation_id IS NOT NULL;
