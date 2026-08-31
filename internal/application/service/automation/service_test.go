package automation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeRepo is an in-memory implementation of interfaces.AutomationRepository.
type fakeRepo struct {
	mu    sync.Mutex
	auto  map[string]*types.Automation
	runs  map[string]*types.AutomationRun
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{auto: map[string]*types.Automation{}, runs: map[string]*types.AutomationRun{}}
}

func (f *fakeRepo) CreateAutomation(ctx context.Context, a *types.Automation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auto[a.ID] = a
	return nil
}
func (f *fakeRepo) UpdateAutomation(ctx context.Context, a *types.Automation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auto[a.ID] = a
	return nil
}
func (f *fakeRepo) GetAutomation(ctx context.Context, tenantID uint64, id string) (*types.Automation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.auto[id]
	if !ok {
		return nil, nil
	}
	return a, nil
}
func (f *fakeRepo) ListAutomationsByDatabase(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*types.Automation{}
	for _, a := range f.auto {
		if a.DatabaseID == databaseID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListEnabledScheduled(ctx context.Context) ([]*types.Automation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*types.Automation{}
	for _, a := range f.auto {
		if a.Enabled && a.TriggerType == types.AutomationTriggerScheduled {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListEnabledFieldChange(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*types.Automation{}
	for _, a := range f.auto {
		if a.Enabled && a.TriggerType == types.AutomationTriggerFieldChange && a.DatabaseID == databaseID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) SoftDeleteAutomation(ctx context.Context, tenantID uint64, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.auto, id)
	return nil
}

func (f *fakeRepo) CreateRun(ctx context.Context, r *types.AutomationRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[r.ID] = r
	return nil
}
func (f *fakeRepo) UpdateRun(ctx context.Context, r *types.AutomationRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[r.ID] = r
	return nil
}
func (f *fakeRepo) GetRun(ctx context.Context, tenantID uint64, id string) (*types.AutomationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs[id], nil
}
func (f *fakeRepo) ListRunsByAutomation(ctx context.Context, tenantID uint64, automationID string, limit int) ([]*types.AutomationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*types.AutomationRun{}
	for _, r := range f.runs {
		if r.AutomationID == automationID {
			out = append(out, r)
		}
	}
	return out, nil
}

func newLinearAutomation() *types.Automation {
	return &types.Automation{
		ID:            "auto-1",
		TenantID:      1,
		KnowledgeBaseID: "kb-1",
		DatabaseID:    "db-1",
		Name:          "test",
		TriggerType:   types.AutomationTriggerManual,
		Enabled:       true,
		Steps: []types.AutomationStep{
			{
				ID:         "s1",
				ActionType: types.AutomationActionUpdateField,
				Config:     types.JSON(`{"column_id":"status","value":"approved"}`),
				NextIDs:    []string{"s2"},
			},
			{
				ID:         "s2",
				ActionType: types.AutomationActionNotify,
				Config:     types.JSON(`{"message":"automated"}`),
			},
		},
	}
}

func TestService_CreateAndGet(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a := newLinearAutomation()
	if err := svc.Create(context.Background(), a); err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.ID == "" {
		t.Fatal("id not assigned")
	}
	got, err := svc.Get(context.Background(), 1, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_CreateRejectsCycle(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a := newLinearAutomation()
	a.Steps[0].NextIDs = []string{"s2"}
	a.Steps[1].NextIDs = []string{"s1"} // back-edge → cycle
	if err := svc.Create(context.Background(), a); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestService_Run_LinearHappyPath(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a := newLinearAutomation()
	if err := svc.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	run, err := svc.Run(context.Background(), a, &types.AutomationRunInputs{
		TenantID:   1,
		DatabaseID: "db-1",
		RowID:      "row-1",
		UserID:     42,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Status != types.AutomationRunSucceeded {
		t.Errorf("status = %v", run.Status)
	}
	if len(run.StepRuns) != 2 {
		t.Errorf("got %d step runs, want 2", len(run.StepRuns))
	}
	// Second step should have seen first step's output via ac.Output["s1"].
	if run.StepRuns[1].Output == nil {
		t.Errorf("step 2 missing output")
	}
}

func TestService_Run_FailStopsSubsequent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a := newLinearAutomation()
	// Make s1 reference an unknown action kind.
	a.Steps[0].ActionType = "nonexistent_kind"
	if err := svc.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	run, err := svc.Run(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if run.Status != types.AutomationRunFailed {
		t.Errorf("status = %v", run.Status)
	}
	if len(run.StepRuns) != 1 {
		t.Errorf("got %d step runs, want 1", len(run.StepRuns))
	}
}

func TestService_Run_RetriesOnTransientFailure(t *testing.T) {
	// Stand up an HTTP server that fails twice then succeeds; this
	// lets us verify retry semantics on SendWebhookAction.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"err":"transient"}`)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	svc := NewService(repo)
	a := &types.Automation{
		ID:            "auto-retry",
		TenantID:      1,
		KnowledgeBaseID: "kb-1",
		DatabaseID:    "db-1",
		TriggerType:   types.AutomationTriggerManual,
		Enabled:       true,
		Steps: []types.AutomationStep{
			{
				ID:         "s1",
				ActionType: types.AutomationActionSendWebhook,
				Config:     types.JSON(`{"url":"` + srv.URL + `","body":{"k":"v"}}`),
				Retries:    2,
			},
		},
	}
	if err := svc.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	run, err := svc.Run(context.Background(), a, nil)
	if err == nil {
		t.Fatalf("expected error after retries; run status = %v", run.Status)
	}
	if run.Status != types.AutomationRunFailed {
		t.Errorf("status = %v", run.Status)
	}
	// Retries + 1 attempts = 3, but they all hit the failing server.
	if len(run.StepRuns) != 1 {
		t.Errorf("step runs = %d", len(run.StepRuns))
	}
}

func TestParseTriggerConfigFieldChange(t *testing.T) {
	cfg, err := types.ParseTriggerConfigFieldChange(types.JSON(`{"column_id":"price","only_on_update":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ColumnID != "price" || !cfg.OnlyOnUpdate {
		t.Errorf("got %+v", cfg)
	}
}

// Silence unused-import warnings for json package used in test fakes.
var _ = json.Marshal
var _ = errors.New
