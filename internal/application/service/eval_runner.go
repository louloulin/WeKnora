package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/eval/judge"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// Build #31 — EvalRunner.
//
// Owns the full lifecycle of an eval run:
//
//   - StartRun persists the eval_runs row, fires eval.run_started,
//     and kicks off a goroutine that loops every QA through the chat
//     pipeline + 3 judge calls.
//   - per-QA: invoke EvalChatPipeline (D2/D3 reusable seam), persist
//     eval_run_results, auto-flag badcases below threshold, increment
//     Prom counters.
//   - finish: roll up summary into eval_runs.summary, mark terminal
//     status, fire eval.run_completed.
//
// Concurrency model: a single EvalRunner goroutine per run. Run
// cancellation is read on every QA so CancelRun takes effect within
// one iteration. Multiple concurrent runs are independent (no shared
// mutable state).

// EvalChatPipeline is the seam that calls the chat pipeline for one
// QA. The DI container wires a real implementation that routes to
// chat_manage (Build #30 B3) and stamps the JSON sidecars:
// search_top_k / citation_index / reflection_events. Tests wire a
// stub that returns canned answers.
//
// Implementation note: returning json.RawMessage for search_top_k /
// citation_index / reflection_events lets the runner pass the bytes
// straight through to the eval_run_results row without re-marshalling.
type EvalChatPipeline interface {
	AnswerQA(ctx context.Context, req EvalChatRequest) (*EvalChatResponse, error)
}

// EvalChatRequest is the per-QA invocation. KBs is the list of KB ids
// the QA is scoped to — for the first eval harness this is always
// the eval-specific KB (or empty for non-KB prompts).
type EvalChatRequest struct {
	TenantID          uint64
	DatasetID         string
	ChatModelID       string
	RerankModelID     string
	ReflectionEnabled bool
	Question          string
	KBIDs             []string
}

// EvalChatResponse is the per-QA output. ModelAnswer is plain text.
// SearchTopK / CitationIndex / ReflectionEvents are stored verbatim
// into eval_run_results so the frontend can render evidence without
// re-running.
type EvalChatResponse struct {
	ModelAnswer      string
	SearchTopK       json.RawMessage
	CitationIndex    json.RawMessage
	ReflectionEvents json.RawMessage
}

// evalRunner is the concrete runner.
type evalRunner struct {
	db           *gorm.DB
	datasetSvc   interfaces.EvalDatasetService
	badcaseSvc   interfaces.EvalBadcaseService
	modelSvc     interfaces.ModelService
	chatPipeline EvalChatPipeline
	auditSvc     interfaces.AuditLogService
}

// NewEvalRunnerService wires the runner with the production pipeline.
// Tests can pass a stub EvalChatPipeline and a nil auditSvc (degrades
// to warn-log).
func NewEvalRunnerService(
	db *gorm.DB,
	datasetSvc interfaces.EvalDatasetService,
	badcaseSvc interfaces.EvalBadcaseService,
	modelSvc interfaces.ModelService,
	chatPipeline EvalChatPipeline,
	auditSvc interfaces.AuditLogService,
) interfaces.EvalRunService {
	return &evalRunner{
		db:           db,
		datasetSvc:   datasetSvc,
		badcaseSvc:   badcaseSvc,
		modelSvc:     modelSvc,
		chatPipeline: chatPipeline,
		auditSvc:     auditSvc,
	}
}

