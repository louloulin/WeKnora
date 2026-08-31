DROP INDEX IF EXISTS idx_wiki_pages_verified_at;
DROP INDEX IF EXISTS idx_wiki_pages_review_due;
DROP INDEX IF EXISTS idx_wiki_pages_review_owner;
ALTER TABLE wiki_pages DROP COLUMN verified_by;
ALTER TABLE wiki_pages DROP COLUMN verified_at;
ALTER TABLE wiki_pages DROP COLUMN review_due_at;
ALTER TABLE wiki_pages DROP COLUMN review_owner;
