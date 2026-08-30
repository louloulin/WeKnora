-- Wiki page comments: Notion / Feishu / Confluence collaboration primitive.
-- Threads are flat (no nesting) to keep the schema simple; rendering layer
-- can synthesize nested views from parent_comment_id. Resolved/deleted
-- comments are soft-deleted so audit trail stays intact.
CREATE TABLE IF NOT EXISTS wiki_page_comments (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    wiki_page_id VARCHAR(36) NOT NULL,
    parent_comment_id VARCHAR(36),
    author_id VARCHAR(36) NOT NULL,
    body TEXT NOT NULL,
    mentions JSONB NOT NULL DEFAULT '[]',
    resolved_at TIMESTAMP WITH TIME ZONE,
    resolved_by VARCHAR(36),
    deleted_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_wiki_page_comments_page FOREIGN KEY (wiki_page_id) REFERENCES wiki_pages(id) ON DELETE CASCADE,
    CONSTRAINT fk_wiki_page_comments_author FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_page ON wiki_page_comments(wiki_page_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_author ON wiki_page_comments(author_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_resolved ON wiki_page_comments(resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_wiki_page_comments_tenant ON wiki_page_comments(tenant_id);