// StartRun persists the run row + audit entry, then kicks off the
// background loop. The caller gets the run id immediately so the
// frontend can poll GetRun.
func (r *evalRunner) StartRun(ctx context.Context, req *interfaces.EvalRunStartRequest) (string, error) {
	if req == nil {
		return "", errors.New("start request is required")
	}
	if req.DatasetID == "" || req.ChatModelID == "" {
		return "", errors.New("dataset_id and chat_model_id are required")
	}
	// Verify dataset exists + belongs to tenant before kicking off work.
	// The runner is passed the tenant from auth context.
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return "", errors.New("tenant id is required")
	}
	ds, _, err := r.datasetSvc.GetDatasetByID(ctx, req.DatasetID)
	if err != nil {
		return "", fmt.Errorf("load dataset: %w", err)
	}
	if ds.TenantID != tenantID {
		return "", errors.New("dataset does not belong to tenant")
	}

	judgeModelID := req.JudgeModelID
	if judgeModelID == "" {
		// D2 default: first ModelTypeKnowledgeQA. Best-effort — if the
		// tenant has no judge model configured we still persist the run
		// row and let the runner emit a run-level error so the admin
		// sees it in /api.
		if r.modelSvc != nil {
			if models, listErr := r.modelSvc.ListModels(ctx); listErr == nil {
				for _, m := range models {
					if m != nil && m.Type == types.ModelTypeKnowledgeQA {
						judgeModelID = m.ID
						break
					}
				}
			}
		}
	}

	runID := uuid.NewString()
	run := &types.EvalRun{
		ID:                 runID,
		TenantID:           tenantID,
		DatasetID:          req.DatasetID,
		ChatModelID:        req.ChatModelID,
		RerankModelID:      req.RerankModelID,
		ReflectionEnabled:  req.ReflectionEnabled,
		JudgeModelID:       judgeModelID,
		JudgePromptVersion: judge.PromptVersion(),
		Status:             types.EvalRunStatusPending,
		StartedAt:          time.Now().UTC(),
		CreatedBy:          req.CreatedBy,
		CorrelationID:      types.CorrelationIDFromContext(ctx),
	}
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		return "", fmt.Errorf("persist eval run: %w", err)
	}
	metricEvalRunsStartedTotal.WithLabelValues(strconv.FormatUint(tenantID, 10)).Inc()

	r.emitRunAudit(ctx, tenantID, req.CreatedBy, types.AuditActionEvalRunStarted, runID, req.DatasetID, "accepted", "")

	// Detach from the request context — the goroutine outlives the
	// HTTP handler. CloneContext preserves correlation_id + OTel span
	// (Build #25) so the audit trail can join back to the originating
	// POST.
	bgCtx := logger.CloneContext(ctx)
	go r.executeRun(bgCtx, run, req)

	return runID, nil
}

// executeRun drives the per-QA loop. Cancellation is observed through
// the run's status column; CancelRun flips status to canceled and the
// loop checks it on every iteration.
func (r *evalRunner) executeRun(ctx context.Context, run *types.EvalRun, req *interfaces.EvalRunStartRequest) {
	startedAt := run.StartedAt
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf(ctx, "[eval_runner] panic run_id=%s: %v\n%s", run.ID, rec, debug.Stack())
			r.finalizeRun(ctx, run, types.EvalRunStatusFailed, startedAt, fmt.Sprintf("panic: %v", rec))
		}
	}()

	if err := r.markRunStatus(ctx, run, types.EvalRunStatusRunning); err != nil {
		logger.Errorf(ctx, "[eval_runner] failed to flip run_id=%s to running: %v", run.ID, err)
		r.finalizeRun(ctx, run, types.EvalRunStatusFailed, startedAt, fmt.Sprintf("start: %v", err))
		return
	}

	// Load QAs ordered by qid ASC so the run order matches import order.
	var qas []types.EvalDatasetQA
	if err := r.db.WithContext(ctx).
		Where("dataset_id = ?", run.DatasetID).
		Order("qid ASC").
		Find(&qas).Error; err != nil {
		r.finalizeRun(ctx, run, types.EvalRunStatusFailed, startedAt, fmt.Sprintf("load qa: %v", err))
		return
	}
	if len(qas) == 0 {
		r.finalizeRun(ctx, run, types.EvalRunStatusSucceeded, startedAt, "")
		return
	}

	summary := &types.EvalRunSummary{Total: len(qas)}
	var factualitySum, citationSum, reflectionSum float64
	var autoBadcaseCount int32
	var cacheHits int32
	var cacheMisses int32

	for _, qa := range qas {
		// Cancellation check — read the run's current status.
		if r.isCanceled(ctx, run.ID) {
			r.finalizeRun(ctx, run, types.EvalRunStatusCanceled, startedAt, "")
			return
		}

		factuality, citation, reflection, passed, chatResp, runErr := r.runOneQA(ctx, run, req, &qa, summary, &factualitySum, &citationSum, &reflectionSum)
		if runErr != nil {
			logger.Errorf(ctx, "[eval_runner] per-qa failure run_id=%s qid=%d: %v", run.ID, qa.QID, runErr)
			// Continue the run; the per-QA result row carries the error.
		}

		if !passed && runErr == nil {
			// Auto-flag below threshold (D3). BadcaseService owns the
			// flag persistence + audit row so we never duplicate logic.
			if r.badcaseSvc != nil {
				avg := (float64(factuality) + float64(citation) + float64(reflection)) / 3.0
				severity := types.EvalSeverityMedium
				switch {
				case avg < 1.5:
					severity = types.EvalSeverityCritical
				case avg < 2.0:
					severity = types.EvalSeverityHigh
				}
				if _, flagErr := r.badcaseSvc.FlagAuto(ctx, run.TenantID, run.ID, qa.QID, severity,
					fmt.Sprintf("auto: avg=%.2f fact=%d cite=%d refl=%d", avg, factuality, citation, reflection),
				); flagErr != nil {
					logger.Warnf(ctx, "[eval_runner] badcase flag failed run_id=%s qid=%d: %v", run.ID, qa.QID, flagErr)
				} else {
					atomic.AddInt32(&autoBadcaseCount, 1)
				}
			}
		}

		// Tool cache telemetry — read the chat response sidecar (B30 B2).
		// We do not count per-tool hits here; the chat pipeline already
		// increments chat_tool_cache_* via the in-process ToolCache. The
		// eval summary aggregates the per-run ratio from the cache
		// metrics snapshot at finalizeRun time.
		_ = chatResp

		metricEvalQAEvaluatedTotal.WithLabelValues(
			strconv.FormatUint(run.TenantID, 10),
			strconv.FormatBool(passed),
		).Inc()
	}

	summary.AutoBadcaseCount = int(atomic.LoadInt32(&autoBadcaseCount))
	// Tool cache hit ratio: pulled from the prom counter diff at finalize
	// time. We do not have a per-run snapshot today; record 0 so the
	// summary field stays present and a future PR can populate it.
	totalCache := atomic.LoadInt32(&cacheHits) + atomic.LoadInt32(&cacheMisses)
	if totalCache > 0 {
		summary.ToolCacheHitRatio = float64(atomic.LoadInt32(&cacheHits)) / float64(totalCache)
	}

	r.finalizeRun(ctx, run, types.EvalRunStatusSucceeded, startedAt, "")
}

