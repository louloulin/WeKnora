-- v0.7.35 — collab_doc_comments: cross-doc comment threads for DOC / PPT / SHEET.
CREATE TABLE IF NOT EXISTS collab_doc_comments (
    id              BIGINT       PRIMARY KEY AUTO_INCREMENT,
    tenant_id       BIGINT       NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    thread_id       VARCHAR(36)  NOT NULL,
    parent_id       BIGINT       NULL,
    author_user_id  BIGINT       NOT NULL,
    author_name     VARCHAR(128) NOT NULL DEFAULT '',
    author_color    VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    anchor_type     VARCHAR(16)  NOT NULL,
    anchor_ref      TEXT         NOT NULL,
    body            TEXT         NOT NULL,
    resolved        TINYINT(1)   NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_collab_doc_comments_doc (doc_id, created_at),
    INDEX idx_collab_doc_comments_thread (thread_id, created_at),
    INDEX idx_collab_doc_comments_tenant (tenant_id),
    CONSTRAINT fk_collab_doc_comments_parent FOREIGN KEY (parent_id) REFERENCES collab_doc_comments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
