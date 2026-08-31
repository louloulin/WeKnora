-- v0.7.30 — collab_doc_audit_log: immutable operation history for collaborative docs.
CREATE TABLE IF NOT EXISTS collab_doc_audit_log (
    id              BIGINT       PRIMARY KEY AUTO_INCREMENT,
    tenant_id       BIGINT       NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    actor_user_id   BIGINT       NOT NULL DEFAULT 0,
    actor_name      VARCHAR(128) NOT NULL DEFAULT '',
    actor_color     VARCHAR(16)  NOT NULL DEFAULT '',
    action          VARCHAR(32)  NOT NULL,
    target          VARCHAR(64)  NOT NULL DEFAULT '',
    payload         TEXT         NOT NULL,
    ip              VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(256) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_collab_doc_audit_doc (doc_id, created_at),
    INDEX idx_collab_doc_audit_tenant (tenant_id, created_at),
    INDEX idx_collab_doc_audit_action (tenant_id, action, created_at),
    INDEX idx_collab_doc_audit_actor (tenant_id, actor_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