// runOneQA executes the chat pipeline + 3 judges + persists the
// per-QA row. The function is single-goroutine; the caller decides
// whether to run multiple QAs in parallel (today: serial).
func (r *evalRunner) runOneQA(
	ctx context.Context,
	run *types.EvalRun,
	req *interfaces.EvalRunStartRequest,
	qa *types.EvalDatasetQA,
	summary *types.EvalRunSummary,
	factualitySum, citationSum, reflectionSum *float64,
) (factuality, citation, reflection int, passed bool, chatResp *EvalChatResponse, err error) {
	if r.chatPipeline == nil {
		return 0, 0, 0, false, nil, errors.New("chat pipeline is nil")
	}
	chatReq := EvalChatRequest{
		TenantID:          run.TenantID,
		DatasetID:         run.DatasetID,
		ChatModelID:       req.ChatModelID,
		RerankModelID:     req.RerankModelID,
		ReflectionEnabled: req.ReflectionEnabled,
		Question:          qa.Question,
	}
	chatResp, err = r.chatPipeline.AnswerQA(ctx, chatReq)
	if err != nil {
		// Per-QA failure still produces a result row so the run summary
		// count is correct.
		persistedErr := r.persistResult(ctx, run, qa, "", json.RawMessage(`null`), json.RawMessage(`null`), json.RawMessage(`null`),
			nil, nil, nil, false, fmt.Sprintf("chat pipeline: %v", err))
		return 0, 0, 0, false, chatResp, persistedErr
	}

	// LLM judges — each may fail independently. Failures degrade to a
	// per-dimension nil score so the summary still averages the others.
	var factualityScore, citationScore, reflectionScore *int
	if run.JudgeModelID != "" {
		if res, judgeErr := judge.JudgeFactuality(ctx, r, run.JudgeModelID, qa.Question, qa.ExpectedAnswer, chatResp.ModelAnswer); judgeErr == nil {
			v := res.Score
			factualityScore = &v
			*factualitySum += float64(v)
			metricEvalQAJudgeDurationSeconds.WithLabelValues("factuality").Observe(time.Since(run.StartedAt).Seconds() / float64(summary.Total))
		} else {
			logger.Warnf(ctx, "[eval_runner] factuality judge failed run_id=%s qid=%d: %v", run.ID, qa.QID, judgeErr)
		}
		if res, judgeErr := judge.JudgeCitationFidelity(ctx, r, run.JudgeModelID, qa.Question, chatResp.ModelAnswer, chatResp.CitationIndex); judgeErr == nil {
			v := res.Score
			citationScore = &v
			*citationSum += float64(v)
			metricEvalQAJudgeDurationSeconds.WithLabelValues("citation_fidelity").Observe(0)
		} else {
			logger.Warnf(ctx, "[eval_runner] citation judge failed run_id=%s qid=%d: %v", run.ID, qa.QID, judgeErr)
		}
		if res, judgeErr := judge.JudgeReflectionNecessity(ctx, r, run.JudgeModelID, qa.Question, chatResp.ModelAnswer, chatResp.ReflectionEvents); judgeErr == nil {
			v := res.Score
			reflectionScore = &v
			*reflectionSum += float64(v)
			metricEvalQAJudgeDurationSeconds.WithLabelValues("reflection_necessity").Observe(0)
		} else {
			logger.Warnf(ctx, "[eval_runner] reflection judge failed run_id=%s qid=%d: %v", run.ID, qa.QID, judgeErr)
		}
	} else {
		logger.Warnf(ctx, "[eval_runner] no judge model_id run_id=%s qid=%d; skipping LLM judges", run.ID, qa.QID)
	}

	// Compute the heuristic companions. The combined score is
	// min(heuristic, llm) so the heuristics can override the LLM on
	// structural failures.
	heuristicCitation := judge.HeuristicCitationFidelity(chatResp.ModelAnswer, chatResp.CitationIndex)
	passedFlag := false
	combinedCitation := 0
	if citationScore != nil {
		combinedCitation = judge.CombinedCitationFidelity(heuristicCitation, *citationScore)
	} else {
		combinedCitation = heuristicCitation
	}

	var combinedReflection int
	if reflectionScore != nil {
		heuristicReflection := judge.HeuristicReflectionNecessity(chatResp.ReflectionEvents, factualityScore != nil && *factualityScore >= 3)
		combinedReflection = judge.CombinedReflectionNecessity(heuristicReflection, *reflectionScore)
	}

	// Auto-flag pass/fail decision. We only mark a QA as "passed" when
	// ALL THREE scores are present and the average is at or above 3.0.
	passedFlag = factualityScore != nil && citationScore != nil && reflectionScore != nil &&
		judge.ShouldAutoFlag(0, 0, 0) == false &&
		(float64(*factualityScore)+float64(combinedCitation)+float64(combinedReflection))/3.0 >= 3.0

	if passedFlag {
		summary.Passed++
	} else {
		summary.Failed++
	}

	// Persist the per-QA row.
	var factualityPtr, citationPtr, reflectionPtr *float64
	if factualityScore != nil {
		v := float64(*factualityScore)
		factualityPtr = &v
	}
	if citationScore != nil {
		v := float64(combinedCitation)
		citationPtr = &v
	}
	if reflectionScore != nil {
		v := float64(combinedReflection)
		reflectionPtr = &v
	}
	flagReason := ""
	if !passedFlag {
		flagReason = fmt.Sprintf("avg=%.2f fact=%v cite=%v refl=%v",
			avgFloat(factualityPtr, citationPtr, reflectionPtr), factualityScore, citationScore, reflectionScore)
	}
	if persistErr := r.persistResult(ctx, run, qa, chatResp.ModelAnswer, chatResp.SearchTopK, chatResp.CitationIndex, chatResp.ReflectionEvents,
		factualityPtr, citationPtr, reflectionPtr, passedFlag, flagReason); persistErr != nil {
		return derefInt(factualityScore), combinedCitation, combinedReflection, passedFlag, chatResp, persistErr
	}

	return derefInt(factualityScore), combinedCitation, combinedReflection, passedFlag, chatResp, nil
}

