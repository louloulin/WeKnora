-- v0.7.26 — MySQL mirror of 000040_collab_doc_files.
CREATE TABLE IF NOT EXISTS collab_doc_files (
    id              BIGINT       AUTO_INCREMENT PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    format          VARCHAR(16)  NOT NULL,
    content         LONGBLOB     NOT NULL,
    size_bytes      INT          NOT NULL,
    sha256          VARCHAR(64)  NOT NULL DEFAULT '',
    version         INT          NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_collab_doc_files_doc_version (doc_id, version),
    KEY idx_collab_doc_files_doc (doc_id, version DESC),
    KEY idx_collab_doc_files_tenant (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
