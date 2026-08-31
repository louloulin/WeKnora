-- v0.7.36 — Build #43 MindMaps (MySQL mirror of 000048_mindmaps).
CREATE TABLE IF NOT EXISTS mindmaps (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    title           VARCHAR(255) NOT NULL,
    layout          VARCHAR(16)  NOT NULL DEFAULT 'tree',
    theme           VARCHAR(32)  NOT NULL DEFAULT 'feishu',
    root_node_id    VARCHAR(36)  NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL DEFAULT '',
    owner_user_id   BIGINT       NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_mindmaps_tenant (tenant_id),
    INDEX idx_mindmaps_kb (tenant_id, kb_id),
    INDEX idx_mindmaps_owner (tenant_id, owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mindmap_nodes (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    map_id          VARCHAR(36)  NOT NULL,
    parent_id       VARCHAR(36),
    node_type       VARCHAR(16)  NOT NULL DEFAULT 'text',
    label           VARCHAR(512) NOT NULL,
    body            LONGTEXT     NOT NULL,
    x               DOUBLE       NOT NULL DEFAULT 0,
    y               DOUBLE       NOT NULL DEFAULT 0,
    width           DOUBLE       NOT NULL DEFAULT 160,
    height          DOUBLE       NOT NULL DEFAULT 48,
    color           VARCHAR(16)  NOT NULL DEFAULT '',
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    doc_ref         VARCHAR(36),
    kb_ref          VARCHAR(36),
    task_ref        BIGINT,
    formula         LONGTEXT     NOT NULL,
    order_hint      INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_mindmap_nodes_map (tenant_id, map_id),
    INDEX idx_mindmap_nodes_parent (tenant_id, map_id, parent_id),
    INDEX idx_mindmap_nodes_docref (tenant_id, doc_ref),
    INDEX idx_mindmap_nodes_kbref (tenant_id, kb_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
