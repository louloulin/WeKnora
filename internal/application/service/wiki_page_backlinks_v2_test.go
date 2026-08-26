package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// The harness tests below exercise `wikiPageService.ListBacklinkGraph`
// (Build #20) using the same `stubBacklinksRepo` shape as Build #11's
// `wiki_page_backlinks_test.go` — only `GetBySlug` and `ListBySlugs`
// are exercised, since the v2 method reuses both.
//
// Each test prepares a `target` page (the one whose backlink graph we
// are inspecting) plus a `lites` map keyed by slug. When the test wants
// to assert behaviour on a row's `out_links` overlap (for Jaccard) or
// `in_links` walk (for indirect 2-hop), it adds an `OutLinks` or
// `InLinks` slice on the corresponding lite — both fields exist on
// `WikiPageLite` and the service reads them.

// liteWith is a small constructor that lets tests attach InLinks /
// OutLinks without spelling out the full struct every time.
func liteWith(slug, title, ptype, status string, updatedAt time.Time, inLinks, outLinks []string) *types.WikiPageLite {
	return &types.WikiPageLite{
		Slug:      slug,
		Title:     title,
		PageType:  ptype,
		Status:    status,
		UpdatedAt: updatedAt,
		InLinks:   types.StringArray(inLinks),
		OutLinks:  types.StringArray(outLinks),
	}
}

// TestListBacklinkGraph_EmptyPageReturnsEmptyShape verifies that a
// page with no in_links and no out_links produces the canonical empty
// payload (every section a non-nil empty slice, stats all zero).
func TestListBacklinkGraph_EmptyPageReturnsEmptyShape(t *testing.T) {
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{},
			// OutLinks zero-value is also empty.
		},
		lites: map[string]*types.WikiPageLite{},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Direct) != 0 || len(got.Indirect) != 0 || len(got.Related) != 0 || len(got.Broken) != 0 {
		t.Fatalf("expected all sections empty, got direct=%d indirect=%d related=%d broken=%d",
			len(got.Direct), len(got.Indirect), len(got.Related), len(got.Broken))
	}
	if got.Stats.DirectCount != 0 || got.Stats.IndirectCount != 0 ||
		got.Stats.RelatedCount != 0 || got.Stats.BrokenCount != 0 ||
		got.Stats.OutLinkCount != 0 {
		t.Fatalf("expected all stats zero, got %+v", got.Stats)
	}
}

// TestListBacklinkGraph_IndirectDedupAndVia verifies that two 1-hop
// sources both linking to the same 2-hop candidate produce a single
// indirect row tagged with one of the via slugs (dedup wins; the
// first-seen 1-hop is the recorded `via`).
func TestListBacklinkGraph_IndirectDedupAndVia(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"concept/a", "concept/b"},
		},
		lites: map[string]*types.WikiPageLite{
			"concept/a":   liteWith("concept/a", "A", "concept", "live", t0, []string{"summary/intro"}, nil),
			"concept/b":   liteWith("concept/b", "B", "concept", "live", t0, []string{"summary/intro", "concept/shared"}, nil),
			"concept/shared": liteWith("concept/shared", "Shared", "concept", "live", t0, nil, nil),
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Direct) != 2 {
		t.Fatalf("expected 2 direct, got %d", len(got.Direct))
	}
	if len(got.Indirect) != 1 {
		t.Fatalf("expected 1 indirect (deduped), got %d", len(got.Indirect))
	}
	if got.Indirect[0].Slug != "concept/shared" {
		t.Fatalf("expected indirect slug=concept/shared, got %s", got.Indirect[0].Slug)
	}
	if got.Indirect[0].Via == "" {
		t.Fatalf("expected indirect `via` to be set, got empty")
	}
	if got.Indirect[0].Via != "concept/a" && got.Indirect[0].Via != "concept/b" {
		t.Fatalf("expected via to be one of {concept/a, concept/b}, got %s", got.Indirect[0].Via)
	}
}

