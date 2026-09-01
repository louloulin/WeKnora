-- v0.7.90 — collab_doc_form_responses (MySQL): see sqlite/000055 for rationale.
CREATE TABLE IF NOT EXISTS collab_doc_form_responses (
    id                BIGINT       AUTO_INCREMENT PRIMARY KEY,
    tenant_id         BIGINT       NOT NULL,
    doc_id            VARCHAR(36)  NOT NULL,
    submitter_token   VARCHAR(64)  NOT NULL,
    submitter_name    VARCHAR(128) NOT NULL DEFAULT '',
    submitter_user_id BIGINT       NOT NULL DEFAULT 0,
    answers           TEXT         NOT NULL,
    client_ip         VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent        VARCHAR(256) NOT NULL DEFAULT '',
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_collab_doc_form_responses_doc (doc_id, created_at),
    INDEX idx_collab_doc_form_responses_token (submitter_token),
    INDEX idx_collab_doc_form_responses_tenant (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
