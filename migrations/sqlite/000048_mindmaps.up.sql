-- v0.7.36 — Build #43 MindMaps: docs × KB 一体化 · 富表达层 (思维导图)
--
-- Two tables: mindmaps (the aggregate root) + mindmap_nodes (children).
-- Composite indexes cover the hot read paths:
--   - (tenant_id, kb_id) for "list maps for a KB"
--   - (tenant_id, map_id) for "list nodes for a map"
--   - (tenant_id, map_id, parent_id) for layout walks
CREATE TABLE IF NOT EXISTS mindmaps (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    title           VARCHAR(255) NOT NULL,
    layout          VARCHAR(16)  NOT NULL DEFAULT 'tree',
    theme           VARCHAR(32)  NOT NULL DEFAULT 'feishu',
    root_node_id    VARCHAR(36)  NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL DEFAULT '',
    owner_user_id   INTEGER      NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mindmaps_tenant ON mindmaps (tenant_id);
CREATE INDEX IF NOT EXISTS idx_mindmaps_kb ON mindmaps (tenant_id, kb_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_mindmaps_owner ON mindmaps (tenant_id, owner_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS mindmap_nodes (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    map_id          VARCHAR(36)  NOT NULL,
    parent_id       VARCHAR(36),
    node_type       VARCHAR(16)  NOT NULL DEFAULT 'text',
    label           VARCHAR(512) NOT NULL,
    body            TEXT         NOT NULL DEFAULT '',
    x               REAL         NOT NULL DEFAULT 0,
    y               REAL         NOT NULL DEFAULT 0,
    width           REAL         NOT NULL DEFAULT 160,
    height          REAL         NOT NULL DEFAULT 48,
    color           VARCHAR(16)  NOT NULL DEFAULT '',
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    doc_ref         VARCHAR(36),
    kb_ref          VARCHAR(36),
    task_ref        INTEGER,
    formula         TEXT         NOT NULL DEFAULT '',
    order_hint      INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mindmap_nodes_map ON mindmap_nodes (tenant_id, map_id);
CREATE INDEX IF NOT EXISTS idx_mindmap_nodes_parent ON mindmap_nodes (tenant_id, map_id, parent_id, order_hint);
CREATE INDEX IF NOT EXISTS idx_mindmap_nodes_docref ON mindmap_nodes (tenant_id, doc_ref);
CREATE INDEX IF NOT EXISTS idx_mindmap_nodes_kbref ON mindmap_nodes (tenant_id, kb_ref);
