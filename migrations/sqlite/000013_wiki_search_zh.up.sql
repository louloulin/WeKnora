-- Mirrors versioned migration 000096_wiki_search_zh:
-- tokenized Chinese search text persisted on wiki pages.

ALTER TABLE wiki_pages ADD COLUMN content_ts_zh TEXT;

