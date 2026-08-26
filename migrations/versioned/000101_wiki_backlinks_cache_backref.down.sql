-- Migration 000101 down — drop the reverse-lookup inverted index.
--
-- Down-migration drops the index entirely; no data preservation. The
-- wiki_backlinks_cache table is unchanged (the index is a derivative
-- surface). After this down the FindReferencingSlugs callers fall back
-- to nothing — the codebase is designed to treat that as a hard error
-- and roll forward to 000101.

DROP INDEX IF EXISTS idx_wbc_backref_kb_refslug;
DROP TABLE IF EXISTS wiki_backlinks_cache_backref;