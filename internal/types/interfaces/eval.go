package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// EvalDatasetService manages persistent eval datasets and their QA
// entries. The legacy DatasetService (interfaces/evaluation.go) is
// kept untouched so the existing /api/v1/evaluation route still serves
// the parquet fixture; this interface backs the new /api/v1/eval/datasets
// endpoints.
type EvalDatasetService interface {
	// CreateDataset persists a new dataset metadata row.
	CreateDataset(ctx context.Context, ds *types.EvalDataset) error
	// GetDatasetByID loads metadata + full QA list for one dataset.
	GetDatasetByID(ctx context.Context, datasetID string) (*types.EvalDataset, []types.EvalDatasetQA, error)
	// ListDatasets returns metadata rows for the tenant, newest first.
	ListDatasets(ctx context.Context, tenantID uint64, limit, offset int) ([]*types.EvalDataset, int, error)
	// UpdateDataset mutates name/description fields. The QA list is
	// replaced via ReplaceQAList, not here.
	UpdateDataset(ctx context.Context, ds *types.EvalDataset) error
	// DeleteDataset cascades to QA + runs (foreign keys handle cleanup).
	DeleteDataset(ctx context.Context, tenantID uint64, datasetID string) error
	// ReplaceQAList swaps the dataset's QA rows in one transaction. Pass
	// an empty slice to leave the dataset with zero questions.
	ReplaceQAList(ctx context.Context, datasetID string, qas []types.EvalDatasetQA) error
	// ImportJSON parses an EvalDatasetJSONPayload and creates the dataset
	// + QA rows in one call. Returns the new dataset id.
	ImportJSON(ctx context.Context, tenantID uint64, createdBy string, payload *EvalDatasetJSONPayload) (string, error)
}

// EvalDatasetJSONPayload is the wire shape accepted by ImportJSON. The
// JSON shape mirrors EvalDataset + []EvalDatasetQA so a single curl POST
// creates both rows in one round-trip. Tags are optional.
type EvalDatasetJSONPayload struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description,omitempty"`
	QA          []EvalDatasetJSONPayloadQA   `json:"qa"`
}

// EvalDatasetJSONPayloadQA is the import shape — qid is optional and
// defaults to the array index when omitted.
type EvalDatasetJSONPayloadQA struct {
	QID             int                       `json:"qid,omitempty"`
	Question        string                    `json:"question"`
	ExpectedAnswer  string                    `json:"expected_answer"`
	ExpectedPassages []types.EvalExpectedPassage `json:"expected_passages"`
	Tags            []string                  `json:"tags,omitempty"`
}

// EvalRunService starts and tracks persistent eval runs. Status is
// pulled from the DB on every Get so a stale in-process cache cannot
// hide a finished run from the UI.
type EvalRunService interface {
	// StartRun persists the run, fires the audit row, and kicks off the
	// background EvalRunner goroutine. Returns the persisted run id.
	StartRun(ctx context.Context, req *EvalRunStartRequest) (string, error)
	// GetRun loads metadata + summary. Caller does not need to pass
	// tenantID; the service enforces tenant scoping internally.
	GetRun(ctx context.Context, tenantID uint64, runID string) (*types.EvalRun, error)
	// ListRuns returns runs newest-first, optionally filtered by dataset.
	ListRuns(ctx context.Context, tenantID uint64, datasetID string, limit, offset int) ([]*types.EvalRun, int, error)
	// ListResults loads every per-QA row for a run.
	ListResults(ctx context.Context, runID string) ([]types.EvalRunResult, error)
	// CancelRun marks a run as canceled if it is still pending/running.
	// Returns ErrRunNotCancelable when the run is already terminal.
	CancelRun(ctx context.Context, tenantID uint64, runID string) error
}

// EvalRunStartRequest is the wire shape for POST /api/v1/eval/runs.
// JudgeModelID is optional — when empty the runner falls back to the
// first ModelTypeKnowledgeQA model (Build #31 D2 default).
type EvalRunStartRequest struct {
	DatasetID         string `json:"dataset_id"`
	ChatModelID       string `json:"chat_model_id"`
	RerankModelID     string `json:"rerank_model_id,omitempty"`
	ReflectionEnabled bool   `json:"reflection_enabled"`
	JudgeModelID      string `json:"judge_model_id,omitempty"`
	CreatedBy         string `json:"-"`
}

// EvalBadcaseService manages the badcase library. Promote is the only
// path that can set severity higher than the auto-flag default; Resolve
// stamps resolved_at so B22 cleanup cron can sweep resolved rows after
// the 90-day window.
type EvalBadcaseService interface {
	// ListBadcases returns rows newest-first, filterable by status /
	// severity / flag_source.
	ListBadcases(ctx context.Context, tenantID uint64, filter EvalBadcaseFilter) ([]*types.EvalBadcase, int, error)
	// FlagAuto inserts an auto-flagged row from the runner.
	FlagAuto(ctx context.Context, tenantID uint64, runID string, qid int, severity types.EvalSeverity, reason string) (*types.EvalBadcase, error)
	// Promote manually raises an existing row (or creates a new one from a
	// previously-passing QA). Emits eval.run_reviewed.
	Promote(ctx context.Context, tenantID uint64, runID string, qid int, severity types.EvalSeverity, notes, promotedBy string) (*types.EvalBadcase, error)
	// Resolve stamps resolved_at + emits eval.run_reviewed.
	Resolve(ctx context.Context, tenantID uint64, badcaseID, notes string) error
}

// EvalBadcaseFilter narrows the badcase list query. Empty fields are
// treated as "no filter". The DB indexes on (tenant_id, status,
// created_at DESC) so a status filter lands on the index; severity-only
// or flag_source-only filters fall back to a tenant_id index scan.
type EvalBadcaseFilter struct {
	Status     string
	Severity   string
	FlagSource string
	Limit      int
	Offset     int
}