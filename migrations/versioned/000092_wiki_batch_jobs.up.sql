-- Migration 000092: wiki_batch_jobs
--
-- Build #13 — async batch jobs. Stores background batch operations queued
-- by POST /wiki/pages/batch-{move,delete,status} when the request contains
-- >= WikiBatchAsyncThreshold slugs (currently 20). Workers (in-process
-- goroutine pool, see cmd/server/main.go) consume queued jobs, run them
-- through the existing partial-success Batch* service methods, and write
-- the WikiBatchResult back to the `result` JSONB column.
--
-- Fields:
--   * id UUID — exposed to the client for poll/undo.
--   * knowledge_base_id — every job is KB-scoped; cross-KB reads return 404.
--   * type — discriminator for undo semantics (see UndoJob in service):
--       move    → undo restores folder_id from undo_state
--       delete  → undo restores deleted_at = NULL + slug gets __restored_<id> suffix
--       status  → not undoable (UI hides the button)
--   * params — type-specific input (slugs + folder_id / target status).
--   * undo_state — per-page original state captured before mutation, so
--     undo can be deterministic without re-fetching prior values.
--   * state machine: queued → running → (succeeded | failed | partial).
--   * expires_at = finished_at + 7 days. After this, UndoJob returns 410 Gone.
--     Frontend also hides the undo button after 60s (UX window) — the two
--     windows are independent.

CREATE TABLE IF NOT EXISTS wiki_batch_jobs (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('move', 'delete', 'status', 'tag')),
    params JSONB NOT NULL,
    undo_state JSONB,
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'partial')),
    result JSONB,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_wiki_batch_jobs_kb_state
    ON wiki_batch_jobs (knowledge_base_id, state);

CREATE INDEX IF NOT EXISTS idx_wiki_batch_jobs_expires
    ON wiki_batch_jobs (expires_at)
    WHERE expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wiki_batch_jobs_kb_created
    ON wiki_batch_jobs (knowledge_base_id, created_at DESC);