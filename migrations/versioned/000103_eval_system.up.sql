-- B31 — Eval System (Build #31)
-- Adds five tables for the persistent eval system: eval_datasets, eval_dataset_qa,
-- eval_runs, eval_run_results, eval_badcases. The legacy evaluation pipeline
-- (EvaluationService, EvaluationMemoryStorage, ./dataset/samples/*.parquet)
-- is left untouched — it lives behind the existing /api/v1/evaluation route
-- and serves as the seed-import fixture for B31's new EvalRunner.

CREATE TABLE eval_datasets (
    id              VARCHAR(64) PRIMARY KEY,
    tenant_id       BIGINT NOT NULL,
    name            VARCHAR(200) NOT NULL,
    description     TEXT,
    qa_count        INT NOT NULL DEFAULT 0,
    schema_version  INT NOT NULL DEFAULT 1,
    created_by      VARCHAR(64) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_eval_datasets_tenant ON eval_datasets(tenant_id, created_at DESC);

CREATE TABLE eval_dataset_qa (
    dataset_id        VARCHAR(64) NOT NULL REFERENCES eval_datasets(id) ON DELETE CASCADE,
    qid               INT NOT NULL,
    question          TEXT NOT NULL,
    expected_answer   TEXT NOT NULL,
    expected_passages JSONB NOT NULL DEFAULT '[]',
    tags              TEXT[] NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (dataset_id, qid)
);
CREATE INDEX idx_eval_dataset_qa_dataset ON eval_dataset_qa(dataset_id);

CREATE TABLE eval_runs (
    id                    VARCHAR(64) PRIMARY KEY,
    tenant_id             BIGINT NOT NULL,
    dataset_id            VARCHAR(64) NOT NULL,
    chat_model_id         VARCHAR(64) NOT NULL,
    rerank_model_id       VARCHAR(64),
    reflection_enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    judge_model_id        VARCHAR(64) NOT NULL,
    judge_prompt_version  VARCHAR(32) NOT NULL DEFAULT 'v1',
    status                VARCHAR(32) NOT NULL DEFAULT 'pending',
    started_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at           TIMESTAMPTZ,
    canceled_at           TIMESTAMPTZ,
    error                 TEXT,
    summary               JSONB,
    correlation_id        VARCHAR(64),
    git_sha               VARCHAR(40),
    created_by            VARCHAR(64) NOT NULL
);
CREATE INDEX idx_eval_runs_tenant ON eval_runs(tenant_id, started_at DESC);
CREATE INDEX idx_eval_runs_dataset ON eval_runs(dataset_id, started_at DESC);
CREATE INDEX idx_eval_runs_correlation ON eval_runs(correlation_id);

CREATE TABLE eval_run_results (
    run_id                     VARCHAR(64) NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    qid                        INT NOT NULL,
    question                   TEXT NOT NULL,
    model_answer               TEXT NOT NULL,
    expected_answer            TEXT NOT NULL,
    search_top_k               JSONB NOT NULL DEFAULT '[]',
    citation_index             JSONB NOT NULL DEFAULT '[]',
    reflection_events          JSONB NOT NULL DEFAULT '[]',
    factuality_score           FLOAT,
    citation_fidelity_score    FLOAT,
    reflection_necessity_score FLOAT,
    passed                     BOOLEAN NOT NULL,
    badcase_flag_reason        VARCHAR(64),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, qid)
);
CREATE INDEX idx_eval_run_results_run ON eval_run_results(run_id);
CREATE INDEX idx_eval_run_results_failed ON eval_run_results(run_id) WHERE passed = FALSE;

CREATE TABLE eval_badcases (
    id                   VARCHAR(64) PRIMARY KEY,
    tenant_id            BIGINT NOT NULL,
    run_id               VARCHAR(64) NOT NULL,
    qid                  INT NOT NULL,
    flag_source          VARCHAR(32) NOT NULL,
    severity             VARCHAR(16) NOT NULL DEFAULT 'medium',
    status               VARCHAR(32) NOT NULL DEFAULT 'open',
    notes                TEXT,
    jump_chat_message_id VARCHAR(64),
    promoted_by          VARCHAR(64),
    promoted_at          TIMESTAMPTZ,
    resolved_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_eval_badcases_tenant ON eval_badcases(tenant_id, status, created_at DESC);
CREATE INDEX idx_eval_badcases_run ON eval_badcases(run_id);