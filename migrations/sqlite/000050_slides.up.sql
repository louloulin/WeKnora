-- v0.7.37 — Build #44 Slides: 文档 → 演示 (Docs × KB 一体化).
CREATE TABLE IF NOT EXISTS slide_decks (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       INTEGER      NOT NULL,
    title           VARCHAR(255) NOT NULL,
    theme           VARCHAR(32)  NOT NULL DEFAULT 'notion',
    source_doc_id   VARCHAR(36)  NOT NULL DEFAULT '',
    kb_id           VARCHAR(36)  NOT NULL DEFAULT '',
    owner_user_id   INTEGER      NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    slide_count     INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_slide_decks_tenant ON slide_decks (tenant_id);
CREATE INDEX IF NOT EXISTS idx_slide_decks_kb ON slide_decks (tenant_id, kb_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_slide_decks_owner ON slide_decks (tenant_id, owner_user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_slide_decks_source ON slide_decks (source_doc_id);

CREATE TABLE IF NOT EXISTS slides (
    id              VARCHAR(36)  PRIMARY KEY,
    deck_id         VARCHAR(36)  NOT NULL,
    idx             INTEGER      NOT NULL DEFAULT 0,
    layout          VARCHAR(32)  NOT NULL DEFAULT 'bullet',
    title           VARCHAR(512) NOT NULL DEFAULT '',
    body            TEXT         NOT NULL DEFAULT '',
    bullets         TEXT         NOT NULL DEFAULT '[]',
    left_col        TEXT         NOT NULL DEFAULT '',
    right_col       TEXT         NOT NULL DEFAULT '',
    image_url       VARCHAR(1024) NOT NULL DEFAULT '',
    quote_text      TEXT         NOT NULL DEFAULT '',
    quote_attr      VARCHAR(255) NOT NULL DEFAULT '',
    notes           TEXT         NOT NULL DEFAULT '',
    background      VARCHAR(255) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_slides_deck ON slides (deck_id, idx);
CREATE INDEX IF NOT EXISTS idx_slides_tenant ON slides (deck_id);
