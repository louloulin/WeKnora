package types

import (
	"encoding/json"
	"time"
)

// EvalBadcaseFlagSource separates automatically flagged badcases from
// manually promoted ones. The UI uses this to label the badge so reviewers
// know whether the row landed via threshold (auto) or a person (human).
type EvalBadcaseFlagSource string

const (
	EvalBadcaseFlagSourceAuto        EvalBadcaseFlagSource = "auto"
	EvalBadcaseFlagSourceHumanPromote EvalBadcaseFlagSource = "human_promote"
)

// EvalSeverity buckets badcases by reviewer priority. The mapping to
// numeric SLAs lives in the frontend; the backend keeps the enum
// closed so audit_logs.Details["severity"] stays consistent across writes.
type EvalSeverity string

const (
	EvalSeverityLow      EvalSeverity = "low"
	EvalSeverityMedium   EvalSeverity = "medium"
	EvalSeverityHigh     EvalSeverity = "high"
	EvalSeverityCritical EvalSeverity = "critical"
)

// EvalBadcaseStatus follows the standard triage flow: open → triaged →
// resolved | wontfix. resolved_at is stamped on every transition out of
// open so B22 cleanup cron can archive after 90 days (Build #31 D4).
type EvalBadcaseStatus string

const (
	EvalBadcaseStatusOpen     EvalBadcaseStatus = "open"
	EvalBadcaseStatusTriaged  EvalBadcaseStatus = "triaged"
	EvalBadcaseStatusResolved EvalBadcaseStatus = "resolved"
	EvalBadcaseStatusWontfix  EvalBadcaseStatus = "wontfix"
)

// EvalRunStatus mirrors the existing EvaluationStatue enum but lives in
// its own namespace so the legacy /api/v1/evaluation pipeline and the
// new EvalRunner can evolve independently. pending → running →
// succeeded | failed | canceled.
type EvalRunStatus string

const (
	EvalRunStatusPending   EvalRunStatus = "pending"
	EvalRunStatusRunning   EvalRunStatus = "running"
	EvalRunStatusSucceeded EvalRunStatus = "succeeded"
	EvalRunStatusFailed    EvalRunStatus = "failed"
	EvalRunStatusCanceled  EvalRunStatus = "canceled"
)

// EvalExpectedPassage is one ground-truth passage attached to a QA
// entry. pid is the integer passage id from the source parquet (or
// the JSON upload), text is the raw passage body. Matches the shape
// eval_run_results.search_top_k produces so reflection_necessity and
// citation_fidelity can index directly into both.
type EvalExpectedPassage struct {
	PID  int    `json:"pid"`
	Text string `json:"text"`
}

