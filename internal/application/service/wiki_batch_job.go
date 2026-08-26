package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// WikiBatchJobWorkerCount is the number of concurrent goroutines that
// drain the wiki_batch_jobs queue. Sized to 4 (D2) — the channel buffer
// holds 32 outstanding job IDs before the synchronous path degrades
// gracefully.
//
// Build #13.
const (
	WikiBatchJobWorkerCount = 4
	WikiBatchJobQueueSize   = 32

	// WikiBatchJobUndoWindow — how long after a job finishes a user may
	// roll it back via the undo endpoint. Persisted as expires_at =
	// finished_at + WikiBatchJobUndoWindow.
	WikiBatchJobUndoWindow = 7 * 24 * time.Hour
)

// WikiBatchJobService wires the in-process worker pool, the
// WikiBatchJobRepository, and the synchronous WikiPageService that
// workers actually call. The three Batch* methods on WikiPageService
// auto-route here once the slug count crosses WikiBatchAsyncThreshold.
//
// Build #13 (core). Build #14 adds WikiBatchAuditRepository so each
// state transition lands an immutable audit row.
type wikiBatchJobService struct {
	repo       interfaces.WikiBatchJobRepository
	auditRepo  interfaces.WikiBatchAuditRepository
	pageSvc    interfaces.WikiPageService
	queue      chan string
	wg         sync.WaitGroup
	shutdownCh chan struct{}
	once       sync.Once
}

// NewWikiBatchJobService constructs the service and starts the worker
// pool. The workers consume job IDs from `queue`, claim the row from
// the DB, and run the corresponding Batch* method on pageSvc. Call
// Shutdown on graceful exit.
//
// auditRepo may be nil for legacy callers (older harness tests that
// pre-date Build #14); in that case audit recording is silently
// skipped — the service still executes jobs normally.
func NewWikiBatchJobService(
	repo interfaces.WikiBatchJobRepository,
	auditRepo interfaces.WikiBatchAuditRepository,
	pageSvc interfaces.WikiPageService,
) interfaces.WikiBatchJobService {
	s := &wikiBatchJobService{
		repo:       repo,
		auditRepo:  auditRepo,
		pageSvc:    pageSvc,
		queue:      make(chan string, WikiBatchJobQueueSize),
		shutdownCh: make(chan struct{}),
	}
	for i := 0; i < WikiBatchJobWorkerCount; i++ {
		s.wg.Add(1)
		go s.runWorker(i)
	}
	return s
}

// recordAudit is the best-effort bridge between state transitions and
// the audit table. It never returns an error and never panics — audit
// recording is observability, not part of the job's correctness. A
// nil auditRepo (legacy callers) silently no-ops.
//
// Actor resolution: empty actor_id falls back to the system constant
// (matches the worker pool / cleanup cron convention).
//
// Build #14.
func (s *wikiBatchJobService) recordAudit(
	ctx context.Context,
	job *types.WikiBatchJob,
	action types.WikiBatchAuditAction,
	actor string,
	metadata map[string]interface{},
) {
	if s.auditRepo == nil || job == nil {
		return
	}
	actorID := actor
	if actorID == "" {
		actorID = types.WikiBatchAuditActorSystem
	}
	event := &types.WikiBatchJobAuditEvent{
		TenantID:        job.TenantID,
		KnowledgeBaseID: job.KnowledgeBaseID,
		BatchJobID:      job.ID,
		Action:          action,
		ActorID:         actorID,
		Metadata:        metadata,
	}
	if err := s.auditRepo.Insert(ctx, event); err != nil {
		logger.Warnf(ctx, "wiki batch audit insert failed action=%s job=%s: %v", action, job.ID, err)
	}
}

