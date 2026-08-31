-- v0.7.30 — collab_doc_audit_log: immutable operation history for collaborative docs.
--
-- Writes happen on every meaningful user action (upload, save, share, archive,
-- delete, comment add/resolve/delete, polish, sync-to-KB). Reads power the
-- doc-detail "history" panel and tenant-level audit queries. `action` is the
-- closed enum of operation types; `payload` carries JSON detail (free-form
-- but validated at the application layer).
--
-- The table never updates rows — every action is a new row, which keeps the
-- history tamper-evident.
CREATE TABLE IF NOT EXISTS collab_doc_audit_log (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    actor_user_id   INTEGER      NOT NULL DEFAULT 0,
    actor_name      VARCHAR(128) NOT NULL DEFAULT '',
    actor_color     VARCHAR(16)  NOT NULL DEFAULT '',
    action          VARCHAR(32)  NOT NULL,
    target          VARCHAR(64)  NOT NULL DEFAULT '',
    payload         TEXT         NOT NULL DEFAULT '',
    ip              VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(256) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_audit_doc ON collab_doc_audit_log (doc_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_audit_tenant ON collab_doc_audit_log (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_audit_action ON collab_doc_audit_log (tenant_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_audit_actor ON collab_doc_audit_log (tenant_id, actor_user_id, created_at DESC);
