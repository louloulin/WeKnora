-- SQLite mirror of migrations/versioned 000112 + 000113.
-- WeKnora dev profile uses SQLite; production uses Postgres.
CREATE TABLE IF NOT EXISTS wiki_sync_blocks (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    block_id        VARCHAR(36)  NOT NULL,
    title           VARCHAR(256) NOT NULL DEFAULT '',
    content_json    TEXT         NOT NULL DEFAULT '{}',
    content_md      TEXT         NOT NULL DEFAULT '',
    version         INTEGER      NOT NULL DEFAULT 1,
    owner_id        INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, block_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_sync_blocks_tenant ON wiki_sync_blocks (tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_sync_blocks_kb ON wiki_sync_blocks (tenant_id, kb_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS wiki_sync_block_refs (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    block_id        VARCHAR(36)  NOT NULL,
    page_id         VARCHAR(36)  NOT NULL,
    anchor_slug     VARCHAR(256) NOT NULL DEFAULT '',
    content_version INTEGER      NOT NULL DEFAULT 0,
    rendered_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, block_id, page_id, anchor_slug)
);
CREATE INDEX IF NOT EXISTS idx_wiki_sync_block_refs_block ON wiki_sync_block_refs (tenant_id, block_id);
CREATE INDEX IF NOT EXISTS idx_wiki_sync_block_refs_page ON wiki_sync_block_refs (tenant_id, page_id);
