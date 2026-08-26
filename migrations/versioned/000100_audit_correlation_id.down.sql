-- Migration 000100 down — drop audit_correlation_id support.

DROP INDEX IF EXISTS idx_audit_logs_correlation;
DROP INDEX IF EXISTS idx_wbb_audit_correlation;
DROP INDEX IF EXISTS idx_wiki_page_acl_audit_correlation;
DROP INDEX IF EXISTS idx_wbc_invalidation_log_correlation;

ALTER TABLE audit_logs DROP COLUMN IF EXISTS correlation_id;
ALTER TABLE wiki_batch_job_audit DROP COLUMN IF EXISTS correlation_id;
ALTER TABLE wiki_page_acl_audit DROP COLUMN IF EXISTS correlation_id;

-- Restore the original source_event_id column on the invalidation log.
-- We dropped the rename in `up` only inside a transaction-like DO block,
-- so a `down` reversal is safe to re-add the original column. Operators
-- who rollback should be aware that any correlation_id written between
-- up and down is lost.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'wiki_backlinks_cache_invalidation_log'
          AND column_name = 'source_event_id'
    ) AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'wiki_backlinks_cache_invalidation_log'
          AND column_name = 'correlation_id'
    ) THEN
        ALTER TABLE wiki_backlinks_cache_invalidation_log
            RENAME COLUMN correlation_id TO source_event_id;
    END IF;
END $$;
