-- Doc ↔ KB AI Summary Bridge (Build v0.7.23). For each knowledge entry
-- the system can produce an AI-generated summary + keyphrase + tag list
-- that is independently searchable. This bridges the doc-side
-- lifecycle (upload → parse → chunk) with the AI-side knowledge layer
-- so the AI Assistant can answer "what is in this document?" without
-- re-running the full retrieval over every raw chunk.
--
-- One row per (knowledge_id, chunk_id) so we can attribute the
-- summary to the exact source span — vital for citation provenance.
-- The (knowledge_id, chunk_id) pair is unique so a re-run of the
-- summariser is idempotent and overwrites the prior summary.
CREATE TABLE IF NOT EXISTS doc_kb_summaries (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    knowledge_id    VARCHAR(36) NOT NULL,
    chunk_id        VARCHAR(36) NOT NULL,
    summary         TEXT NOT NULL,
    keyphrases      JSONB NOT NULL DEFAULT '[]'::jsonb,
    auto_tags       JSONB NOT NULL DEFAULT '[]'::jsonb,
    model_name      VARCHAR(64) NOT NULL DEFAULT '',
    confidence      REAL NOT NULL DEFAULT 0.0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_doc_kb_summaries_chunk
    ON doc_kb_summaries (tenant_id, knowledge_id, chunk_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_doc_kb_summaries_kb
    ON doc_kb_summaries (tenant_id, knowledge_id)
    WHERE deleted_at IS NULL;
