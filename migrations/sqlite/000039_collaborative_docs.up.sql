-- SQLite mirror of 000132_collaborative_docs.
CREATE TABLE IF NOT EXISTS collaborative_docs (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    title           VARCHAR(256) NOT NULL,
    doc_kind        VARCHAR(16)  NOT NULL DEFAULT 'doc',
    schema_version  INTEGER      NOT NULL DEFAULT 1,
    owner_user_id   INTEGER      NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    share_token     VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_collaborative_docs_tenant ON collaborative_docs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_collaborative_docs_kb ON collaborative_docs (kb_id);

CREATE TABLE IF NOT EXISTS collab_doc_snapshots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER     NOT NULL,
    doc_id          VARCHAR(36) NOT NULL,
    doc_kind        VARCHAR(16) NOT NULL DEFAULT 'doc',
    schema_version  INTEGER     NOT NULL DEFAULT 1,
    ydoc_state      BLOB        NOT NULL,
    vector_clock    BLOB        NOT NULL DEFAULT (X''),
    version         INTEGER     NOT NULL DEFAULT 1,
    size_bytes      INTEGER     NOT NULL,
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, doc_id)
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_snapshots_tenant ON collab_doc_snapshots (tenant_id);
CREATE INDEX IF NOT EXISTS idx_collab_doc_snapshots_updated ON collab_doc_snapshots (updated_at DESC);

CREATE TABLE IF NOT EXISTS collab_doc_sessions (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    user_id         INTEGER      NOT NULL,
    client_id       INTEGER      NOT NULL,
    color           VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    last_heartbeat  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    joined_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, doc_id, client_id)
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_doc ON collab_doc_sessions (doc_id, last_heartbeat DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_user ON collab_doc_sessions (user_id, last_heartbeat DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_heartbeat ON collab_doc_sessions (last_heartbeat);
