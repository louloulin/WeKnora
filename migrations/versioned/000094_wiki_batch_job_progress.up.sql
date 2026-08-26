-- 000094_wiki_batch_job_progress.up.sql
--
-- Build #15 — wiki batch observability.
--
-- Adds a JSONB `progress` column to `wiki_batch_jobs` so the worker can
-- publish running counters (total / processed / succeeded / failed) on
-- a throttled cadence (every 5 slugs). The polling endpoint surfaces
-- this field to the toast so the user sees e.g. "23/100 已处理".
--
-- `wiki_batch_job_failures` is the per-slug failure ledger. The audit
-- log (Build #14) records "what happened when" — this table records
-- "which slug failed because of what", so the UI can group errors by
-- code and the operator can grep by slug without parsing the result
-- JSONB blob.

ALTER TABLE wiki_batch_jobs
    ADD COLUMN IF NOT EXISTS progress JSONB;

-- progress JSONB shape:
--   {
--     "total":      100,
--     "processed":  23,
--     "succeeded":  21,
--     "failed":     2,
--     "updated_at": "2026-08-26T05:00:00Z"
--   }

CREATE TABLE IF NOT EXISTS wiki_batch_job_failures (
    id                BIGSERIAL PRIMARY KEY,
    tenant_id         BIGINT    NOT NULL,
    knowledge_base_id UUID      NOT NULL,
    batch_job_id      UUID      NOT NULL,
    slug              TEXT      NOT NULL,
    code              TEXT      NOT NULL,  -- not_found / folder_not_found / ...
    error             TEXT      NOT NULL,
    occurred_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Single-job drill-down (the failure drawer's primary query path).
CREATE INDEX IF NOT EXISTS idx_wbbf_job
    ON wiki_batch_job_failures (batch_job_id);

-- KB-wide code aggregation ("how many folder_conflict this week?").
CREATE INDEX IF NOT EXISTS idx_wbbf_kb_code
    ON wiki_batch_job_failures (knowledge_base_id, code);