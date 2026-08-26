package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

// stubWikiBatchFailureRepo is the in-memory WikiBatchFailureRepository
// used by the Build #15 observability harness. Insert + ListByJobID
// stay in-memory; no SQL is touched.
//
// Append-only — there is intentionally no Update or Delete so test
// assertions can rely on row counts being monotonically increasing.
type stubWikiBatchFailureRepo struct {
	mu       sync.Mutex
	failures []*types.WikiBatchJobFailureRecord
}

func newStubWikiBatchFailureRepo() *stubWikiBatchFailureRepo {
	return &stubWikiBatchFailureRepo{}
}

func (r *stubWikiBatchFailureRepo) Insert(_ context.Context, rec *types.WikiBatchJobFailureRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now()
	}
	cp := *rec
	cp.ID = int64(len(r.failures) + 1)
	r.failures = append(r.failures, &cp)
	rec.ID = cp.ID
	rec.OccurredAt = cp.OccurredAt
	return nil
}

func (r *stubWikiBatchFailureRepo) ListByJobID(
	_ context.Context, kbID, jobID, code string, page, pageSize int,
) ([]*types.WikiBatchJobFailureRecord, []types.WikiBatchFailureGroupCount, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	matched := make([]*types.WikiBatchJobFailureRecord, 0)
	for _, f := range r.failures {
		if f.KnowledgeBaseID != kbID || f.BatchJobID != jobID {
			continue
		}
		if code != "" && f.Code != code {
			continue
		}
		cp := *f
		matched = append(matched, &cp)
	}
	total := int64(len(matched))
	start := (page - 1) * pageSize
	if start >= len(matched) {
		return nil, nil, total, nil
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	out := make([]*types.WikiBatchJobFailureRecord, 0, end-start)
	out = append(out, matched[start:end]...)

	// Group counts over the full filtered set (matches production
	// semantics — tabs always reflect the full picture, not the page).
	groupMap := map[string]int{}
	for _, f := range matched {
		groupMap[f.Code]++
	}
	groups := make([]types.WikiBatchFailureGroupCount, 0, len(groupMap))
	for c, n := range groupMap {
		groups = append(groups, types.WikiBatchFailureGroupCount{Code: c, Count: n})
	}
	// Stable order: count desc, then code asc.
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].Code < groups[j].Code
	})
	return out, groups, total, nil
}

func (r *stubWikiBatchFailureRepo) byJob(kbID, jobID string) []*types.WikiBatchJobFailureRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*types.WikiBatchJobFailureRecord, 0)
	for _, f := range r.failures {
		if f.KnowledgeBaseID == kbID && f.BatchJobID == jobID {
			cp := *f
			out = append(out, &cp)
		}
	}
	return out
}

