-- SQLite mirror of migrations/versioned 000110 + 000111.
-- WeKnora dev profile uses SQLite; production uses Postgres.
CREATE TABLE IF NOT EXISTS wiki_doc_snapshots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER     NOT NULL,
    kb_id           VARCHAR(36) NOT NULL,
    page_id         VARCHAR(36) NOT NULL,
    ydoc_state      BLOB        NOT NULL,
    vector_clock    BLOB        NOT NULL DEFAULT (X''),
    version         INTEGER     NOT NULL DEFAULT 1,
    size_bytes      INTEGER     NOT NULL,
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, kb_id, page_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_doc_snapshots_tenant ON wiki_doc_snapshots (tenant_id);

CREATE TABLE IF NOT EXISTS wiki_realtime_sessions (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    page_id         VARCHAR(36)  NOT NULL,
    user_id         INTEGER      NOT NULL,
    client_id       INTEGER      NOT NULL,
    color           VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    last_heartbeat  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    joined_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, page_id, client_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_realtime_sessions_page ON wiki_realtime_sessions (page_id, last_heartbeat DESC);