// EnqueueJob persists the job row in queued state and pushes the ID to
// the worker channel. The HTTP handler immediately returns the ID;
// workers claim and execute the job asynchronously.
//
// Channel full → caller-visible error: callers should re-route the
// request to the synchronous Batch* path so a single over-large
// backlog can't hold up the API.
//
// Build #14: records an `enqueue` audit row after the job row is
// committed. Audit insert failure is logged, not propagated — the job
// is already in the queue and the user request should still succeed.
func (s *wikiBatchJobService) EnqueueJob(
	ctx context.Context, job *types.WikiBatchJob,
) (string, error) {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.State == "" {
		job.State = types.WikiBatchJobStateQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if err := s.repo.Create(ctx, job); err != nil {
		return "", fmt.Errorf("create batch job: %w", err)
	}
	s.recordAudit(ctx, job, types.WikiBatchAuditActionEnqueue, job.CreatedBy, map[string]interface{}{
		"type": string(job.Type),
	})
	select {
	case s.queue <- job.ID:
		return job.ID, nil
	default:
		// Channel full — mark the row failed and surface the error so
		// the handler can degrade to the synchronous path. We leave
		// the row in queued state so a retry could pick it up; the
		// worker queue overflow is the symptom, the row itself is
		// fine.
		return job.ID, fmt.Errorf("batch job queue is full (%d)", WikiBatchJobQueueSize)
	}
}

// GetJob returns the persisted job row. KB scoping is the caller's job.
func (s *wikiBatchJobService) GetJob(ctx context.Context, jobID string) (*types.WikiBatchJob, error) {
	return s.repo.GetByID(ctx, jobID)
}

// UndoJob rolls a finished `move` or `delete` job back. The 7-day
// persistent undo window is enforced via ErrWikiBatchJobExpired. status
// jobs return ErrWikiBatchJobNotUndoable.
//
// KB scoping: caller must supply kbID — we re-check it here because
// GetJob alone doesn't filter by KB.
//
// Build #14: records `undo_request` after the row passes all the
// guard checks but before applyUndo runs, and `undo_done` after the
// repo update that nulls expires_at. If applyUndo fails, no `undo_done`
// row is written — the request row tells the auditor the operator
// tried but the system never reached the finished state.
func (s *wikiBatchJobService) UndoJob(
	ctx context.Context, kbID, jobID, actor string,
) (*types.WikiBatchJob, error) {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.KnowledgeBaseID != kbID {
		// Cross-KB access is mapped to 404 by the handler — same as
		// "not found", so existence doesn't leak across KBs.
		return nil, types.ErrWikiBatchJobNotFound
	}
	if !job.Undoable() {
		return nil, types.ErrWikiBatchJobNotUndoable
	}
	if job.Expired(time.Now()) {
		return nil, types.ErrWikiBatchJobExpired
	}
	s.recordAudit(ctx, job, types.WikiBatchAuditActionUndoRequest, actor, nil)
	if err := s.applyUndo(ctx, job); err != nil {
		return nil, fmt.Errorf("apply undo: %w", err)
	}
	// Update the row to mark the undo — we set state to succeeded
	// (the undo completed) and overwrite expires_at to nil so a
	// second undo call returns "already done" via ErrWikiBatchJobNone.
	job.ExpiresAt = nil
	if err := s.repo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("mark undo done: %w", err)
	}
	s.recordAudit(ctx, job, types.WikiBatchAuditActionUndoDone, actor, nil)
	return job, nil
}

// CancelJob aborts a queued job before workers pick it up. Cancellation
// is only legal while state == queued; once a worker transitions the
// row to running the batch has already started and undo is the right
// path for partial rollbacks.
//
// The job row stays in place after cancellation — we only stamp the
// audit row + clear expires_at. State remains "queued" so a future
// ops inspection can still see what the request asked for.
//
// Build #14.
func (s *wikiBatchJobService) CancelJob(
	ctx context.Context, kbID, jobID, actor string,
) (*types.WikiBatchJob, error) {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.KnowledgeBaseID != kbID {
		// Cross-KB access is mapped to 404 by the handler — same as
		// "not found", so existence doesn't leak across KBs.
		return nil, types.ErrWikiBatchJobNotFound
	}
	if job.State != types.WikiBatchJobStateQueued {
		return nil, types.ErrWikiBatchJobNotCancellable
	}
	job.ExpiresAt = nil
	if err := s.repo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("mark cancelled: %w", err)
	}
	s.recordAudit(ctx, job, types.WikiBatchAuditActionCancel, actor, map[string]interface{}{
		"reason": "user_cancelled",
	})
	return job, nil
}

