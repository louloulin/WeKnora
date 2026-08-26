package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubWikiBatchJobRepo is the in-memory WikiBatchJobRepository used by
// the async-batch harness. Real persistence is exercised by the smoke
// script against PostgreSQL.
//
// Build #13.
type stubWikiBatchJobRepo struct {
	mu   sync.Mutex
	jobs map[string]*types.WikiBatchJob
}

func newStubWikiBatchJobRepo() *stubWikiBatchJobRepo {
	return &stubWikiBatchJobRepo{jobs: map[string]*types.WikiBatchJob{}}
}

func (r *stubWikiBatchJobRepo) Create(_ context.Context, job *types.WikiBatchJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[job.ID]; ok {
		return fmt.Errorf("dup id %s", job.ID)
	}
	cp := *job
	r.jobs[job.ID] = &cp
	return nil
}

func (r *stubWikiBatchJobRepo) GetByID(_ context.Context, id string) (*types.WikiBatchJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, types.ErrWikiBatchJobNotFound
	}
	cp := *job
	return &cp, nil
}

func (r *stubWikiBatchJobRepo) Update(_ context.Context, job *types.WikiBatchJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[job.ID]; !ok {
		return types.ErrWikiBatchJobNotFound
	}
	cp := *job
	r.jobs[job.ID] = &cp
	return nil
}

// ClaimNextQueued — simplified single-instance model: pick the
// oldest queued job, advance to running in-memory. Mirrors the SQLite
// fallback path in the production repo (the harness never runs against
// Postgres).
func (r *stubWikiBatchJobRepo) ClaimNextQueued(_ context.Context) (*types.WikiBatchJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var oldest *types.WikiBatchJob
	for _, job := range r.jobs {
		if job.State != types.WikiBatchJobStateQueued {
			continue
		}
		if oldest == nil || job.CreatedAt.Before(oldest.CreatedAt) {
			oldest = job
		}
	}
	if oldest == nil {
		return nil, types.ErrWikiBatchJobNone
	}
	now := time.Now()
	oldest.State = types.WikiBatchJobStateRunning
	oldest.StartedAt = &now
	cp := *oldest
	r.jobs[cp.ID] = &cp
	return &cp, nil
}

