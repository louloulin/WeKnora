DROP INDEX idx_wiki_pages_verified_at  ON wiki_pages;
DROP INDEX idx_wiki_pages_review_due   ON wiki_pages;
DROP INDEX idx_wiki_pages_review_owner ON wiki_pages;
ALTER TABLE wiki_pages DROP COLUMN verified_by;
ALTER TABLE wiki_pages DROP COLUMN verified_at;
ALTER TABLE wiki_pages DROP COLUMN review_due_at;
ALTER TABLE wiki_pages DROP COLUMN review_owner;