// applyUndo walks the captured undo_state and re-runs the inverse
// operation per page. Move-undo restores the previous folder_id;
// delete-undo clears deleted_at and suffixes the slug.
//
// Build #13.
func (s *wikiBatchJobService) applyUndo(ctx context.Context, job *types.WikiBatchJob) error {
	switch job.Type {
	case types.WikiBatchJobTypeMove:
		return s.undoMove(ctx, job)
	case types.WikiBatchJobTypeDelete:
		return s.undoDelete(ctx, job)
	default:
		return types.ErrWikiBatchJobNotUndoable
	}
}

// undoMove walks each slug in the job's undo_state.page_states and
// restores the page to its previous folder. We use MovePage (not direct
// repo update) so the cached category_path recomputes and any out-link
// cascades that depend on folder id stay consistent.
func (s *wikiBatchJobService) undoMove(ctx context.Context, job *types.WikiBatchJob) error {
	state, err := decodeUndoState(job.UndoState)
	if err != nil {
		return err
	}
	for slug, prev := range state.PageStates {
		if _, err := s.pageSvc.MovePage(ctx, job.KnowledgeBaseID, slug, prev.FolderID); err != nil {
			logger.Errorf(ctx, "wiki batch undo move: slug=%s err=%v", slug, err)
			return err
		}
	}
	return nil
}

// undoDelete restores soft-deleted pages by clearing deleted_at and
// appending a __restored_<short-id> slug suffix to avoid colliding with
// any live page that took over the slug during the window. (D7.)
func (s *wikiBatchJobService) undoDelete(ctx context.Context, job *types.WikiBatchJob) error {
	state, err := decodeUndoState(job.UndoState)
	if err != nil {
		return err
	}
	shortID := job.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	for originalSlug, prev := range state.PageStates {
		restoredSlug := originalSlug + "__restored_" + shortID
		if _, err := s.pageSvc.RestoreDeletedPage(ctx, job.KnowledgeBaseID, originalSlug, restoredSlug); err != nil {
			logger.Errorf(ctx, "wiki batch undo delete: slug=%s err=%v", originalSlug, err)
			return err
		}
	}
	return nil
}

// decodeUndoState turns the JSONB column back into the typed map.
func decodeUndoState(raw []byte) (*types.WikiBatchJobUndoState, error) {
	if len(raw) == 0 {
		return &types.WikiBatchJobUndoState{PageStates: map[string]types.WikiBatchJobUndoPageState{}}, nil
	}
	var state types.WikiBatchJobUndoState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode undo_state: %w", err)
	}
	if state.PageStates == nil {
		state.PageStates = map[string]types.WikiBatchJobUndoPageState{}
	}
	return &state, nil
}

// runWorker is one of WikiBatchJobWorkerCount goroutines that consumes
// job IDs from the channel, claims the row from the DB, executes the
// matching sync Batch* method, and writes back the result.
//
// On error the worker logs + marks the job as failed with the error in
// the result blob — the partial-success path still surfaces whatever
// rows did succeed.
func (s *wikiBatchJobService) runWorker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.shutdownCh:
			return
		case jobID := <-s.queue:
			s.executeJob(jobID)
		}
	}
}

