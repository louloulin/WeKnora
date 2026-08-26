-- Build #27 — reverse migration for wiki_pages.acl_snapshot_hash.
--
-- Drops the column added by 000102_wiki_pages_acl_snapshot_hash.up.sql.
-- No data preservation: down migration removes the column entirely.
-- Going down means every subsequent PutAcl loses the hash-skip
-- optimization and reverts to the Build #24 behaviour (always wipe).
-- This is a non-recoverable change for the skipped-wipe counts in
-- metric_acl_change_skipped_total; dashboards will simply stop
-- reporting those increments.

ALTER TABLE wiki_pages
    DROP COLUMN acl_snapshot_hash;
