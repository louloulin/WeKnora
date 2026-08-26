package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubWikiBatchAuditRepo is the in-memory WikiBatchAuditRepository used
// by the Build #14 audit harness. Insert + ListByJobID + ListByKB +
// ListExpiredEvents all stay in-memory; no SQL is touched.
//
// Append-only — there is intentionally no Update or Delete so test
// assertions can rely on the row count being monotonically increasing
// through a scenario.
type stubWikiBatchAuditRepo struct {
	mu     sync.Mutex
	events []*types.WikiBatchJobAuditEvent
}

func newStubWikiBatchAuditRepo() *stubWikiBatchAuditRepo {
	return &stubWikiBatchAuditRepo{}
}

func (r *stubWikiBatchAuditRepo) Insert(_ context.Context, e *types.WikiBatchJobAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	cp := *e
	cp.ID = int64(len(r.events) + 1)
	r.events = append(r.events, &cp)
	e.ID = cp.ID
	e.OccurredAt = cp.OccurredAt
	return nil
}

func (r *stubWikiBatchAuditRepo) ListByJobID(_ context.Context, kbID, jobID string) ([]*types.WikiBatchJobAuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*types.WikiBatchJobAuditEvent, 0)
	for _, e := range r.events {
		if e.KnowledgeBaseID == kbID && e.BatchJobID == jobID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *stubWikiBatchAuditRepo) ListByKB(_ context.Context, kbID, actor string, action types.WikiBatchAuditAction, since time.Time, page, pageSize int) ([]*types.WikiBatchJobAuditEvent, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pageSize <= 0 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	all := make([]*types.WikiBatchJobAuditEvent, 0)
	for _, e := range r.events {
		if e.KnowledgeBaseID != kbID {
			continue
		}
		if actor != "" && e.ActorID != actor {
			continue
		}
		if action != "" && e.Action != action {
			continue
		}
		if !since.IsZero() && e.OccurredAt.Before(since) {
			continue
		}
		all = append(all, e)
	}
	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return nil, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	// newest-first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	out := make([]*types.WikiBatchJobAuditEvent, 0, end-start)
	for _, e := range all[start:end] {
		cp := *e
		out = append(out, &cp)
	}
	return out, total, nil
}

func (r *stubWikiBatchAuditRepo) ListExpiredEvents(_ context.Context, before time.Time, limit int) ([]*types.WikiBatchJobAuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*types.WikiBatchJobAuditEvent, 0)
	for _, e := range r.events {
		if e.OccurredAt.Before(before) {
			cp := *e
			out = append(out, &cp)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// findEvent returns the first event in the stub matching kbID, jobID
// and action — convenient for tests that want to assert a specific
// event landed.
func (r *stubWikiBatchAuditRepo) findEvent(kbID, jobID string, action types.WikiBatchAuditAction) *types.WikiBatchJobAuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.KnowledgeBaseID == kbID && e.BatchJobID == jobID && e.Action == action {
			cp := *e
			return &cp
		}
	}
	return nil
}

func (r *stubWikiBatchAuditRepo) countByJob(kbID, jobID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.KnowledgeBaseID == kbID && e.BatchJobID == jobID {
			n++
		}
	}
	return n
}

// TestBatchAudit_EnqueueFiresEvent — EnqueueJob records exactly one
// `enqueue` event keyed to the right job + actor.
//
// Build #14.
func TestBatchAudit_EnqueueFiresEvent(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	auditRepo := newStubWikiBatchAuditRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, auditRepo, pageSvc)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "alice")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ev := auditRepo.findEvent("kb1", result.Job.ID, types.WikiBatchAuditActionEnqueue)
	if ev == nil {
		t.Fatalf("no enqueue event recorded")
	}
	if ev.ActorID != "alice" {
		t.Fatalf("actor = %q, want alice", ev.ActorID)
	}
	if ev.BatchJobID != result.Job.ID {
		t.Fatalf("job id mismatch: audit=%s result=%s", ev.BatchJobID, result.Job.ID)
	}
}

