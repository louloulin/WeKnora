-- Migration 000096 down — drop search-zh + fuzzy additions
--
-- Reverses 000096_wiki_search_zh.up.sql. The pg_trgm extension and the
-- title trigram index are NOT dropped because they pre-date this
-- migration (000002 / 000041) and may be used by other features.

DROP INDEX IF EXISTS idx_wiki_pages_content_ts_zh;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS content_ts_zh;
