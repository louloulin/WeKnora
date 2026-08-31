-- v0.7.25 — Wiki page comments (Build #22)
-- Adds reply / resolve / anchor support for wiki page discussions.
-- Migration is idempotent so re-runs on a partially-applied DB are safe.
CREATE TABLE IF NOT EXISTS wiki_page_comments (
    id                VARCHAR(36) PRIMARY KEY,
    tenant_id         BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    page_slug         TEXT    NOT NULL,
    parent_id         VARCHAR(36),
    body              TEXT    NOT NULL,
    mentions          JSONB   NOT NULL DEFAULT '[]'::jsonb,
    anchor_block_id   VARCHAR(64),
    author_id         VARCHAR(64) NOT NULL,
    author_name       TEXT    NOT NULL,
    author_avatar_url TEXT,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at       TIMESTAMP,
    resolved_by       VARCHAR(64),
    CONSTRAINT fk_wiki_page_comments_parent
        FOREIGN KEY (parent_id) REFERENCES wiki_page_comments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_kb_slug
    ON wiki_page_comments(knowledge_base_id, page_slug);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_parent
    ON wiki_page_comments(parent_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_tenant
    ON wiki_page_comments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_author
    ON wiki_page_comments(author_id);
