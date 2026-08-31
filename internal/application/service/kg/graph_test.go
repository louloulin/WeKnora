package kg

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestKGGraphService_BuildGraphBySupertag(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()

	_ = repo.CreateSupertag(ctx, &types.KGSupertag{ID: "st-person", TenantID: 1, KBID: "kb-1", Name: "Person"})
	stid := "st-person"
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		_ = repo.CreateEntity(ctx, &types.KGEntity{
			ID: "e-" + name, TenantID: 1, KBID: "kb-1",
			SupertagID: &stid, Name: name, Occurrence: 5,
		})
	}
	_ = repo.CreateRelation(ctx, &types.KGEntityRelation{
		ID: "r-1",
		SrcEntityID: "e-Alice", DstEntityID: "e-Bob",
		Relation: "knows", Confidence: 0.9,
	})
	_ = repo.CreateRelation(ctx, &types.KGEntityRelation{
		ID: "r-2",
		SrcEntityID: "e-Alice", DstEntityID: "e-Carol",
		Relation: "manages", Confidence: 0.7,
	})

	svc := NewKGGraphService(repo)
	g, err := svc.BuildGraph(ctx, 1, "kb-1", "st-person", 100)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if g.KBID != "kb-1" || g.SupertagID != "st-person" {
		t.Fatalf("meta wrong: %+v", g)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(g.Edges))
	}
	for _, n := range g.Nodes {
		if n.KBID != "kb-1" {
			t.Fatalf("node kb wrong: %+v", n)
		}
		if n.NodeSize <= 0 {
			t.Fatalf("node size must be positive: %+v", n)
		}
		if n.Color == "" || n.Color[0] != '#' {
			t.Fatalf("color must be hex: %s", n.Color)
		}
	}
}

func TestKGGraphService_BuildGraphAllEntitiesInKB(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	_ = repo.CreateSupertag(ctx, &types.KGSupertag{ID: "st-a", TenantID: 1, KBID: "kb-1", Name: "A"})
	_ = repo.CreateSupertag(ctx, &types.KGSupertag{ID: "st-b", TenantID: 1, KBID: "kb-1", Name: "B"})
	sta := "st-a"
	stb := "st-b"
	_ = repo.CreateEntity(ctx, &types.KGEntity{ID: "e1", TenantID: 1, KBID: "kb-1", SupertagID: &sta, Name: "A1"})
	_ = repo.CreateEntity(ctx, &types.KGEntity{ID: "e2", TenantID: 1, KBID: "kb-1", SupertagID: &stb, Name: "B1"})

	svc := NewKGGraphService(repo)
	g, err := svc.BuildGraph(ctx, 1, "kb-1", "", 100)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
}

func TestKGGraphService_DropsEdgesToMissingNodes(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	_ = repo.CreateSupertag(ctx, &types.KGSupertag{ID: "st-x", TenantID: 1, KBID: "kb-1", Name: "X"})
	stx := "st-x"
	_ = repo.CreateEntity(ctx, &types.KGEntity{ID: "e-a", TenantID: 1, KBID: "kb-1", SupertagID: &stx, Name: "A"})
	// Edge points to a non-existent entity.
	_ = repo.CreateRelation(ctx, &types.KGEntityRelation{
		ID: "r-orphan",
		SrcEntityID: "e-a", DstEntityID: "e-ghost",
		Relation: "refers_to", Confidence: 0.5,
	})
	svc := NewKGGraphService(repo)
	g, err := svc.BuildGraph(ctx, 1, "kb-1", "st-x", 100)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("orphan edge should be dropped, got %d", len(g.Edges))
	}
}

func TestKGGraphService_DedupesEdgesByID(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	_ = repo.CreateSupertag(ctx, &types.KGSupertag{ID: "st-x", TenantID: 1, KBID: "kb-1", Name: "X"})
	stx := "st-x"
	_ = repo.CreateEntity(ctx, &types.KGEntity{ID: "e-a", TenantID: 1, KBID: "kb-1", SupertagID: &stx, Name: "A"})
	_ = repo.CreateEntity(ctx, &types.KGEntity{ID: "e-b", TenantID: 1, KBID: "kb-1", SupertagID: &stx, Name: "B"})
	_ = repo.CreateRelation(ctx, &types.KGEntityRelation{
		ID: "r-1",
		SrcEntityID: "e-a", DstEntityID: "e-b",
		Relation: "knows", Confidence: 0.9,
	})
	// Create the same relation again with same ID — should overwrite.
	_ = repo.CreateRelation(ctx, &types.KGEntityRelation{
		ID: "r-1",
		SrcEntityID: "e-a", DstEntityID: "e-b",
		Relation: "knows", Confidence: 0.4,
	})
	svc := NewKGGraphService(repo)
	g, err := svc.BuildGraph(ctx, 1, "kb-1", "st-x", 100)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge after dedup, got %d", len(g.Edges))
	}
}

func TestKGGraphService_DefaultLimitApplied(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	svc := NewKGGraphService(repo)
	g, err := svc.BuildGraph(ctx, 1, "kb-1", "missing", 0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if g == nil {
		t.Fatal("graph must not be nil")
	}
}

func TestKGGraphService_GeneratedAtFrozen(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	svc := NewKGGraphService(repo)
	frozen := time.Unix(1234567890, 0).UTC()
	svc.SetNow(func() time.Time { return frozen })
	g, _ := svc.BuildGraph(ctx, 1, "kb-1", "", 100)
	if g.GeneratedAt != 1234567890 {
		t.Fatalf("generated_at = %d, want 1234567890", g.GeneratedAt)
	}
}

func TestEntitySize(t *testing.T) {
	if entitySize(0) != 4 {
		t.Fatal("size 0 must be 4")
	}
	if entitySize(1) <= 4 {
		t.Fatal("size 1 must exceed 4")
	}
	if entitySize(10000) > 25 {
		t.Fatalf("size must be capped at 24, got %v", entitySize(10000))
	}
}

func TestSupertagColor_DeterministicAndDistinct(t *testing.T) {
	if supertagColor("") != "#888888" {
		t.Fatal("empty supertag must be grey")
	}
	c1 := supertagColor("st-a")
	c2 := supertagColor("st-a")
	if c1 != c2 {
		t.Fatal("same id must produce same colour")
	}
	c3 := supertagColor("st-b")
	if c1 == c3 {
		t.Fatal("different ids should usually produce different colours")
	}
	if c1[0] != '#' || len(c1) != 7 {
		t.Fatalf("invalid hex: %s", c1)
	}
}