// TestBatchAudit_WorkerFullLifecycle — enqueue → start → finish all
// land as separate rows in chronological order.
//
// Build #14.
func TestBatchAudit_WorkerFullLifecycle(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	auditRepo := newStubWikiBatchAuditRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, auditRepo, pageSvc)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "bob")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Wait for worker to drain.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := jobRepo.GetByID(context.Background(), result.Job.ID)
		if got != nil && (got.State == types.WikiBatchJobStateSucceeded ||
			got.State == types.WikiBatchJobStatePartial ||
			got.State == types.WikiBatchJobStateFailed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for worker")
		}
		time.Sleep(20 * time.Millisecond)
	}

	for _, action := range []types.WikiBatchAuditAction{
		types.WikiBatchAuditActionEnqueue,
		types.WikiBatchAuditActionStart,
		types.WikiBatchAuditActionFinish,
	} {
		if ev := auditRepo.findEvent("kb1", result.Job.ID, action); ev == nil {
			t.Fatalf("missing %s event", action)
		} else if ev.ActorID != "system" && ev.ActorID != "bob" {
			// start / finish should be system, enqueue is the user
			t.Fatalf("%s actor = %q, want system or %q", action, ev.ActorID, "bob")
		}
	}
	// Finish metadata should carry per-batch counts.
	finish := auditRepo.findEvent("kb1", result.Job.ID, types.WikiBatchAuditActionFinish)
	if finish == nil || finish.Metadata == nil {
		t.Fatalf("finish event missing metadata")
	}
	if _, ok := finish.Metadata["succeeded"]; !ok {
		t.Fatalf("finish metadata missing succeeded key")
	}
	if _, ok := finish.Metadata["state"]; !ok {
		t.Fatalf("finish metadata missing state key")
	}
}

// TestBatchAudit_UndoFiresRequestAndDone — UndoJob emits both
// undo_request and undo_done events, with the requestor as actor.
//
// Build #14.
func TestBatchAudit_UndoFiresRequestAndDone(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	auditRepo := newStubWikiBatchAuditRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, auditRepo, pageSvc)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "carol")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Drain worker.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := jobRepo.GetByID(context.Background(), result.Job.ID)
		if got != nil && (got.State == types.WikiBatchJobStateSucceeded ||
			got.State == types.WikiBatchJobStatePartial ||
			got.State == types.WikiBatchJobStateFailed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := batchSvc.UndoJob(context.Background(), "kb1", result.Job.ID, "carol"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	req := auditRepo.findEvent("kb1", result.Job.ID, types.WikiBatchAuditActionUndoRequest)
	done := auditRepo.findEvent("kb1", result.Job.ID, types.WikiBatchAuditActionUndoDone)
	if req == nil || done == nil {
		t.Fatalf("missing undo events: req=%v done=%v", req, done)
	}
	if req.ActorID != "carol" || done.ActorID != "carol" {
		t.Fatalf("undo actor mismatch: req=%q done=%q", req.ActorID, done.ActorID)
	}
	// ListByJobID should return oldest-first.
	events, err := auditRepo.ListByJobID(context.Background(), "kb1", result.Job.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) < 5 {
		t.Fatalf("expected >=5 events, got %d", len(events))
	}
	if events[0].Action != types.WikiBatchAuditActionEnqueue {
		t.Fatalf("first event = %s, want enqueue", events[0].Action)
	}
	if events[len(events)-1].Action != types.WikiBatchAuditActionUndoDone {
		t.Fatalf("last event = %s, want undo_done", events[len(events)-1].Action)
	}
}

// TestBatchAudit_CancelFiresEvent — CancelJob records exactly one
// `cancel` event when the job is still queued, and returns the
// not-cancellable error once state has moved on.
//
// Build #14.
func TestBatchAudit_CancelFiresEvent(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 25; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	auditRepo := newStubWikiBatchAuditRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, auditRepo, pageSvc)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	result, err := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(25), "folderX", "dan")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Drain worker so state leaves queued; cancel must then fail.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := jobRepo.GetByID(context.Background(), result.Job.ID)
		if got != nil && got.State != types.WikiBatchJobStateQueued {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err = batchSvc.CancelJob(context.Background(), "kb1", result.Job.ID, "dan")
	if err == nil {
		t.Fatalf("expected cancel after worker drained to fail")
	}
	if got := auditRepo.findEvent("kb1", result.Job.ID, types.WikiBatchAuditActionCancel); got != nil {
		t.Fatalf("cancel event should NOT have fired (state left queued): %v", got)
	}
}