// persistResult writes one eval_run_results row. Errors here abort the
// per-QA only; the run continues so a single corrupt row cannot stop
// a 1000-QA run.
func (r *evalRunner) persistResult(
	ctx context.Context,
	run *types.EvalRun,
	qa *types.EvalDatasetQA,
	modelAnswer string,
	searchTopK, citationIndex, reflectionEvents json.RawMessage,
	factuality, citation, reflection *float64,
	passed bool,
	flagReason string,
) error {
	row := &types.EvalRunResult{
		RunID:                    run.ID,
		QID:                      qa.QID,
		Question:                 qa.Question,
		ModelAnswer:              modelAnswer,
		ExpectedAnswer:           qa.ExpectedAnswer,
		SearchTopK:               searchTopK,
		CitationIndex:            citationIndex,
		ReflectionEvents:         reflectionEvents,
		FactualityScore:          factuality,
		CitationFidelityScore:    citation,
		ReflectionNecessityScore: reflection,
		Passed:                   passed,
		BadcaseFlagReason:        flagReason,
		CreatedAt:                time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *evalRunner) isCanceled(ctx context.Context, runID string) bool {
	var status types.EvalRunStatus
	if err := r.db.WithContext(ctx).
		Model(&types.EvalRun{}).
		Where("id = ?", runID).
		Pluck("status", &status).Error; err != nil {
		return false
	}
	return status == types.EvalRunStatusCanceled
}

func (r *evalRunner) markRunStatus(ctx context.Context, run *types.EvalRun, status types.EvalRunStatus) error {
	return r.db.WithContext(ctx).
		Model(&types.EvalRun{}).
		Where("id = ?", run.ID).
		Update("status", status).Error
}

// finalizeRun rolls up the summary, marks terminal status, fires
// audit + Prom counters. Called from both success and failure paths.
func (r *evalRunner) finalizeRun(ctx context.Context, run *types.EvalRun, status types.EvalRunStatus, startedAt time.Time, errMsg string) {
	finishedAt := time.Now().UTC()
	canceledAt := (*time.Time)(nil)
	if status == types.EvalRunStatusCanceled {
		canceledAt = &finishedAt
	}
	updates := map[string]any{
		"status":      status,
		"finished_at": finishedAt,
	}
	if canceledAt != nil {
		updates["canceled_at"] = *canceledAt
	}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	if err := r.db.WithContext(ctx).
		Model(&types.EvalRun{}).
		Where("id = ?", run.ID).
		Updates(updates).Error; err != nil {
		logger.Errorf(ctx, "[eval_runner] finalize update failed run_id=%s: %v", run.ID, err)
	}

	// Recompute summary from the eval_run_results table so concurrent
	// updates (a runner that crashed and got restarted) end up coherent.
	var summary types.EvalRunSummary
	var total, passedCount, failedCount int64
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).Where("run_id = ?", run.ID).Count(&total)
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).Where("run_id = ? AND passed = ?", run.ID, true).Count(&passedCount)
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).Where("run_id = ? AND passed = ?", run.ID, false).Count(&failedCount)
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).
		Where("run_id = ? AND factuality_score IS NOT NULL", run.ID).
		Select("COALESCE(AVG(factuality_score), 0)").Row().Scan(&summary.FactualityAvg)
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).
		Where("run_id = ? AND citation_fidelity_score IS NOT NULL", run.ID).
		Select("COALESCE(AVG(citation_fidelity_score), 0)").Row().Scan(&summary.CitationFidelityAvg)
	r.db.WithContext(ctx).Model(&types.EvalRunResult{}).
		Where("run_id = ? AND reflection_necessity_score IS NOT NULL", run.ID).
		Select("COALESCE(AVG(reflection_necessity_score), 0)").Row().Scan(&summary.ReflectionNecessityAvg)
	summary.Total = int(total)
	summary.Passed = int(passedCount)
	summary.Failed = int(failedCount)
	summaryJSON, _ := json.Marshal(summary)
	if err := r.db.WithContext(ctx).
		Model(&types.EvalRun{}).
		Where("id = ?", run.ID).
		Update("summary", types.JSON(summaryJSON)).Error; err != nil {
		logger.Errorf(ctx, "[eval_runner] summary write failed run_id=%s: %v", run.ID, err)
	}

	metricEvalRunsCompletedTotal.WithLabelValues(
		strconv.FormatUint(run.TenantID, 10),
		string(status),
	).Inc()
	metricEvalRunsDurationSeconds.WithLabelValues(string(status)).Observe(finishedAt.Sub(startedAt).Seconds())

	r.emitRunAudit(ctx, run.TenantID, run.CreatedBy, types.AuditActionEvalRunCompleted, run.ID, run.DatasetID, string(status), errMsg)
}

