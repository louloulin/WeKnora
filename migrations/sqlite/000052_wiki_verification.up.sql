-- v0.7.38 Build #48 — Verified Knowledge Engine.
--
-- Adds 4 columns to wiki_pages so a page can be tracked through a
-- human-driven review cycle:
--
--   review_owner   user id of the accountable reviewer
--   review_due_at  next deadline (NULL = no schedule)
--   verified_at    last manual verification timestamp
--   verified_by    user id who last verified
--
-- Each column is index-eligible so the dashboard queries:
--   - "pages I own that are due"      (review_owner + review_due_at)
--   - "pages verified in the last N days" (verified_at)
-- can run without scanning the full wiki_pages table.
ALTER TABLE wiki_pages ADD COLUMN review_owner   VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE wiki_pages ADD COLUMN review_due_at  DATETIME;
ALTER TABLE wiki_pages ADD COLUMN verified_at    DATETIME;
ALTER TABLE wiki_pages ADD COLUMN verified_by    VARCHAR(64)  NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_wiki_pages_review_owner   ON wiki_pages (review_owner, review_due_at);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_review_due     ON wiki_pages (review_due_at);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_verified_at    ON wiki_pages (verified_at);
