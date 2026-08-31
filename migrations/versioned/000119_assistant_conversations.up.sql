-- AI Assistant conversation history (Build v0.7.15). Each row is one
-- Q&A turn: the user's natural-language question, the persisted
-- citations (KB + Wiki), and the source timestamps. The LLM answer
-- itself is intentionally NOT stored — answers are cheap to regenerate
-- from the same citations, and storing them invites the answer to
-- drift from the cited evidence over time. Audit / retention rules
-- apply via the existing audit_log + tenant policies.
--
-- The persisted_search_payload column holds the post-fusion result
-- (KB + Wiki citations + ranks + scores) so a future "rewind this
-- conversation" or "what did the assistant see?" UI can replay the
-- retrieval exactly as it happened.
--
-- The composite index on (tenant_id, created_at DESC) supports the
-- hot read path:
--   SELECT ... FROM assistant_conversations
--   WHERE tenant_id = ? ORDER BY created_at DESC LIMIT 20
CREATE TABLE IF NOT EXISTS assistant_conversations (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    user_id         VARCHAR(36) NOT NULL,
    conversation_id VARCHAR(36) NOT NULL,
    query_text      TEXT NOT NULL,
    kb_citations    JSONB NOT NULL DEFAULT '[]'::jsonb,
    wiki_citations  JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_kb_ids   JSONB NOT NULL DEFAULT '[]'::jsonb,
    include_wiki    BOOLEAN NOT NULL DEFAULT TRUE,
    result_count    INTEGER NOT NULL DEFAULT 0,
    model_name       VARCHAR(64) NOT NULL DEFAULT '',
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_assistant_conv_tenant_created
    ON assistant_conversations (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_assistant_conv_conversation
    ON assistant_conversations (conversation_id, created_at DESC)
    WHERE deleted_at IS NULL;