// TestListBacklinkGraph_JaccardBoundary verifies that two pages whose
// out_links overlap exactly half (Jaccard = 0.5) at threshold 0.5 are
// excluded, and pages with overlap strictly above the threshold are
// included.
func TestListBacklinkGraph_JaccardBoundary(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	// current.out_links = {a, b}
	// X.out_links = {a, b, c, d}    → intersect {a,b}=2, union {a,b,c,d}=4, jaccard=0.5 (excluded at threshold 0.5)
	// Y.out_links = {a, b, c}       → intersect {a,b}=2, union {a,b,c}=3, jaccard≈0.667 (included)
	// Z.out_links = {a, c, d, e}    → intersect {a}=1, union {a,b,c,d,e}=5, jaccard=0.2 (excluded)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:     "summary/intro",
			InLinks:  types.StringArray{},
			OutLinks: types.StringArray{"a", "b"},
		},
		lites: map[string]*types.WikiPageLite{
			"a": liteWith("a", "A", "entity", "live", t0, nil, nil),
			"b": liteWith("b", "B", "entity", "live", t0, nil, nil),
			"c": liteWith("c", "C", "entity", "live", t0, nil, nil),
			"d": liteWith("d", "D", "entity", "live", t0, nil, nil),
			"e": liteWith("e", "E", "entity", "live", t0, nil, nil),
			"x": liteWith("x", "X", "entity", "live", t0, nil, []string{"a", "b", "c", "d"}),
			"y": liteWith("y", "Y", "entity", "live", t0, nil, []string{"a", "b", "c"}),
			"z": liteWith("z", "Z", "entity", "live", t0, nil, []string{"a", "c", "d", "e"}),
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
		JaccardThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// X has jaccard = 2/4 = 0.5 → filtered (boundary exclusive).
	// Y has jaccard = 2/3 ≈ 0.667 → included.
	// Z has jaccard = 1/5 = 0.2 → filtered.
	if len(got.Related) != 1 {
		t.Fatalf("expected 1 related at threshold 0.5 (Y only), got %d", len(got.Related))
	}
	if got.Related[0].Slug != "y" {
		t.Fatalf("expected related[0]=y, got %s", got.Related[0].Slug)
	}
	if got.Related[0].Jaccard < 0.6 || got.Related[0].Jaccard > 0.7 {
		t.Fatalf("expected jaccard in [0.6, 0.7], got %f", got.Related[0].Jaccard)
	}
	if got.Stats.OutLinkCount != 2 {
		t.Fatalf("expected out_link_count=2, got %d", got.Stats.OutLinkCount)
	}
}

// TestListBacklinkGraph_BrokenDetection verifies that slugs in the
// current page's `out_links` that don't resolve to a live row appear
// in the broken section, ordered alphabetically.
func TestListBacklinkGraph_BrokenDetection(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:     "summary/intro",
			InLinks:  types.StringArray{},
			OutLinks: types.StringArray{"alive", "missing-one", "missing-two"},
		},
		lites: map[string]*types.WikiPageLite{
			"alive": liteWith("alive", "Alive", "entity", "live", t0, nil, nil),
			// missing-one + missing-two intentionally absent.
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Broken) != 2 {
		t.Fatalf("expected 2 broken slugs, got %d", len(got.Broken))
	}
	// Alphabetical: missing-one < missing-two.
	if got.Broken[0].TargetSlug != "missing-one" || got.Broken[1].TargetSlug != "missing-two" {
		t.Fatalf("expected alphabetical broken order, got %s then %s",
			got.Broken[0].TargetSlug, got.Broken[1].TargetSlug)
	}
	if got.Stats.BrokenCount != 2 {
		t.Fatalf("expected broken_count=2, got %d", got.Stats.BrokenCount)
	}
}

