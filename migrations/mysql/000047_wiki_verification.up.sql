-- v0.7.38 Build #48 — Verified Knowledge Engine (MySQL flavor).
--
-- Same columns as 000052_wiki_verification.up.sql on sqlite; uses
-- DATETIME rather than DATETIME NULL and an explicit NULL default
-- so legacy rows that pre-date the migration don't need a backfill
-- script.
ALTER TABLE wiki_pages
    ADD COLUMN review_owner   VARCHAR(64)  NOT NULL DEFAULT '' AFTER last_editor_id,
    ADD COLUMN review_due_at  DATETIME     NULL          AFTER review_owner,
    ADD COLUMN verified_at    DATETIME     NULL          AFTER review_due_at,
    ADD COLUMN verified_by    VARCHAR(64)  NOT NULL DEFAULT '' AFTER verified_at;

CREATE INDEX idx_wiki_pages_review_owner ON wiki_pages (review_owner, review_due_at);
CREATE INDEX idx_wiki_pages_review_due   ON wiki_pages (review_due_at);
CREATE INDEX idx_wiki_pages_verified_at  ON wiki_pages (verified_at);
