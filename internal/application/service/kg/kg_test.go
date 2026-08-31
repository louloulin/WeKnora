package kg

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeKGRepo is an in-memory KGRepository for tests.
type fakeKGRepo struct {
	mu         sync.Mutex
	supertags  map[string]*types.KGSupertag
	entities   map[string]*types.KGEntity
	relations  map[string]*types.KGEntityRelation
	cmds       map[string]*types.KGSupertagCommand
	failCreate bool
}

func newFakeRepo() *fakeKGRepo {
	return &fakeKGRepo{
		supertags: map[string]*types.KGSupertag{},
		entities:  map[string]*types. KGEntity{},
		relations: map[string]*types.KGEntityRelation{},
		cmds:      map[string]*types.KGSupertagCommand{},
	}
}

func (r *fakeKGRepo) CreateSupertag(ctx context.Context, st *types.KGSupertag) error {
	r.mu.Lock(); defer r.mu.Unlock()
	if r.failCreate {
		return errors.New("fake: fail")
	}
	r.supertags[st.ID] = st
	return nil
}
func (r *fakeKGRepo) GetSupertag(ctx context.Context, tenantID uint64, id string) (*types.KGSupertag, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	if st, ok := r.supertags[id]; ok && st.TenantID == tenantID {
		return st, nil
	}
	return nil, nil
}
func (r *fakeKGRepo) ListSupertagsByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.KGSupertag, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGSupertag
	for _, st := range r.supertags {
		if st.TenantID == tenantID && st.KBID == kbID {
			out = append(out, st)
		}
	}
	return out, nil
}
func (r *fakeKGRepo) UpdateSupertag(ctx context.Context, st *types.KGSupertag) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.supertags[st.ID] = st
	return nil
}
func (r *fakeKGRepo) DeleteSupertag(ctx context.Context, tenantID uint64, id string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.supertags, id)
	return nil
}
func (r *fakeKGRepo) CreateEntity(ctx context.Context, e *types.KGEntity) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.entities[e.ID] = e
	return nil
}
func (r *fakeKGRepo) GetEntity(ctx context.Context, tenantID uint64, id string) (*types.KGEntity, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	if e, ok := r.entities[id]; ok && e.TenantID == tenantID {
		return e, nil
	}
	return nil, nil
}
func (r *fakeKGRepo) FindEntitiesByName(ctx context.Context, tenantID uint64, kbID, name string) ([]*types.KGEntity, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGEntity
	for _, e := range r.entities {
		if e.TenantID == tenantID && e.KBID == kbID && e.Name == name {
			out = append(out, e)
		}
	}
	return out, nil
}
func (r *fakeKGRepo) UpdateEntity(ctx context.Context, e *types.KGEntity) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.entities[e.ID] = e
	return nil
}
func (r *fakeKGRepo) BumpEntityOccurrence(ctx context.Context, id string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	if e, ok := r.entities[id]; ok {
		e.Occurrence++
	}
	return nil
}
func (r *fakeKGRepo) ListEntitiesBySupertag(ctx context.Context, tenantID uint64, supertagID string, limit int) ([]*types.KGEntity, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGEntity
	for _, e := range r.entities {
		if e.TenantID == tenantID && e.SupertagID != nil && *e.SupertagID == supertagID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (r *fakeKGRepo) CreateRelation(ctx context.Context, rel *types.KGEntityRelation) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.relations[rel.ID] = rel
	return nil
}
func (r *fakeKGRepo) ListRelationsByEntity(ctx context.Context, tenantID uint64, entityID string, limit int) ([]*types.KGEntityRelation, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGEntityRelation
	for _, r2 := range r.relations {
		if (r2.SrcEntityID == entityID || r2.DstEntityID == entityID) {
			out = append(out, r2)
		}
	}
	return out, nil
}
func (r *fakeKGRepo) ListRelationsByKB(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.KGEntityRelation, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGEntityRelation
	for _, rel := range r.relations {
		if src, ok := r.entities[rel.SrcEntityID]; ok && src.TenantID == tenantID && src.KBID == kbID {
			out = append(out, rel)
		}
	}
	return out, nil
}
func (r *fakeKGRepo) CreateKGSupertagCommand(ctx context.Context, cmd *types.KGSupertagCommand) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.cmds[cmd.ID] = cmd
	return nil
}
func (r *fakeKGRepo) ListKGSupertagCommands(ctx context.Context, supertagID, event string) ([]*types.KGSupertagCommand, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.KGSupertagCommand
	for _, c := range r.cmds {
		if c.SupertagID == supertagID && (event == "" || c.Event == event) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Compile-time check.
var _ interfaces.KGRepository = (*fakeKGRepo)(nil)

func TestSupertagCreateRequiresName(t *testing.T) {
	svc := NewKGSupertagService(newFakeRepo())
	err := svc.Create(context.Background(), &types.KGSupertag{TenantID: 1, KBID: "kb"})
	if err == nil {
		t.Fatal("expected name required error")
	}
}

func TestSupertagCreateRejectsInvalidSchema(t *testing.T) {
	svc := NewKGSupertagService(newFakeRepo())
	err := svc.Create(context.Background(), &types.KGSupertag{
		Name:   "person",
		Schema: json.RawMessage("not json"),
	})
	if err == nil {
		t.Fatal("expected schema parse error")
	}
}

func TestSupertagBindRejectsMissingRequired(t *testing.T) {
	repo := newFakeRepo()
	svc := NewKGSupertagService(repo)
	tagID := "tag-1"
	entID := "ent-1"
	schema, _ := json.Marshal([]types.KGSupertagField{{Name: "email", Required: true, Type: "text"}})
	_ = repo.CreateSupertag(context.Background(), &types.KGSupertag{
		ID: tagID, TenantID: 1, KBID: "kb", Name: "person", Schema: schema,
	})
	_ = repo.CreateEntity(context.Background(), &types. KGEntity{ID: entID, TenantID: 1, KBID: "kb", Name: "Alice"})
	_, err := svc.BindSupertag(context.Background(), 1, entID, tagID, nil)
	if err == nil {
		t.Fatal("expected missing required field error")
	}
}

func TestSupertagBindSucceedsWhenRequiredPresent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewKGSupertagService(repo)
	schema, _ := json.Marshal([]types.KGSupertagField{{Name: "email", Required: true, Type: "text"}})
	_ = repo.CreateSupertag(context.Background(), &types.KGSupertag{
		ID: "tag-1", TenantID: 1, KBID: "kb", Name: "person", Schema: schema,
	})
	_ = repo.CreateEntity(context.Background(), &types. KGEntity{ID: "ent-1", TenantID: 1, KBID: "kb", Name: "Alice"})
	entity, err := svc.BindSupertag(context.Background(), 1, "ent-1", "tag-1", map[string]any{"email": "a@x"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if entity.SupertagID == nil || *entity.SupertagID != "tag-1" {
		t.Fatalf("supertag not attached: %+v", entity)
	}
}

func TestNERPipeline_RegexFallback(t *testing.T) {
	p := NewNERPipeline(nil) // nil llm -> regex fallback
	drafts, err := p.Extract(context.Background(), "doc-1", "Alice met Bob at Acme Corp.")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(drafts) < 2 {
		t.Fatalf("expected >= 2 entities, got %d", len(drafts))
	}
	// Acme Corp is two words so the regex captures it.
	foundAcme := false
	for _, d := range drafts {
		if d.Name == "Acme Corp" {
			foundAcme = true
		}
	}
	if !foundAcme {
		t.Errorf("expected Acme Corp in entities, got %+v", drafts)
	}
}

func TestREPipeline_HeuristicPairsEntities(t *testing.T) {
	p := NewREPipeline(nil)
	entities := []types. KGEntityDraft{
		{TmpID: "1", Name: "Alice"},
		{TmpID: "2", Name: "Bob"},
		{TmpID: "3", Name: "Acme"},
	}
	rels, err := p.Extract(context.Background(), "doc", "x", entities)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("expected 2 heuristic edges, got %d", len(rels))
	}
}

func TestREPipeline_PersistDraftsDedupesByName(t *testing.T) {
	repo := newFakeRepo()
	svc := NewKGSupertagService(repo)
	re := NewREPipeline(nil)
	result := &types.KGExtractionResult{
		DocumentID: "doc-1",
		Entities: []types. KGEntityDraft{
			{TmpID: "a", Name: "Alice"},
			{TmpID: "b", Name: "Bob"},
		},
		Relations: []types.KGRelationDraft{
			{SrcTmpID: "a", DstTmpID: "b", Relation: "knows", Confidence: 0.8},
		},
	}
	if err := re.PersistDrafts(context.Background(), svc, 1, "kb-1", "doc-1", result); err != nil {
		t.Fatalf("persist: %v", err)
	}
	// Re-run — Alice and Bob should be deduped (occurrence bumped).
	if err := re.PersistDrafts(context.Background(), svc, 1, "kb-1", "doc-1", result); err != nil {
		t.Fatalf("persist 2: %v", err)
	}
	if len(repo.entities) != 2 {
		t.Fatalf("expected 2 unique entities, got %d", len(repo.entities))
	}
	for _, e := range repo.entities {
		if e.Occurrence != 2 {
			t.Errorf("entity %s occurrence = %d, want 2", e.Name, e.Occurrence)
		}
	}
}