// TestObservability_HappyPath_PerSlugResult — a fully successful batch
// (no failures) lands with state=succeeded, Progress populated to
// {total=N, processed=N, succeeded=N, failed=0}, and the failures
// repo stays empty.
//
// Build #15.
func TestObservability_HappyPath_PerSlugResult(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 12; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(
		context.Background(), "kb1", makeSlugs(12), "folderX", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.State != types.WikiBatchJobStateSucceeded {
		t.Fatalf("state = %s, want succeeded", final.State)
	}
	if final.Result == nil {
		t.Fatalf("nil result blob")
	}
	var blob types.WikiBatchResult
	if err := json.Unmarshal(final.Result, &blob); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(blob.Succeeded) != 12 {
		t.Fatalf("succeeded = %d, want 12", len(blob.Succeeded))
	}
	if len(blob.Failed) != 0 {
		t.Fatalf("failed = %d, want 0", len(blob.Failed))
	}

	// Progress must reach terminal snapshot.
	if final.Progress == nil {
		t.Fatalf("nil progress")
	}
	var prog types.WikiBatchJobProgress
	if err := json.Unmarshal(final.Progress, &prog); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if prog.Total != 12 || prog.Processed != 12 || prog.Succeeded != 12 || prog.Failed != 0 {
		t.Fatalf("progress = %+v, want total=12 processed=12 succeeded=12 failed=0", prog)
	}

	if rows := failRepo.byJob("kb1", jobID); len(rows) != 0 {
		t.Fatalf("failures repo size = %d, want 0", len(rows))
	}
}

// TestObservability_PartialFailure_PerSlugAndRepoRow — when 3 of 12
// slugs are missing from the KB, the worker surfaces:
//
//   - result.Succeeded = the 9 found
//   - result.Failed = the 3 missing slugs with code=not_found
//   - the failure repo has 3 rows keyed by the same {kbID, jobID}
//   - WikiBatchJob.State = partial (succeeded + failed > 0)
//   - Progress.Failed = 3
//
// Build #15.
func TestObservability_PartialFailure_PerSlugAndRepoRow(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 9; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	// p9 / p10 / p11 intentionally missing → not_found failures.

	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	slugs := []string{"p0", "p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "p10", "p11"}
	result, err := pageSvc.BatchMovePagesRoute(
		context.Background(), "kb1", slugs, "folderX", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.State != types.WikiBatchJobStatePartial {
		t.Fatalf("state = %s, want partial", final.State)
	}
	var blob types.WikiBatchResult
	if err := json.Unmarshal(final.Result, &blob); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(blob.Succeeded) != 9 {
		t.Fatalf("succeeded = %d, want 9", len(blob.Succeeded))
	}
	if len(blob.Failed) != 3 {
		t.Fatalf("failed = %d, want 3", len(blob.Failed))
	}
	for _, f := range blob.Failed {
		if f.Code != "not_found" {
			t.Fatalf("failed code = %q, want not_found", f.Code)
		}
	}

	// Failures repo mirrors the per-slug failures with the right code.
	rows := failRepo.byJob("kb1", jobID)
	if len(rows) != 3 {
		t.Fatalf("failure repo rows = %d, want 3", len(rows))
	}
	seenSlugs := map[string]bool{}
	for _, row := range rows {
		if row.Code != "not_found" {
			t.Fatalf("row code = %q, want not_found", row.Code)
		}
		if row.BatchJobID != jobID {
			t.Fatalf("row job = %s, want %s", row.BatchJobID, jobID)
		}
		seenSlugs[row.Slug] = true
	}
	for _, want := range []string{"p9", "p10", "p11"} {
		if !seenSlugs[want] {
			t.Fatalf("missing failure row for %s", want)
		}
	}

	// Progress reflects 9 + 3.
	var prog types.WikiBatchJobProgress
	if err := json.Unmarshal(final.Progress, &prog); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if prog.Total != 12 || prog.Processed != 12 || prog.Succeeded != 9 || prog.Failed != 3 {
		t.Fatalf("progress = %+v, want 12/12/9/3", prog)
	}
}

// TestObservability_DeleteJob_ProgressPerSlug — the worker iterates
// per slug for delete too (D7 = A); after the batch finishes the
// result + progress + failures repo all reflect the per-slug
// execution.
//
// Build #15.
func TestObservability_DeleteJob_ProgressPerSlug(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 7; i++ {
		pageRepo.addPage(fmt.Sprintf("d%d", i), "published", nil, "")
	}

	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	slugs := []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6"}
	result, err := pageSvc.BatchDeletePagesRoute(
		context.Background(), "kb1", slugs, "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.State != types.WikiBatchJobStateSucceeded {
		t.Fatalf("state = %s, want succeeded", final.State)
	}
	var prog types.WikiBatchJobProgress
	if err := json.Unmarshal(final.Progress, &prog); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if prog.Total != 7 || prog.Processed != 7 || prog.Succeeded != 7 {
		t.Fatalf("progress = %+v, want 7/7/7/0", prog)
	}
	if rows := failRepo.byJob("kb1", jobID); len(rows) != 0 {
		t.Fatalf("unexpected failures: %d", len(rows))
	}
}

// TestObservability_ProgressThrottle_BelowBucketNoFlush — when a batch
// has fewer slugs than WikiBatchProgressThrottle (5), the worker still
// publishes a terminal snapshot via the "last-bucket" branch. Without
// that branch, a 3-slug batch would never publish a Progress row at
// all — verified by reading the final job row.
//
// Build #15 (D6).
func TestObservability_ProgressThrottle_SmallBatchFlushesAtEnd(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 3; i++ {
		pageRepo.addPage(fmt.Sprintf("s%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(
		context.Background(), "kb1", []string{"s0", "s1", "s2"}, "folderX", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.Progress == nil {
		t.Fatalf("nil progress on terminal — small batch never flushed")
	}
	var prog types.WikiBatchJobProgress
	if err := json.Unmarshal(final.Progress, &prog); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if prog.Total != 3 || prog.Processed != 3 || prog.Succeeded != 3 {
		t.Fatalf("progress = %+v, want 3/3/3/0", prog)
	}
}

// TestObservability_StatusJob_PerSlug — a status batch (draft /
// published / archived) goes through the worker per slug, and the
// progress + failure surfaces match the move/delete shape.
//
// Build #15.
func TestObservability_StatusJob_PerSlug(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 6; i++ {
		pageRepo.addPage(fmt.Sprintf("st%d", i), "draft", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	slugs := []string{"st0", "st1", "st2", "st3", "st4", "st5"}
	result, err := pageSvc.BatchUpdatePageStatusRoute(
		context.Background(), "kb1", slugs, "published", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.State != types.WikiBatchJobStateSucceeded {
		t.Fatalf("state = %s, want succeeded", final.State)
	}
	var prog types.WikiBatchJobProgress
	if err := json.Unmarshal(final.Progress, &prog); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if prog.Total != 6 || prog.Processed != 6 || prog.Succeeded != 6 {
		t.Fatalf("progress = %+v, want 6/6/6/0", prog)
	}
	if rows := failRepo.byJob("kb1", jobID); len(rows) != 0 {
		t.Fatalf("unexpected failures: %d", len(rows))
	}
}

// TestObservability_KBMismatch_FailureRowInserted — when one slug
// belongs to a different KB (cross-KB), the worker records a
// kb_mismatch failure row and the per-slug result.Failed gets the
// right classifier code.
//
// Build #15.
func TestObservability_KBMismatch_FailureRowInserted(t *testing.T) {
	pageRepo := newStubBatchRepo()
	// p0 lives in kb1 (target), p1 lives in kb2 → cross-KB.
	pageRepo.addPageInKB("kb1", "p0", "published", nil, "")
	pageRepo.addPageInKB("kb2", "p1", "published", nil, "")

	jobRepo := newStubWikiBatchJobRepo()
	failRepo := newStubWikiBatchFailureRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(
		context.Background(), "kb1", []string{"p0", "p1"}, "folderX", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	// Worker reports cross-KB as a per-slug failure (D2 from Build #12),
	// so the job still terminates in `partial` rather than hard-fail.
	if final.State != types.WikiBatchJobStatePartial {
		t.Fatalf("state = %s, want partial", final.State)
	}
	rows := failRepo.byJob("kb1", jobID)
	if len(rows) != 1 {
		t.Fatalf("failure repo rows = %d, want 1", len(rows))
	}
	if rows[0].Slug != "p1" {
		t.Fatalf("row slug = %q, want p1", rows[0].Slug)
	}
	if rows[0].Code != "kb_mismatch" {
		t.Fatalf("row code = %q, want kb_mismatch", rows[0].Code)
	}
	// And the row is anchored to the caller's KB, not the source KB —
	// the worker uses job.KnowledgeBaseID for the ledger.
	if rows[0].KnowledgeBaseID != "kb1" {
		t.Fatalf("row kb = %q, want kb1", rows[0].KnowledgeBaseID)
	}
}

// TestObservability_RepoInsertError_DoesNotAbortJob — when the failure
// repo Insert call returns an error, the worker logs + continues. The
// job still completes (partial) and the result.Failed still has the
// slug — observability is best-effort.
//
// Build #15.
func TestObservability_RepoInsertError_DoesNotAbortJob(t *testing.T) {
	pageRepo := newStubBatchRepo()
	// p0 exists, p1 missing → 1 success + 1 not_found failure.
	pageRepo.addPage("p0", "published", nil, "")

	jobRepo := newStubWikiBatchJobRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	failRepo := &flakyFailRepo{stub: newStubWikiBatchFailureRepo(), failNext: true}
	batchSvc := NewWikiBatchJobService(jobRepo, nil, failRepo, pageSvc, nil)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(
		context.Background(), "kb1", []string{"p0", "p1"}, "folderX", "user-1",
	)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobID := result.Job.ID

	final := waitForTerminal(t, jobRepo, jobID, 5*time.Second)
	if final.State != types.WikiBatchJobStatePartial {
		t.Fatalf("state = %s, want partial", final.State)
	}
	var blob types.WikiBatchResult
	if err := json.Unmarshal(final.Result, &blob); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(blob.Failed) != 1 || blob.Failed[0].Code != "not_found" {
		t.Fatalf("failed = %+v, want 1 not_found", blob.Failed)
	}
}

// flakyFailRepo wraps the in-memory stub and forces one Insert to fail.
type flakyFailRepo struct {
	stub     *stubWikiBatchFailureRepo
	failNext bool
}

func (r *flakyFailRepo) Insert(ctx context.Context, rec *types.WikiBatchJobFailureRecord) error {
	if r.failNext {
		r.failNext = false
		return fmt.Errorf("simulated insert failure")
	}
	return r.stub.Insert(ctx, rec)
}

func (r *flakyFailRepo) ListByJobID(
	ctx context.Context, kbID, jobID, code string, page, pageSize int,
) ([]*types.WikiBatchJobFailureRecord, []types.WikiBatchFailureGroupCount, int64, error) {
	return r.stub.ListByJobID(ctx, kbID, jobID, code, page, pageSize)
}

// waitForTerminal polls the job row until it reaches a terminal state
// or the deadline fires.
func waitForTerminal(
	t *testing.T, repo *stubWikiBatchJobRepo, jobID string, timeout time.Duration,
) *types.WikiBatchJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		job, err := repo.GetByID(context.Background(), jobID)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if job.State == types.WikiBatchJobStateSucceeded ||
			job.State == types.WikiBatchJobStatePartial ||
			job.State == types.WikiBatchJobStateFailed {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for terminal state, got %s", job.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Reference ErrWikiBatchJobNotFound so the import stays linked even
// when a future change drops the only test that used it.
var _ = repository.ErrWikiPageNotFound
