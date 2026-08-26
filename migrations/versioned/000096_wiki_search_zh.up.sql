-- Migration 000096: wiki search zh + pg_trgm fuzzy support
--
-- Build #19.x — extends Build #19's `to_tsvector('simple', ...)` path with
--   (a) a separate `content_ts_zh` text column populated by gojieba at
--       write-time (jieba cannot run inside a PostgreSQL trigger because
--       the dictionary lives in the application). NULL is acceptable on
--       rows that pre-date this migration; the server backfill loop in
--       cmd/server picks them up.
--   (b) a defensive `pg_trgm` extension CREATE — already loaded by 000002
--       / 000041, but a fresh DB that lands on 000096 without the prior
--       migrations must still succeed.
--
-- Why a `text` column with an expression index instead of a stored `tsvector`
-- column? GORM passes strings as-is; PostgreSQL rejects arbitrary text into a
-- `tsvector` typed column. Storing the jieba-tokenized string and indexing it
-- with `to_tsvector('simple', content_ts_zh)` mirrors the existing pattern of
-- `idx_wiki_pages_fulltext` on `to_tsvector('simple', coalesce(title,'') || ' '
-- || coalesce(content,''))` (000037 line 73-74) — zero changes to the ORM,
-- the database does the lexeme work at query time.
--
-- The title trigram GIN index `idx_wiki_pages_title_trgm` was added by 000041
-- (line 145-146) on `lower(title)` — we intentionally reuse it instead of
-- recreating it on `title gin_trgm_ops`. `similarity(lower(title), q)` from
-- the search repo matches that index.

DO $$ BEGIN RAISE NOTICE '[Migration 000096] Applying wiki search zh + pg_trgm fuzzy schema'; END $$;

-- Defensive extension load. Safe no-op when 000002 / 000041 already ran.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- jieba-tokenized text — populated by service layer at write-time via
-- gojieba.CutForSearch(). NULL is acceptable on rows that pre-date this
-- migration; the server backfill loop in cmd/server picks them up.
ALTER TABLE wiki_pages
    ADD COLUMN IF NOT EXISTS content_ts_zh text;

-- Chinese GIN index — backs the `@@ plainto_tsquery('simple', $jieba)` arm
-- of the three-layer OR in WikiSearchV2Repository. The expression index
-- tokenizes the jieba output (already word-aligned) using the `simple`
-- regconfig so query-time `@@` works without a separate tsvector column.
CREATE INDEX IF NOT EXISTS idx_wiki_pages_content_ts_zh
    ON wiki_pages USING GIN (to_tsvector('simple', coalesce(content_ts_zh, '')));

DO $$ BEGIN RAISE NOTICE '[Migration 000096] wiki search zh + pg_trgm fuzzy schema applied successfully'; END $$;
