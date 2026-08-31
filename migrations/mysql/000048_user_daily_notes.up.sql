-- v0.7.38 Build #45.a — Daily Note 默认页 (MySQL flavor).
--
-- MySQL DATE columns accept the same YYYY-MM-DD ISO literal format
-- so the schema is byte-identical to the sqlite variant apart from
-- the trailing-identifier / engine clause.
CREATE TABLE user_daily_notes (
    id                  VARCHAR(36)  NOT NULL,
    tenant_id           BIGINT       NOT NULL,
    user_id             VARCHAR(64)  NOT NULL,
    knowledge_base_id   VARCHAR(36)  NOT NULL,
    note_date           DATE         NOT NULL,
    slug                VARCHAR(255) NOT NULL DEFAULT '',
    page_id             VARCHAR(36)  NOT NULL DEFAULT '',
    title               VARCHAR(255) NOT NULL DEFAULT '',
    content             TEXT         NOT NULL,
    summary             VARCHAR(512) NOT NULL DEFAULT '',
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_user_daily_notes_user_kb_date (user_id, knowledge_base_id, note_date),
    KEY idx_user_daily_notes_user_date (user_id, note_date),
    KEY idx_user_daily_notes_kb_date   (knowledge_base_id, note_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
