package mindmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeRepo is an in-memory MindMapRepository used to drive the service tests.
type fakeRepo struct {
	maps   map[string]*types.MindMap
	nodes  map[string]*types.MindMapNode
	fail   bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		maps:  map[string]*types.MindMap{},
		nodes: map[string]*types.MindMapNode{},
	}
}

func (r *fakeRepo) Create(ctx context.Context, m *types.MindMap) error {
	if r.fail {
		return errors.New("create failed")
	}
	r.maps[m.ID] = m
	return nil
}

func (r *fakeRepo) Get(ctx context.Context, tenantID uint64, id string) (*types.MindMap, error) {
	m, ok := r.maps[id]
	if !ok || m.TenantID != tenantID {
		return nil, nil
	}
	return m, nil
}

func (r *fakeRepo) Update(ctx context.Context, tenantID uint64, id string, patch types.UpdateMindMapRequest) (*types.MindMap, error) {
	m, ok := r.maps[id]
	if !ok || m.TenantID != tenantID {
		return nil, nil
	}
	if patch.Title != nil {
		m.Title = *patch.Title
	}
	if patch.Layout != nil {
		m.Layout = *patch.Layout
	}
	if patch.Theme != nil {
		m.Theme = *patch.Theme
	}
	if patch.Visibility != nil {
		m.Visibility = *patch.Visibility
	}
	return m, nil
}

func (r *fakeRepo) Delete(ctx context.Context, tenantID uint64, id string) error {
	m, ok := r.maps[id]
	if !ok || m.TenantID != tenantID {
		return types.ErrMindMapInvalid("not found")
	}
	delete(r.maps, id)
	for nid, n := range r.nodes {
		if n.MapID == id {
			delete(r.nodes, nid)
		}
	}
	return nil
}

