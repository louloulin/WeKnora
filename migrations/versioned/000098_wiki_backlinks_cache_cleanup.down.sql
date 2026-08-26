-- Migration 000098 down — drop the cleanup support index.

DROP INDEX IF EXISTS idx_wiki_backlinks_cache_updated_at;
