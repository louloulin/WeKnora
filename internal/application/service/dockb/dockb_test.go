package dockb

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubDocKBSummaryRepo is an in-memory implementation for tests.
type stubDocKBSummaryRepo struct {
	rows  map[string]*types.DocKBSummary // key = tenant|kb|chunk
	seq   uint64
}

func newStubDocKBSummaryRepo() *stubDocKBSummaryRepo {
	return &stubDocKBSummaryRepo{rows: map[string]*types.DocKBSummary{}}
}

func stubKey(tenant, kb, chunk string) string { return tenant + "|" + kb + "|" + chunk }

func (s *stubDocKBSummaryRepo) Upsert(_ context.Context, sum *types.DocKBSummary) error {
	k := stubKey(sum.TenantID, sum.KnowledgeID, sum.ChunkID)
	if existing, ok := s.rows[k]; ok {
		existing.Summary = sum.Summary
		existing.Keyphrases = sum.Keyphrases
		existing.AutoTags = sum.AutoTags
		existing.ModelName = sum.ModelName
		existing.Confidence = sum.Confidence
		sum.ID = existing.ID
		return nil
	}
	s.seq++
	sum.ID = s.seq
	s.rows[k] = sum
	return nil
}

func (s *stubDocKBSummaryRepo) GetByChunk(_ context.Context, tenant, kb, chunk string) (*types.DocKBSummary, error) {
	if v, ok := s.rows[stubKey(tenant, kb, chunk)]; ok {
		return v, nil
	}
	return nil, nil
}

func (s *stubDocKBSummaryRepo) ListByKnowledge(_ context.Context, tenant, kb string) ([]*types.DocKBSummary, error) {
	out := []*types.DocKBSummary{}
	for _, v := range s.rows {
		if v.TenantID == tenant && v.KnowledgeID == kb {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *stubDocKBSummaryRepo) DeleteSummary(_ context.Context, tenantID string, id uint64) error {
	for k, v := range s.rows {
		if v.ID == id && v.TenantID == tenantID {
			delete(s.rows, k)
			return nil
		}
	}
	return errors.New("not found")
}

// --- tests ---

func TestSummariserService_SummariseChunk_HappyPath(t *testing.T) {
	repo := newStubDocKBSummaryRepo()
	svc := NewSummariserService(repo, NoopSummariser{})
	ctx := context.Background()

	got, err := svc.SummariseChunk(ctx, "t1", "kb1", "chunk1",
		"This is a sample knowledge chunk about machine learning and AI applications.", "test-model")
	if err != nil {
		t.Fatalf("summarise: %v", err)
	}
	if got.ID == 0 {
		t.Errorf("expected non-zero id")
	}
	if got.Summary == "" {
		t.Errorf("expected non-empty summary")
	}
	if len(got.Keyphrases) == 0 {
		t.Errorf("expected keyphrases, got none")
	}
}

func TestSummariserService_SummariseChunk_Idempotent(t *testing.T) {
	repo := newStubDocKBSummaryRepo()
	svc := NewSummariserService(repo, NoopSummariser{})
	ctx := context.Background()

	first, err := svc.SummariseChunk(ctx, "t1", "kb1", "chunk1", "First text about database indexing.", "test-model")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.SummariseChunk(ctx, "t1", "kb1", "chunk1", "Second text replaces first.", "test-model")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("idempotency violated: first.ID=%d second.ID=%d", first.ID, second.ID)
	}
}

func TestSummariserService_SummariseChunk_RejectsEmptyText(t *testing.T) {
	repo := newStubDocKBSummaryRepo()
	svc := NewSummariserService(repo, NoopSummariser{})
	_, err := svc.SummariseChunk(context.Background(), "t1", "kb1", "chunk1", "   ", "m")
	if !errors.Is(err, ErrEmptyChunkText) {
		t.Fatalf("expected ErrEmptyChunkText, got %v", err)
	}
}

func TestSummariserService_ListByKnowledge(t *testing.T) {
	repo := newStubDocKBSummaryRepo()
	svc := NewSummariserService(repo, NoopSummariser{})
	ctx := context.Background()

	if _, err := svc.SummariseChunk(ctx, "t1", "kb1", "c1", "alpha beta gamma delta.", "m"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SummariseChunk(ctx, "t1", "kb1", "c2", "epsilon zeta eta.", "m"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SummariseChunk(ctx, "t1", "kb2", "c1", "other knowledge.", "m"); err != nil {
		t.Fatal(err)
	}

	rows, err := svc.ListByKnowledge(ctx, "t1", "kb1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows for kb1, got %d", len(rows))
	}
}

func TestSummariserService_Delete(t *testing.T) {
	repo := newStubDocKBSummaryRepo()
	svc := NewSummariserService(repo, NoopSummariser{})
	ctx := context.Background()

	got, err := svc.SummariseChunk(ctx, "t1", "kb1", "c1", "text to be deleted later.", "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "t1", got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, _ := svc.ListByKnowledge(ctx, "t1", "kb1")
	if len(rows) != 0 {
		t.Errorf("expected empty after delete, got %d", len(rows))
	}
}

func TestNoopSummariser_ProducesStableOutput(t *testing.T) {
	noo := NoopSummariser{}
	s1, kp1, tags1, _ := noo.Summarize(context.Background(), "machine learning algorithms")
	s2, kp2, tags2, _ := noo.Summarize(context.Background(), "machine learning algorithms")
	if s1 != s2 || len(kp1) != len(kp2) || len(tags1) != len(tags2) {
		t.Errorf("NoopSummariser not deterministic: %q vs %q", s1, s2)
	}
	if s1 == "" {
		t.Error("expected non-empty summary")
	}
}

// interface satisfaction check
var _ interfaces.DocKBSummaryRepository = (*stubDocKBSummaryRepo)(nil)