func (r *fakeRepo) List(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) ([]*types.MindMap, error) {
	out := []*types.MindMap{}
	for _, m := range r.maps {
		if m.TenantID != tenantID {
			continue
		}
		if filter.KBID != "" && m.KBID != filter.KBID {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *fakeRepo) Count(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) (int64, error) {
	out, err := r.List(ctx, tenantID, filter)
	return int64(len(out)), err
}

func (r *fakeRepo) CreateNode(ctx context.Context, n *types.MindMapNode) error {
	if r.fail {
		return errors.New("node create failed")
	}
	r.nodes[n.ID] = n
	return nil
}

func (r *fakeRepo) GetNode(ctx context.Context, tenantID uint64, mapID, nodeID string) (*types.MindMapNode, error) {
	n, ok := r.nodes[nodeID]
	if !ok || n.TenantID != tenantID || n.MapID != mapID {
		return nil, nil
	}
	return n, nil
}

func (r *fakeRepo) UpdateNode(ctx context.Context, tenantID uint64, mapID, nodeID string, patch types.UpdateMindMapNodeRequest) (*types.MindMapNode, error) {
	n, ok := r.nodes[nodeID]
	if !ok || n.TenantID != tenantID || n.MapID != mapID {
		return nil, nil
	}
	if patch.Label != nil {
		n.Label = *patch.Label
	}
	if patch.X != nil {
		n.X = *patch.X
	}
	if patch.Y != nil {
		n.Y = *patch.Y
	}
	if patch.Color != nil {
		n.Color = *patch.Color
	}
	return n, nil
}

func (r *fakeRepo) DeleteNode(ctx context.Context, tenantID uint64, mapID, nodeID string) error {
	n, ok := r.nodes[nodeID]
	if !ok || n.TenantID != tenantID || n.MapID != mapID {
		return types.ErrMindMapInvalid("node not found")
	}
	delete(r.nodes, nodeID)
	return nil
}

func (r *fakeRepo) ListNodesByMap(ctx context.Context, tenantID uint64, mapID string) ([]*types.MindMapNode, error) {
	out := []*types.MindMapNode{}
	for _, n := range r.nodes {
		if n.TenantID == tenantID && n.MapID == mapID {
			out = append(out, n)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteByKB(ctx context.Context, tenantID uint64, kbID string) (int64, error) {
	var total int64
	for id, m := range r.maps {
		if m.TenantID == tenantID && m.KBID == kbID {
			delete(r.maps, id)
			total++
			for nid, n := range r.nodes {
				if n.MapID == id {
					delete(r.nodes, nid)
				}
			}
		}
	}
	return total, nil
}

func TestCreateMindMapWithRoot(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, err := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{
		Title:      "Weekly plan",
		Layout:     types.MindMapLayoutTree,
		Theme:      "feishu",
		RootLabel:  "Plan",
		RootBody:   "kickoff body",
		RootColor:  "#58a6ff",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Title != "Weekly plan" || m.OwnerUserID != 42 {
		t.Errorf("unexpected map: %+v", m)
	}
	nodes, _ := svc.ListNodes(context.Background(), 1, m.ID)
	if len(nodes) != 1 {
		t.Fatalf("want 1 root node, got %d", len(nodes))
	}
	if nodes[0].Label != "Plan" || nodes[0].Color != "#58a6ff" {
		t.Errorf("root node mismatch: %+v", nodes[0])
	}
}

func TestCreateMindMapRejectsBadLayout(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	_, err := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{
		Title:  "x",
		Layout: "bogus",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !types.IsMindMapInvalid(err) {
		t.Errorf("expected ErrMindMapInvalid, got %v", err)
	}
}

func TestCreateNodeRejectsMissingParent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, err := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	missing := "00000000-0000-0000-0000-000000000000"
	_, err = svc.CreateNode(context.Background(), 1, 42, m.ID, types.CreateMindMapNodeRequest{
		NodeType: types.MindMapNodeText,
		Label:    "child",
		ParentID: &missing,
	})
	if err == nil {
		t.Fatal("expected parent-not-found error")
	}
}

func TestAutoLayoutTree(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, err := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{
		Title:    "x",
		RootLabel: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 3 child nodes.
	for i := 0; i < 3; i++ {
		_, err := svc.CreateNode(context.Background(), 1, 42, m.ID, types.CreateMindMapNodeRequest{
			NodeType: types.MindMapNodeText,
			Label:    "child",
			ParentID: &m.RootNodeID,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	updated, err := svc.AutoLayout(context.Background(), 1, 42, m.ID, types.AutoLayoutRequest{
		Layout:  types.MindMapLayoutTree,
		Spacing: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 4 { // root + 3 children
		t.Fatalf("want 4 nodes updated, got %d", len(updated))
	}
	// Root should be at (0, 0); children should be at depth=1.
	if updated[0].X != 0 || updated[0].Y != 0 {
		t.Errorf("root position: %v %v", updated[0].X, updated[0].Y)
	}
	for _, n := range updated[1:] {
		if n.X <= 0 {
			t.Errorf("child X should be > 0, got %v", n.X)
		}
	}
}

func TestExportMarkdown(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, err := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{
		Title:     "Q3 Plan",
		RootLabel: "Q3 Goals",
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID := m.RootNodeID
	_, err = svc.CreateNode(context.Background(), 1, 42, m.ID, types.CreateMindMapNodeRequest{
		NodeType: types.MindMapNodeText,
		Label:    "Engineering",
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	md, err := svc.ExportMarkdown(context.Background(), 1, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "# Q3 Plan") {
		t.Errorf("missing top heading: %s", md)
	}
	if !strings.Contains(md, "## Q3 Goals") {
		t.Errorf("missing root heading: %s", md)
	}
	if !strings.Contains(md, "### Engineering") {
		t.Errorf("missing child heading: %s", md)
	}
}

func TestExportOPML(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, _ := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{
		Title: "Outliner",
	})
	opml, err := svc.ExportOPML(context.Background(), 1, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(opml, "<?xml") {
		t.Errorf("missing XML prolog: %s", opml)
	}
	if !strings.Contains(opml, "<opml version=\"2.0\">") {
		t.Errorf("missing OPML root: %s", opml)
	}
	if !strings.Contains(opml, "Outliner") {
		t.Errorf("missing title: %s", opml)
	}
}

func TestExportXMind(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, _ := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{Title: "XM"})
	js, err := svc.ExportXMind(context.Background(), 1, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"title":"XM"`) {
		t.Errorf("missing title in xmind json: %s", js)
	}
}

func TestDeleteMindMapAlsoRemovesNodes(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, _ := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{Title: "x"})
	parentID := m.RootNodeID
	_, _ = svc.CreateNode(context.Background(), 1, 42, m.ID, types.CreateMindMapNodeRequest{
		NodeType: types.MindMapNodeText, Label: "c", ParentID: &parentID,
	})
	if err := svc.DeleteMindMap(context.Background(), 1, 42, m.ID); err != nil {
		t.Fatal(err)
	}
	nodes, _ := svc.ListNodes(context.Background(), 1, m.ID)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes after delete, got %d", len(nodes))
	}
}

func TestDeleteRootNodeRefused(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	m, _ := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{Title: "x"})
	err := svc.DeleteNode(context.Background(), 1, 42, m.ID, m.RootNodeID)
	if err == nil {
		t.Fatal("expected root deletion refused")
	}
}

func TestAuditHookFires(t *testing.T) {
	repo := newFakeRepo()
	svc := NewMindMapService(repo)
	called := 0
	svc.SetAuditHook(func(ctx context.Context, tenantID uint64, mapID string, userID uint64, action, detail string) {
		called++
	})
	m, _ := svc.CreateMindMap(context.Background(), 1, 42, types.CreateMindMapRequest{Title: "x"})
	if called == 0 {
		t.Fatal("expected audit hook to fire")
	}
	_ = m
}
