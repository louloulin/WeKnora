-- v0.7.37 — Build #44 Slides (MySQL mirror of 000050_slides).
CREATE TABLE IF NOT EXISTS slide_decks (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    title           VARCHAR(255) NOT NULL,
    theme           VARCHAR(32)  NOT NULL DEFAULT 'notion',
    source_doc_id   VARCHAR(36)  NOT NULL DEFAULT '',
    kb_id           VARCHAR(36)  NOT NULL DEFAULT '',
    owner_user_id   BIGINT       NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    slide_count     INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_slide_decks_tenant (tenant_id),
    INDEX idx_slide_decks_kb (tenant_id, kb_id),
    INDEX idx_slide_decks_owner (tenant_id, owner_user_id),
    INDEX idx_slide_decks_source (source_doc_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS slides (
    id              VARCHAR(36)  PRIMARY KEY,
    deck_id         VARCHAR(36)  NOT NULL,
    idx             INTEGER      NOT NULL DEFAULT 0,
    layout          VARCHAR(32)  NOT NULL DEFAULT 'bullet',
    title           VARCHAR(512) NOT NULL DEFAULT '',
    body            LONGTEXT     NOT NULL,
    bullets         LONGTEXT     NOT NULL,
    left_col        LONGTEXT     NOT NULL,
    right_col       LONGTEXT     NOT NULL,
    image_url       VARCHAR(1024) NOT NULL DEFAULT '',
    quote_text      LONGTEXT     NOT NULL,
    quote_attr      VARCHAR(255) NOT NULL DEFAULT '',
    notes           LONGTEXT     NOT NULL,
    background      VARCHAR(255) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_slides_deck (deck_id, idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