// EvalDataset is one tenant-owned eval collection. schema_version lets
// future EvalDatasetQA shape changes land without forcing a re-import;
// today only v1 is supported.
type EvalDataset struct {
	ID            string    `json:"id"`
	TenantID      uint64    `json:"tenant_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	QACount       int       `json:"qa_count"`
	SchemaVersion int       `json:"schema_version"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EvalDatasetQA is one question inside a dataset. expected_passages is
// always a JSON array of {pid, text}; tags is an optional list used by
// eval_runner to slice sub-cohorts (math, "citation-heavy",
// "reflection-required" etc.).
type EvalDatasetQA struct {
	DatasetID        string              `json:"dataset_id"`
	QID              int                 `json:"qid"`
	Question         string              `json:"question"`
	ExpectedAnswer   string              `json:"expected_answer"`
	ExpectedPassages []EvalExpectedPassage `json:"expected_passages"`
	Tags             []string            `json:"tags,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
}

// EvalRunSummary is the rolled-up scorecard persisted in eval_runs.summary
// (jsonb) and returned by GET /api/v1/eval/runs/:id. Fields are nullable
// so a partial run (canceled mid-flight) still produces a coherent
// summary instead of zero-fills.
type EvalRunSummary struct {
	Total                   int     `json:"total"`
	Passed                  int     `json:"passed"`
	Failed                  int     `json:"failed"`
	FactualityAvg           float64 `json:"factuality_avg"`
	CitationFidelityAvg     float64 `json:"citation_fidelity_avg"`
	ReflectionNecessityAvg  float64 `json:"reflection_necessity_avg"`
	ToolCacheHitRatio       float64 `json:"tool_cache_hit_ratio"`
	AutoBadcaseCount        int     `json:"auto_badcase_count"`
}

// EvalRun is one eval execution. summary is filled only on terminal
// status (succeeded / failed / canceled); the in-flight view is empty.
// git_sha is captured at run time so cross-PR comparisons remain valid
// even after the chat pipeline changes underneath (Build #31 risk #3).
type EvalRun struct {
	ID                 string          `json:"id"`
	TenantID           uint64          `json:"tenant_id"`
	DatasetID          string          `json:"dataset_id"`
	ChatModelID        string          `json:"chat_model_id"`
	RerankModelID      string          `json:"rerank_model_id,omitempty"`
	ReflectionEnabled  bool            `json:"reflection_enabled"`
	JudgeModelID       string          `json:"judge_model_id"`
	JudgePromptVersion string          `json:"judge_prompt_version"`
	Status             EvalRunStatus   `json:"status"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
	CanceledAt         *time.Time      `json:"canceled_at,omitempty"`
	Error              string          `json:"error,omitempty"`
	Summary            *EvalRunSummary `json:"summary,omitempty"`
	CorrelationID       string          `json:"correlation_id,omitempty"`
	GitSHA             string          `json:"git_sha,omitempty"`
	CreatedBy          string          `json:"created_by"`
}

// EvalRunResult is one QA row inside an eval run. The JSON columns
// (search_top_k / citation_index / reflection_events) mirror the
// corresponding fields in chat_manage so the frontend can render the
// per-question evidence panel without re-running anything.
type EvalRunResult struct {
	RunID                     string          `json:"run_id"`
	QID                       int                 `json:"qid"`
	Question                  string          `json:"question"`
	ModelAnswer               string          `json:"model_answer"`
	ExpectedAnswer            string          `json:"expected_answer"`
	SearchTopK                json.RawMessage `json:"search_top_k"`
	CitationIndex             json.RawMessage `json:"citation_index"`
	ReflectionEvents          json.RawMessage `json:"reflection_events"`
	FactualityScore           *float64        `json:"factuality_score,omitempty"`
	CitationFidelityScore     *float64        `json:"citation_fidelity_score,omitempty"`
	ReflectionNecessityScore  *float64        `json:"reflection_necessity_score,omitempty"`
	Passed                    bool            `json:"passed"`
	BadcaseFlagReason         string          `json:"badcase_flag_reason,omitempty"`
	CreatedAt                 time.Time       `json:"created_at"`
}

// EvalBadcase is one row in the badcase library. jump_chat_message_id
// is optional because the source may be a one-off synthetic QA with
// no real chat turn — when present it points at the chat message that
// surfaced the failure (Build #30 B4 source_message_id lineage).
type EvalBadcase struct {
	ID                string              `json:"id"`
	TenantID          uint64              `json:"tenant_id"`
	RunID             string              `json:"run_id"`
	QID               int                 `json:"qid"`
	FlagSource        EvalBadcaseFlagSource `json:"flag_source"`
	Severity          EvalSeverity        `json:"severity"`
	Status            EvalBadcaseStatus   `json:"status"`
	Notes             string              `json:"notes,omitempty"`
	JumpChatMessageID string              `json:"jump_chat_message_id,omitempty"`
	PromotedBy        string              `json:"promoted_by,omitempty"`
	PromotedAt        *time.Time          `json:"promoted_at,omitempty"`
	ResolvedAt        *time.Time          `json:"resolved_at,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
}