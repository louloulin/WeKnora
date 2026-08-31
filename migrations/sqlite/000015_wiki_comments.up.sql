CREATE TABLE IF NOT EXISTS wiki_page_comments (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    wiki_page_id VARCHAR(36) NOT NULL,
    parent_comment_id VARCHAR(36),
    author_id VARCHAR(36) NOT NULL,
    body TEXT NOT NULL,
    mentions TEXT NOT NULL DEFAULT '[]',
    resolved_at DATETIME,
    resolved_by VARCHAR(36),
    deleted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (wiki_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE,
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_page ON wiki_page_comments(wiki_page_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_author ON wiki_page_comments(author_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_tenant ON wiki_page_comments(tenant_id);
