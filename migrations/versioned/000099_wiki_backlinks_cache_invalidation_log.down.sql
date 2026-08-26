-- Migration 000099 down — drop invalidation log support.

DROP INDEX IF EXISTS idx_wbc_invalidation_log_kb_created;
DROP TABLE IF EXISTS wiki_backlinks_cache_invalidation_log;