//go:build verification

package verification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeFetcher struct {
	pages map[string]*PageSummary
}

func (f *fakeFetcher) GetPage(_ context.Context, kbID, slug string) (*PageSummary, error) {
	if p, ok := f.pages[slug]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeFetcher) ListSlugs(_ context.Context, _ string) ([]string, error) {
	out := make([]string, 0, len(f.pages))
	for k := range f.pages {
		out = append(out, k)
	}
	return out, nil
}
func (f *fakeFetcher) ListBySlugs(_ context.Context, _ string, slugs []string) (map[string]*PageSummary, error) {
	out := map[string]*PageSummary{}
	for _, s := range slugs {
		if p, ok := f.pages[s]; ok {
			out[s] = p
		}
	}
	return out, nil
}

func TestRunForPage_Fresh(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{pages: map[string]*PageSummary{
		"a": {
			Slug:      "a",
			PageID:    "id-a",
			UpdatedAt: now.Add(-30 * 24 * time.Hour),
			Status:    "published",
			OutLinks:  []string{},
			Content:   "hello world",
		},
	}}
	svc := NewService(f)
	rep, err := svc.RunForPage(context.Background(), "kb1", "a")
	if err != nil {
		t.Fatalf("RunForPage: %v", err)
	}
	if rep.Status != types.VerificationStatusOK {
		t.Fatalf("expected OK, got %s", rep.Status)
	}
	if rep.TrustScore != 1.0 {
		t.Fatalf("expected trust 1.0, got %.2f", rep.TrustScore)
	}
}

func TestRunForPage_Stale(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{pages: map[string]*PageSummary{
		"a": {
			Slug:      "a",
			PageID:    "id-a",
			UpdatedAt: now.Add(-400 * 24 * time.Hour),
			Status:    "published",
			Content:   "stale page",
		},
	}}
	svc := NewService(f)
	rep, err := svc.RunForPage(context.Background(), "kb1", "a")
	if err != nil {
		t.Fatalf("RunForPage: %v", err)
	}
	if rep.Status != types.VerificationStatusBad {
		t.Fatalf("expected Bad, got %s", rep.Status)
	}
}

func TestRunForPage_Missing(t *testing.T) {
	f := &fakeFetcher{pages: map[string]*PageSummary{}}
	svc := NewService(f)
	rep, err := svc.RunForPage(context.Background(), "kb1", "nope")
	if err != nil {
		t.Fatalf("RunForPage: %v", err)
	}
	if rep.Status != types.VerificationStatusMissing {
		t.Fatalf("expected Missing, got %s", rep.Status)
	}
}

func TestRunForKB_Summary(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{pages: map[string]*PageSummary{
		"a": {Slug: "a", UpdatedAt: now.Add(-30 * 24 * time.Hour)},
		"b": {Slug: "b", UpdatedAt: now.Add(-400 * 24 * time.Hour)},
	}}
	svc := NewService(f)
	_, sum, err := svc.RunForKB(context.Background(), "kb1", 10)
	if err != nil {
		t.Fatalf("RunForKB: %v", err)
	}
	if sum.Total != 2 {
		t.Fatalf("expected 2 reports, got %d", sum.Total)
	}
	if sum.OK+sum.Warning+sum.Bad != 2 {
		t.Fatalf("counts inconsistent: %+v", sum)
	}
}

func TestTrigramOverlap(t *testing.T) {
	if trigramOverlap("hello world", "hello world") < 0.99 {
		t.Fatal("identical strings should be ~1.0")
	}
	if trigramOverlap("alpha beta gamma", "completely different") > 0.05 {
		t.Fatal("different strings should be ~0")
	}
}

func TestService_NilFetcher(t *testing.T) {
	svc := &Service{}
	_, err := svc.RunForPage(context.Background(), "k", "s")
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-fetcher error, got %v", err)
	}
	_ = errors.New
}