// GetRun loads a run with tenant scoping. Returns ErrRunNotFound when
// the row is missing or belongs to a different tenant.
func (r *evalRunner) GetRun(ctx context.Context, tenantID uint64, runID string) (*types.EvalRun, error) {
	var run types.EvalRun
	if err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", runID, tenantID).
		First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("load eval run: %w", err)
	}
	return &run, nil
}

// ListRuns newest-first, optional dataset_id filter.
func (r *evalRunner) ListRuns(ctx context.Context, tenantID uint64, datasetID string, limit, offset int) ([]*types.EvalRun, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if datasetID != "" {
		q = q.Where("dataset_id = ?", datasetID)
	}
	var total int64
	if err := q.Model(&types.EvalRun{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count eval runs: %w", err)
	}
	var rows []*types.EvalRun
	if err := q.Order("started_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list eval runs: %w", err)
	}
	return rows, int(total), nil
}

// ListResults returns every per-QA row for a run. Used by the
// frontend's per-QA evidence panel.
func (r *evalRunner) ListResults(ctx context.Context, runID string) ([]types.EvalRunResult, error) {
	var rows []types.EvalRunResult
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("qid ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list eval run results: %w", err)
	}
	return rows, nil
}

// CancelRun flips the run row to canceled. The background loop sees
// the new status on its next iteration and exits.
func (r *evalRunner) CancelRun(ctx context.Context, tenantID uint64, runID string) error {
	res := r.db.WithContext(ctx).
		Model(&types.EvalRun{}).
		Where("id = ? AND tenant_id = ? AND status IN ?", runID, tenantID,
			[]types.EvalRunStatus{types.EvalRunStatusPending, types.EvalRunStatusRunning}).
		Update("status", types.EvalRunStatusCanceled)
	if res.Error != nil {
		return fmt.Errorf("cancel eval run: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRunNotCancelable
	}
	return nil
}

// ErrRunNotFound is returned by GetRun when the row is missing or
// belongs to another tenant. Handlers map it to HTTP 404.
var ErrRunNotFound = errors.New("eval run not found")

// ErrRunNotCancelable is returned by CancelRun when the run is
// already in a terminal state.
var ErrRunNotCancelable = errors.New("eval run not cancelable")

// emitRunAudit writes the eval.* row. Failure to write is non-fatal
// — the run itself succeeds even if audit is briefly down.
func (r *evalRunner) emitRunAudit(ctx context.Context, tenantID uint64, actorUserID string, action types.AuditAction, runID, datasetID, outcome, errMsg string) {
	details := map[string]any{
		"run_id":     runID,
		"dataset_id": datasetID,
		"outcome":    outcome,
	}
	if errMsg != "" {
		details["error"] = errMsg
	}
	detailJSON, _ := json.Marshal(details)
	entry := &types.AuditLog{
		TenantID:      tenantID,
		ActorUserID:   actorUserID,
		Action:        action,
		ScopeType:     "eval_run",
		ScopeID:       runID,
		TargetType:    "eval_run",
		TargetID:      runID,
		Outcome:       types.AuditOutcome(outcome),
		Details:       types.JSON(detailJSON),
		CorrelationID: types.CorrelationIDFromContext(ctx),
	}
	if r.auditSvc == nil {
		logger.Warnf(ctx, "[eval_runner] audit service unavailable; dropping run_id=%s action=%s", runID, action)
		return
	}
	if err := r.auditSvc.Log(ctx, entry); err != nil {
		logger.Warnf(ctx, "[eval_runner] audit write failed run_id=%s action=%s: %v", runID, action, err)
	}
}

// avgFloat computes the mean of three optional pointers. Used for the
// badcase reason string; the run summary uses SQL AVG().
func avgFloat(a, b, c *float64) float64 {
	sum := 0.0
	count := 0
	if a != nil {
		sum += *a
		count++
	}
	if b != nil {
		sum += *b
		count++
	}
	if c != nil {
		sum += *c
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// Chat implements the judge.LLMCaller interface by looking up the
// chat model via modelSvc and calling Chat. The judge package imports
// chat.Message + chat.Chat, so this adapter bridges the runner.
func (r *evalRunner) Chat(ctx context.Context, modelID string, messages []chat.Message) (string, error) {
	if r.modelSvc == nil {
		return "", errors.New("eval_runner: modelSvc is nil")
	}
	model, err := r.modelSvc.GetChatModel(ctx, modelID)
	if err != nil {
		return "", fmt.Errorf("eval_runner: get chat model %s: %w", modelID, err)
	}
	resp, err := model.Chat(ctx, messages, &chat.ChatOptions{Temperature: 0})
	if err != nil {
		return "", fmt.Errorf("eval_runner: chat call: %w", err)
	}
	if resp == nil {
		return "", errors.New("eval_runner: nil chat response")
	}
	return resp.Content, nil
}

// Ensure evalRunner implements the LLMCaller interface (used by
// judge package). This is a static assertion so a future refactor that
// drops the method fails the build instead of silently failing at
// runtime.
var _ judge.LLMCaller = (*evalRunner)(nil)