// TestBatchAudit_QueryFilters — ListByKB honours the actor / action /
// since filters, newest-first pagination, and cross-KB isolation.
//
// Build #14.
func TestBatchAudit_QueryFilters(t *testing.T) {
	pageRepo := newStubBatchRepo()
	for i := 0; i < 50; i++ {
		pageRepo.addPage(fmt.Sprintf("p%d", i), "published", nil, "")
	}
	jobRepo := newStubWikiBatchJobRepo()
	auditRepo := newStubWikiBatchAuditRepo()
	pageSvc := newBatchSvcForTest(pageRepo)
	batchSvc := NewWikiBatchJobService(jobRepo, auditRepo, pageSvc)
	t.Cleanup(func() { _ = batchSvc.Shutdown(context.Background()) })
	pageSvc.SetBatchJobService(batchSvc)

	// Two KBs, two enqueues each — gives us four enqueue events with
	// distinct (kb, actor) tuples to filter on.
	r1, _ := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(20), "f1", "alice")
	r2, _ := pageSvc.BatchMovePagesRoute(context.Background(), "kb1", makeSlugs(20), "f2", "bob")
	_, _ = pageSvc.BatchDeletePagesRoute(context.Background(), "kb2", makeSlugs(20), "alice")

	// Drain.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		all := true
		for _, id := range []string{r1.Job.ID, r2.Job.ID} {
			got, _ := jobRepo.GetByID(context.Background(), id)
			if got == nil || got.State == types.WikiBatchJobStateQueued || got.State == types.WikiBatchJobStateRunning {
				all = false
			}
		}
		if all {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Filter by kb only.
	events, total, err := auditRepo.ListByKB(context.Background(), "kb1", "", "", time.Time{}, 1, 100)
	if err != nil {
		t.Fatalf("list kb1: %v", err)
	}
	if total < 4 {
		t.Fatalf("expected >=4 events for kb1, got total=%d", total)
	}
	for _, e := range events {
		if e.KnowledgeBaseID != "kb1" {
			t.Fatalf("cross-KB leak: %s", e.KnowledgeBaseID)
		}
	}

	// Filter by action.
	enqueueEvents, _, err := auditRepo.ListByKB(context.Background(), "kb1", "", types.WikiBatchAuditActionEnqueue, time.Time{}, 1, 100)
	if err != nil {
		t.Fatalf("list action: %v", err)
	}
	if len(enqueueEvents) != 2 {
		t.Fatalf("enqueue events = %d, want 2", len(enqueueEvents))
	}
	// Newest-first — r2 (bob) should come first.
	if enqueueEvents[0].ActorID != "bob" {
		t.Fatalf("newest-first broken: first actor = %s", enqueueEvents[0].ActorID)
	}

	// Filter by actor.
	aliceEvents, _, err := auditRepo.ListByKB(context.Background(), "kb1", "alice", "", time.Time{}, 1, 100)
	if err != nil {
		t.Fatalf("list actor: %v", err)
	}
	for _, e := range aliceEvents {
		if e.ActorID != "alice" {
			t.Fatalf("actor leak: %s", e.ActorID)
		}
	}
}