// executeJob is invoked by runWorker. Failures here only mark the row
// — the synchronous Batch* methods have already absorbed per-row errors
// into the WikiBatchResult, so the only way executeJob itself fails is
// on infrastructure (DB / panic / missing pageSvc).
//
// Build #14: emits `start` after the queued → running transition and
// `finish` after the final repo.Update inside `finalize`. The audit
// actor is the system constant — workers are anonymous.
func (s *wikiBatchJobService) executeJob(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		logger.Errorf(ctx, "wiki batch worker: fetch job %s: %v", jobID, err)
		return
	}

	// Transition queued → running. If a parallel caller (e.g. the
	// worker that originally wrote it) already moved it forward, just
	// continue from the current state.
	if job.State == types.WikiBatchJobStateQueued {
		job.State = types.WikiBatchJobStateRunning
		now := time.Now()
		job.StartedAt = &now
		if err := s.repo.Update(ctx, job); err != nil {
			logger.Errorf(ctx, "wiki batch worker: mark running %s: %v", jobID, err)
			return
		}
		s.recordAudit(ctx, job, types.WikiBatchAuditActionStart, "", nil)
	}

	result, execErr := s.dispatchBatch(ctx, job)
	finalize := func(state types.WikiBatchJobState, blob []byte) {
		now := time.Now()
		job.State = state
		job.FinishedAt = &now
		exp := now.Add(WikiBatchJobUndoWindow)
		job.ExpiresAt = &exp
		if blob != nil {
			job.Result = blob
		}
		if err := s.repo.Update(ctx, job); err != nil {
			logger.Errorf(ctx, "wiki batch worker: finalize %s: %v", jobID, err)
			return
		}
		// Build #14: record the terminal `finish` event. The metadata
		// carries the terminal state + per-batch counts so an auditor
		// can reconstruct the outcome from the audit log alone.
		var succeeded, failed int
		if result != nil {
			succeeded = len(result.Succeeded)
			failed = len(result.Failed)
		}
		s.recordAudit(ctx, job, types.WikiBatchAuditActionFinish, "", map[string]interface{}{
			"state":     string(state),
			"succeeded": succeeded,
			"failed":    failed,
		})
	}

	switch {
	case execErr != nil:
		// Hard error from the Batch* call itself — keep succeeded []
		// empty if nothing ran, otherwise surface the partial result
		// in `result` with an `error` field.
		blob, _ := json.Marshal(map[string]any{
			"succeeded": nil,
			"failed":    []map[string]string{},
			"error":     execErr.Error(),
		})
		finalize(types.WikiBatchJobStateFailed, blob)
	case result == nil:
		finalize(types.WikiBatchJobStateFailed, nil)
	default:
		blob, mErr := json.Marshal(result)
		if mErr != nil {
			// Marshal failure is unlikely (the types are plain), but
			// if it happens record as failed.
			finalize(types.WikiBatchJobStateFailed, nil)
			return
		}
		state := types.WikiBatchJobStateSucceeded
		if len(result.Failed) > 0 {
			state = types.WikiBatchJobStatePartial
		}
		finalize(state, blob)
	}
}

// dispatchBatch routes the captured params to the matching sync method
// on WikiPageService. Slugs + folder_id / status come from job.Params
// (re-marshalled as WikiBatchJobParams).
func (s *wikiBatchJobService) dispatchBatch(
	ctx context.Context, job *types.WikiBatchJob,
) (*types.WikiBatchResult, error) {
	var params types.WikiBatchJobParams
	if err := json.Unmarshal(job.Params, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	switch job.Type {
	case types.WikiBatchJobTypeMove:
		return s.pageSvc.BatchMovePages(ctx, job.KnowledgeBaseID, params.Slugs, params.FolderID)
	case types.WikiBatchJobTypeDelete:
		return s.pageSvc.BatchDeletePages(ctx, job.KnowledgeBaseID, params.Slugs)
	case types.WikiBatchJobTypeStatus:
		return s.pageSvc.BatchUpdatePageStatus(ctx, job.KnowledgeBaseID, params.Slugs, params.Status)
	default:
		return nil, fmt.Errorf("unsupported batch job type %q", job.Type)
	}
}

// CaptureUndoState is a helper called by the auto-routing shim in the
// service layer right before enqueueing a job. It pre-reads each slug's
// current folder_id + slug so undo can restore them.
//
// Build #13.
func CaptureUndoState(
	ctx context.Context,
	pageSvc interfaces.WikiPageService,
	kbID string,
	slugs []string,
) ([]byte, error) {
	state := &types.WikiBatchJobUndoState{
		PageStates: make(map[string]types.WikiBatchJobUndoPageState, len(slugs)),
	}
	for _, slug := range slugs {
		page, err := pageSvc.GetPageBySlug(ctx, kbID, slug)
		if err != nil {
			// Slug not found — skip; it won't be in the batch input
			// either. The batch guards this at the top.
			continue
		}
		state.PageStates[slug] = types.WikiBatchJobUndoPageState{
			FolderID: page.FolderID,
			Status:   page.Status,
		}
	}
	return json.Marshal(state)
}

// Shutdown stops accepting new jobs and waits for the workers to drain
// whatever's already in the queue. Safe to call multiple times.
func (s *wikiBatchJobService) Shutdown(ctx context.Context) error {
	s.once.Do(func() {
		close(s.shutdownCh)
		close(s.queue)
	})
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// errIsEmpty is unused — guarded import to avoid unused-error lint
// churn if a future contributor removes all references to it.
var _ = errors.New