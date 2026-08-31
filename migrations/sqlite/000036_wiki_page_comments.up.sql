-- v0.7.25 — Wiki page comments (Build #22, SQLite mirror)
CREATE TABLE IF NOT EXISTS wiki_page_comments (
    id                TEXT PRIMARY KEY,
    tenant_id         INTEGER NOT NULL,
    knowledge_base_id TEXT NOT NULL,
    page_slug         TEXT NOT NULL,
    parent_id         TEXT,
    body              TEXT NOT NULL,
    mentions          TEXT NOT NULL DEFAULT '[]',
    anchor_block_id   TEXT,
    author_id         TEXT NOT NULL,
    author_name       TEXT NOT NULL,
    author_avatar_url TEXT,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at       DATETIME,
    resolved_by       TEXT,
    FOREIGN KEY (parent_id) REFERENCES wiki_page_comments(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_kb_slug ON wiki_page_comments(knowledge_base_id, page_slug);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_parent ON wiki_page_comments(parent_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_tenant ON wiki_page_comments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_author ON wiki_page_comments(author_id);