// TestListBacklinkGraph_SelfAndIndirectOrphanFiltered ensures that
// self-slug references in the target's in_links are excluded from
// direct, and that 1-hop pages pointing to orphan slugs produce no
// indirect row (the orphan 2-hop candidate is filtered).
func TestListBacklinkGraph_SelfAndIndirectOrphanFiltered(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"summary/intro", "concept/a"},
		},
		lites: map[string]*types.WikiPageLite{
			"concept/a": liteWith("concept/a", "A", "concept", "live", t0,
				[]string{"summary/intro", "concept/orphan"}, nil),
			// concept/orphan intentionally absent → 2-hop orphan.
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Direct) != 1 {
		t.Fatalf("expected 1 direct (self excluded), got %d", len(got.Direct))
	}
	if got.Direct[0].Slug != "concept/a" {
		t.Fatalf("expected direct[0]=concept/a, got %s", got.Direct[0].Slug)
	}
	if len(got.Indirect) != 0 {
		t.Fatalf("expected 0 indirect (orphan filtered), got %d", len(got.Indirect))
	}
}

// TestListBacklinkGraph_MaxIndirectAndMaxRelated verifies that the
// truncate fields take effect after the ordering stage — only the
// top-N rows survive.
func TestListBacklinkGraph_MaxIndirectAndMaxRelated(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	// 4 direct, each points to a distinct 2-hop candidate → 4 indirect.
	// current.out_links = {a, b, c, d, e} → 5 candidates for related.
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"d1", "d2", "d3", "d4"},
			OutLinks: types.StringArray{"a", "b", "c", "d", "e"},
		},
		lites: map[string]*types.WikiPageLite{
			"d1":   liteWith("d1", "D1", "concept", "live", t0, []string{"summary/intro", "i1"}, nil),
			"d2":   liteWith("d2", "D2", "concept", "live", t1, []string{"summary/intro", "i2"}, nil),
			"d3":   liteWith("d3", "D3", "concept", "live", t2, []string{"summary/intro", "i3"}, nil),
			"d4":   liteWith("d4", "D4", "concept", "live", t3, []string{"summary/intro", "i4"}, nil),
			"i1":   liteWith("i1", "I1", "concept", "live", t0, nil, nil),
			"i2":   liteWith("i2", "I2", "concept", "live", t1, nil, nil),
			"i3":   liteWith("i3", "I3", "concept", "live", t2, nil, nil),
			"i4":   liteWith("i4", "I4", "concept", "live", t3, nil, nil),
			// All five candidates have identical out_links to the
			// current target, so they all score jaccard = 1.0.
			"a": liteWith("a", "A", "entity", "live", t0, nil, []string{"a", "b", "c", "d", "e"}),
			"b": liteWith("b", "B", "entity", "live", t1, nil, []string{"a", "b", "c", "d", "e"}),
			"c": liteWith("c", "C", "entity", "live", t2, nil, []string{"a", "b", "c", "d", "e"}),
			"d": liteWith("d", "D", "entity", "live", t3, nil, []string{"a", "b", "c", "d", "e"}),
			"e": liteWith("e", "E", "entity", "live", t0, nil, []string{"a", "b", "c", "d", "e"}),
		},
	}
	svc := newBacklinksService(repo)

	// MaxIndirect = 2 → keep only the top 2 by updated_at desc.
	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
		MaxIndirect: 2,
		MaxRelated:  3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Indirect) != 2 {
		t.Fatalf("expected indirect truncated to 2, got %d", len(got.Indirect))
	}
	// updated_at desc → i4 (t3), i3 (t2).
	if got.Indirect[0].Slug != "i4" || got.Indirect[1].Slug != "i3" {
		t.Fatalf("expected indirect[0]=i4, indirect[1]=i3, got %s, %s",
			got.Indirect[0].Slug, got.Indirect[1].Slug)
	}
	if got.Stats.IndirectCount != 2 {
		t.Fatalf("expected indirect_count=2, got %d", got.Stats.IndirectCount)
	}
	if len(got.Related) != 3 {
		t.Fatalf("expected related truncated to 3, got %d", len(got.Related))
	}
	if got.Stats.RelatedCount != 3 {
		t.Fatalf("expected related_count=3, got %d", got.Stats.RelatedCount)
	}
}