func (r *stubWikiBatchJobRepo) ListExpired(_ context.Context, now time.Time, _ int) ([]*types.WikiBatchJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.WikiBatchJob
	for _, job := range r.jobs {
		if job.ExpiresAt != nil && now.After(*job.ExpiresAt) {
			cp := *job
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *stubWikiBatchJobRepo) DeleteByID(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.jobs, id)
	return nil
}

// newBatchSvcForTest wires a minimal wikiPageService: stubBatchRepo
// for the page repo, nil everywhere else. Tests that need ACL or
// queue-async pass the result through SetBatchJobService.
func newBatchSvcForTest(repo *stubBatchRepo) *wikiPageService {
	return &wikiPageService{repo: repo}
}

// TestBatchMovePages_SyncPath_Lt20 — sub-threshold slug count keeps
// the synchronous path. Verifies A2 / spec A2.
func TestBatchMovePages_SyncPath_Lt20(t *testing.T) {
	repo := newStubBatchRepo()
	for i := 0; i < 5; i++ {
		repo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	svc := newBatchSvcForTest(repo)
	result, err := svc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(5), "folderX", "user-1")
	if err != nil {
		t.Fatalf("sync path: %v", err)
	}
	if result.Kind != "sync" {
		t.Fatalf("kind = %q, want sync", result.Kind)
	}
	if result.Result == nil {
		t.Fatalf("nil result")
	}
	if len(result.Result.Succeeded) != 5 {
		t.Fatalf("succeeded len = %d, want 5", len(result.Result.Succeeded))
	}
}

// TestBatchMovePages_AsyncPath_Gte20 — at or above threshold the
// request is enqueued. We capture Kind=job without depending on the
// worker pool finishing in time.
//
// Build #13 (A2 / spec A2).
func TestBatchMovePages_AsyncPath_Gte20(t *testing.T) {
	repo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		repo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(repo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "user-1")
	if err != nil {
		t.Fatalf("async path: %v", err)
	}
	if result.Kind != "job" {
		t.Fatalf("kind = %q, want job", result.Kind)
	}
	if result.Job == nil || result.Job.ID == "" {
		t.Fatalf("missing job: %+v", result)
	}
}

// TestWorkerPool_PicksUpQueuedJob — the worker drains a queued job
// and lands it in succeeded/partial with a result blob.
//
// Build #13.
func TestWorkerPool_PicksUpQueuedJob(t *testing.T) {
	repo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		repo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(repo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "user-1")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, gErr := jobRepo.GetByID(context.Background(), jobID)
		if gErr != nil {
			t.Fatalf("poll: %v", gErr)
		}
		if got.State == types.WikiBatchJobStateSucceeded || got.State == types.WikiBatchJobStatePartial || got.State == types.WikiBatchJobStateFailed {
			if got.Result == nil {
				t.Fatalf("state %s but no result blob", got.State)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not finish in 5s (state=%s)", got.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestUndoJob_MoveRestoresFolder — undoing a move batch rewrites
// folder_id back to its captured value.
//
// Build #13 (A6).
func TestUndoJob_MoveRestoresFolder(t *testing.T) {
	repo := newStubBatchRepo()
	for i, folder := range []string{"alpha", "beta", "gamma"} {
		repo.addPage(fmt.Sprintf("p%d", i), "published", nil, folder)
	}
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(repo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })

	// Snapshot pre-move state via CaptureUndoState.
	undo, err := CaptureUndoState(context.Background(), pageSvc, "kb1", []string{"p0", "p1", "p2"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Move every page to delta.
	for _, slug := range []string{"p0", "p1", "p2"} {
		page, _ := repo.GetBySlug(context.Background(), "kb1", slug)
		page.FolderID = "delta"
		_ = repo.UpdateMeta(context.Background(), page)
	}

	job := &types.WikiBatchJob{
		ID:              "job-test-move-undo",
		KnowledgeBaseID: "kb1",
		Type:            types.WikiBatchJobTypeMove,
		UndoState:       undo,
		State:           types.WikiBatchJobStateSucceeded,
	}
	_ = jobRepo.Create(context.Background(), job)

	if _, err := batchSvc.UndoJob(context.Background(), "kb1", job.ID, "user-1"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		got, _ := repo.GetBySlug(context.Background(), "kb1", fmt.Sprintf("p%d", i))
		if got.FolderID != want {
			t.Fatalf("p%d folder = %q, want %q", i, got.FolderID, want)
		}
	}
}

// TestUndoJob_DeleteRestoresPages — after a delete batch, undo brings
// the page back with a __restored_<short> slug suffix.
//
// Build #13 (A6 / D7).
func TestUndoJob_DeleteRestoresPages(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("p0", "published", nil, "alpha")
	repo.addPage("p1", "published", nil, "alpha")
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(repo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })

	// Snapshot pre-delete state, then soft-delete.
	undo, err := CaptureUndoState(context.Background(), pageSvc, "kb1", []string{"p0", "p1"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	for _, slug := range []string{"p0", "p1"} {
		_ = repo.Delete(context.Background(), "kb1", slug)
	}

	job := &types.WikiBatchJob{
		ID:              "job-test-delete-undo-shortid",
		KnowledgeBaseID: "kb1",
		Type:            types.WikiBatchJobTypeDelete,
		UndoState:       undo,
		State:           types.WikiBatchJobStateSucceeded,
	}
	_ = jobRepo.Create(context.Background(), job)

	if _, err := batchSvc.UndoJob(context.Background(), "kb1", job.ID, "user-1"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	for _, slug := range []string{"p0", "p1"} {
		wantPrefix := slug + "__restored_job-test"
		found := false
		for s := range repo.pages {
			if strings.HasPrefix(s, wantPrefix) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("restored slug for %s not found", slug)
		}
	}
}

// TestUndoJob_StatusReturnsNotUndoable — status jobs are not undoable.
//
// Build #13 (A6).
func TestUndoJob_StatusReturnsNotUndoable(t *testing.T) {
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(newStubBatchRepo())
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })

	job := &types.WikiBatchJob{
		ID:              "job-test-status",
		KnowledgeBaseID: "kb1",
		Type:            types.WikiBatchJobTypeStatus,
		State:           types.WikiBatchJobStateSucceeded,
	}
	_ = jobRepo.Create(context.Background(), job)
	_, err := batchSvc.UndoJob(context.Background(), "kb1", job.ID, "user-1")
	if !errors.Is(err, types.ErrWikiBatchJobNotUndoable) {
		t.Fatalf("err = %v, want ErrWikiBatchJobNotUndoable", err)
	}
}

// TestUndoJob_AfterExpiryReturns410 — past expires_at returns the
// expired sentinel.
//
// Build #13 (A6 / D3).
func TestUndoJob_AfterExpiryReturns410(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("p0", "published", nil, "alpha")
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(repo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })

	undo, err := CaptureUndoState(context.Background(), pageSvc, "kb1", []string{"p0"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	past := time.Now().Add(-1 * time.Hour)
	job := &types.WikiBatchJob{
		ID:              "job-test-expired",
		KnowledgeBaseID: "kb1",
		Type:            types.WikiBatchJobTypeMove,
		UndoState:       undo,
		State:           types.WikiBatchJobStateSucceeded,
		ExpiresAt:       &past,
	}
	_ = jobRepo.Create(context.Background(), job)
	_, err = batchSvc.UndoJob(context.Background(), "kb1", job.ID, "user-1")
	if !errors.Is(err, types.ErrWikiBatchJobExpired) {
		t.Fatalf("err = %v, want ErrWikiBatchJobExpired", err)
	}
}

// TestCrossKBJobReturns404 — passing a job_id from KB-A when querying
// under KB-B returns ErrWikiBatchJobNotFound so existence doesn't
// leak across KBs.
//
// Build #13 (A5).
func TestCrossKBJobReturns404(t *testing.T) {
	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(newStubBatchRepo())
	batchSvc := NewWikiBatchJobService(jobRepo, nil, nil, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })

	job := &types.WikiBatchJob{
		ID:              "job-test-cross-kb",
		KnowledgeBaseID: "kb-A",
		Type:            types.WikiBatchJobTypeMove,
		State:           types.WikiBatchJobStateSucceeded,
	}
	_ = jobRepo.Create(context.Background(), job)

	_, err := batchSvc.UndoJob(context.Background(), "kb-B", job.ID, "user-1")
	if !errors.Is(err, types.ErrWikiBatchJobNotFound) {
		t.Fatalf("err = %v, want ErrWikiBatchJobNotFound", err)
	}
}

func makeSlugs(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("p%d", i)
	}
	return out
}