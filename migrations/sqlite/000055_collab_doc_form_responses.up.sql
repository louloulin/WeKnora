-- v0.7.90 — collab_doc_form_responses: Tencent Docs 收集表 response collection.
--
-- Background: forms (CollaborativeDocKindForm) only stored the question
-- schema in the Yjs doc. Respondents had no way to submit answers and
-- owners had no aggregate view. This adds a flat responses table:
--   * one row per submission
--   * `answers` carries the per-question payload as JSON so the schema
--     can evolve (text/choice/multi/rating/date) without migration churn
--   * `submitter_token` is an anonymous UUID the public responder page
--     pins in localStorage; lets the owner de-dupe and lets the public
--     page show "you already answered" without auth.
--
-- The public submit endpoint takes either the doc's share_token (anon
-- respondents) OR a logged-in user (an account-bound row). Owner-only
-- list/export endpoints use the existing auth middleware.
CREATE TABLE IF NOT EXISTS collab_doc_form_responses (
    id               INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id        INTEGER      NOT NULL,
    doc_id           VARCHAR(36)  NOT NULL,
    submitter_token  VARCHAR(64)  NOT NULL,
    submitter_name   VARCHAR(128) NOT NULL DEFAULT '',
    submitter_user_id INTEGER     NOT NULL DEFAULT 0,
    answers          TEXT         NOT NULL,
    client_ip        VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent       VARCHAR(256) NOT NULL DEFAULT '',
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_form_responses_doc
    ON collab_doc_form_responses (doc_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_form_responses_token
    ON collab_doc_form_responses (submitter_token);
CREATE INDEX IF NOT EXISTS idx_collab_doc_form_responses_tenant
    ON collab_doc_form_responses (tenant_id);
