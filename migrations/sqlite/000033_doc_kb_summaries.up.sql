-- SQLite variant of the v0.7.23 Doc ↔ KB AI Summary Bridge table.
CREATE TABLE IF NOT EXISTS doc_kb_summaries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    knowledge_id    VARCHAR(36) NOT NULL,
    chunk_id        VARCHAR(36) NOT NULL,
    summary         TEXT NOT NULL,
    keyphrases      TEXT NOT NULL DEFAULT '[]',
    auto_tags       TEXT NOT NULL DEFAULT '[]',
    model_name      VARCHAR(64) NOT NULL DEFAULT '',
    confidence      REAL NOT NULL DEFAULT 0.0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_doc_kb_summaries_chunk
    ON doc_kb_summaries (tenant_id, knowledge_id, chunk_id);
CREATE INDEX IF NOT EXISTS idx_doc_kb_summaries_kb
    ON doc_kb_summaries (tenant_id, knowledge_id);
