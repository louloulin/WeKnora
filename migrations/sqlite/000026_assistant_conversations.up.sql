CREATE TABLE IF NOT EXISTS assistant_conversations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    user_id         VARCHAR(36) NOT NULL,
    conversation_id VARCHAR(36) NOT NULL,
    query_text      TEXT NOT NULL,
    kb_citations    TEXT NOT NULL DEFAULT,
    wiki_citations  TEXT NOT NULL DEFAULT,
    source_kb_ids   TEXT NOT NULL DEFAULT,
    include_wiki    BOOLEAN NOT NULL DEFAULT 1,
    result_count    INTEGER NOT NULL DEFAULT 0,
    model_name      VARCHAR(64) NOT NULL DEFAULT '',
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_assistant_conv_tenant_created
    ON assistant_conversations (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_assistant_conv_conversation
    ON assistant_conversations (conversation_id, created_at DESC);
