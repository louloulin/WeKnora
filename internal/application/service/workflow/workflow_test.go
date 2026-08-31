package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeRepo is an in-memory implementation of the repo surfaces used by
// the service + executor in tests.
type fakeRepo struct {
	mu        sync.Mutex
	workflows map[string]*types.Workflow
	runs      map[string]*types.WorkflowRun
	nodeRuns  map[string]*types.WorkflowNodeRun
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		workflows: map[string]*types.Workflow{},
		runs:      map[string]*types.WorkflowRun{},
		nodeRuns:  map[string]*types.WorkflowNodeRun{},
	}
}

func (r *fakeRepo) CreateWorkflow(_ context.Context, w *types.Workflow) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.workflows[w.ID] = w
	return nil
}
func (r *fakeRepo) GetWorkflow(_ context.Context, tenantID uint64, id string) (*types.Workflow, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	if w, ok := r.workflows[id]; ok && w.TenantID == tenantID {
		return w, nil
	}
	return nil, errors.New("not found")
}
func (r *fakeRepo) ListWorkflowsByKB(_ context.Context, tenantID uint64, kbID string) ([]*types.Workflow, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.Workflow
	for _, w := range r.workflows {
		if w.TenantID == tenantID && w.KBID == kbID {
			out = append(out, w)
		}
	}
	return out, nil
}
func (r *fakeRepo) UpdateWorkflow(_ context.Context, w *types.Workflow) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.workflows[w.ID] = w
	return nil
}
func (r *fakeRepo) DeleteWorkflow(_ context.Context, tenantID uint64, id string) error {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.workflows, id)
	return nil
}
func (r *fakeRepo) CreateRun(_ context.Context, run *types.WorkflowRun) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}
func (r *fakeRepo) UpdateRun(_ context.Context, run *types.WorkflowRun) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}
func (r *fakeRepo) GetRun(_ context.Context, id string) (*types.WorkflowRun, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	if run, ok := r.runs[id]; ok {
		return run, nil
	}
	return nil, errors.New("not found")
}
func (r *fakeRepo) ListRunsByWorkflow(_ context.Context, workflowID string, limit int) ([]*types.WorkflowRun, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.WorkflowRun
	for _, run := range r.runs {
		if run.WorkflowID == workflowID {
			out = append(out, run)
		}
	}
	return out, nil
}
func (r *fakeRepo) CreateNodeRun(_ context.Context, nr *types.WorkflowNodeRun) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.nodeRuns[nr.ID] = nr
	return nil
}
func (r *fakeRepo) UpdateNodeRun(_ context.Context, nr *types.WorkflowNodeRun) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.nodeRuns[nr.ID] = nr
	return nil
}
func (r *fakeRepo) ListNodeRuns(_ context.Context, runID string) ([]*types.WorkflowNodeRun, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	var out []*types.WorkflowNodeRun
	for _, nr := range r.nodeRuns {
		if nr.RunID == runID {
			out = append(out, nr)
		}
	}
	return out, nil
}

func validWorkflow() *types.Workflow {
	w := &types.Workflow{
		Name: "Triage",
		Nodes: []types.WorkflowNode{
			{ID: "t", Type: types.WorkflowTriggerWebhook},
			{ID: "ask", Type: types.WorkflowAILLM, Config: json.RawMessage(`{"prompt":"summarise"}`)},
			{ID: "post", Type: types.WorkflowSendWebhook, Config: json.RawMessage(`{"url":"https://x.test"}`)},
		},
		Edges: []types.WorkflowEdge{
			{ID: "e1", SrcNodeID: "t", DstNodeID: "ask"},
			{ID: "e2", SrcNodeID: "ask", DstNodeID: "post"},
		},
	}
	w.Enabled = true
	return w
}

func TestService_CreateValidatesDAG(t *testing.T) {
	svc := NewService(newFakeRepo())
	w := validWorkflow()
	if err := svc.Create(context.Background(), w); err != nil {
		t.Fatalf("expected valid DAG, got %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected id to be assigned")
	}
	if w.Version != 1 {
		t.Fatalf("expected version 1, got %d", w.Version)
	}
}

func TestService_CreateRejectsMissingTrigger(t *testing.T) {
	svc := NewService(newFakeRepo())
	w := &types.Workflow{
		Name: "no-trigger",
		Nodes: []types.WorkflowNode{
			{ID: "a", Type: types.WorkflowAILLM},
			{ID: "b", Type: types.WorkflowReturn},
		},
		Edges: []types.WorkflowEdge{{SrcNodeID: "a", DstNodeID: "b"}},
	}
	if err := svc.Create(context.Background(), w); !errors.Is(err, ErrNoTrigger) {
		t.Fatalf("expected ErrNoTrigger, got %v", err)
	}
}

func TestService_CreateRejectsCycle(t *testing.T) {
	svc := NewService(newFakeRepo())
	w := &types.Workflow{
		Name: "cycle",
		Nodes: []types.WorkflowNode{
			{ID: "t", Type: types.WorkflowTriggerManual},
			{ID: "a", Type: types.WorkflowReturn},
			{ID: "b", Type: types.WorkflowReturn},
		},
		Edges: []types.WorkflowEdge{
			{SrcNodeID: "t", DstNodeID: "a"},
			{SrcNodeID: "a", DstNodeID: "b"},
			{SrcNodeID: "b", DstNodeID: "a"},
		},
	}
	if err := svc.Create(context.Background(), w); !errors.Is(err, ErrCyclicWorkflow) {
		t.Fatalf("expected ErrCyclicWorkflow, got %v", err)
	}
}

func TestService_CreateRejectsUnknownEdge(t *testing.T) {
	svc := NewService(newFakeRepo())
	w := &types.Workflow{
		Name: "bad-edge",
		Nodes: []types.WorkflowNode{
			{ID: "t", Type: types.WorkflowTriggerManual},
			{ID: "a", Type: types.WorkflowReturn},
		},
		Edges: []types.WorkflowEdge{{SrcNodeID: "t", DstNodeID: "ghost"}},
	}
	if err := svc.Create(context.Background(), w); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("expected ErrUnknownNode, got %v", err)
	}
}

func TestTopologicalSortOrdersTriggersFirst(t *testing.T) {
	w := validWorkflow()
	order, err := TopologicalSort(w)
	if err != nil {
		t.Fatalf("topo: %v", err)
	}
	if order[0] != "t" {
		t.Fatalf("expected first node to be trigger 't', got %q", order[0])
	}
}

func TestRun_PersistsQueuedRun(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	w := validWorkflow()
	if err := svc.Create(context.Background(), w); err != nil {
		t.Fatalf("create: %v", err)
	}
	run, err := svc.Run(context.Background(), 0, w.ID, "manual", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Status != string(types.WorkflowStatusQueued) {
		t.Fatalf("expected queued, got %s", run.Status)
	}
	if run.ID == "" {
		t.Fatal("expected id")
	}
}

// fakeLLM returns a fixed response for ai_llm nodes.
type fakeLLM struct{ out string }

func (f *fakeLLM) Complete(_ context.Context, _, _ string) (string, error) { return f.out, nil }

// fakeAgent returns a fixed map for ai_agent nodes.
type fakeAgent struct{ out map[string]any }

func (f *fakeAgent) RunAgent(_ context.Context, _ uint64, _ string, input map[string]any) (map[string]any, error) {
	return f.out, nil
}

// fakeWebhook records the URL + payload.
type fakeWebhook struct {
	mu     sync.Mutex
	posts  []string
}

func (f *fakeWebhook) Post(_ context.Context, url string, _ map[string]any) error {
	f.mu.Lock(); defer f.mu.Unlock()
	f.posts = append(f.posts, url)
	return nil
}

func TestExecutor_WalksDAGAndPersistsNodeRuns(t *testing.T) {
	repo := newFakeRepo()
	webhook := &fakeWebhook{}
	ex := NewExecutor(repo, &fakeLLM{out: "OK"}, &fakeAgent{out: map[string]any{"a": 1}}, webhook)

	// Pre-create the workflow so the executor can fetch it via the repo.
	w := validWorkflow()
	if err := repo.CreateWorkflow(context.Background(), w); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	run := &types.WorkflowRun{
		ID:         "run-1",
		WorkflowID: w.ID,
		TenantID:   0,
		Input:      json.RawMessage(`{"q":"hello"}`),
	}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	final, err := ex.Execute(context.Background(), w, run)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if final.Status != string(types.WorkflowStatusSucceeded) {
		t.Fatalf("expected succeeded, got %s", final.Status)
	}
	// Three node runs were persisted.
	if len(repo.nodeRuns) != 3 {
		t.Fatalf("expected 3 node runs, got %d", len(repo.nodeRuns))
	}
	// Webhook was called once.
	if len(webhook.posts) != 1 {
		t.Fatalf("expected 1 webhook post, got %d", len(webhook.posts))
	}
}

func TestExecutor_FailsFastOnNodeError(t *testing.T) {
	repo := newFakeRepo()
	ex := NewExecutor(repo, nil, nil, nil) // no LLM -> ai_llm node fails

	w := &types.Workflow{
		Name: "fail",
		Nodes: []types.WorkflowNode{
			{ID: "t", Type: types.WorkflowTriggerManual},
			{ID: "ask", Type: types.WorkflowAILLM},
		},
		Edges: []types.WorkflowEdge{{SrcNodeID: "t", DstNodeID: "ask"}},
	}
	if err := repo.CreateWorkflow(context.Background(), w); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	run := &types.WorkflowRun{ID: "run-2", WorkflowID: w.ID, TenantID: 0, Input: json.RawMessage(`{}`)}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	final, err := ex.Execute(context.Background(), w, run)
	if err == nil {
		t.Fatal("expected error from missing LLMCaller")
	}
	if final.Status != string(types.WorkflowStatusFailed) {
		t.Fatalf("expected failed, got %s", final.Status)
	}
}

func TestExecutor_NotifyNodeIsNoOp(t *testing.T) {
	repo := newFakeRepo()
	ex := NewExecutor(repo, nil, nil, nil)

	w := &types.Workflow{
		Name: "notify",
		Nodes: []types.WorkflowNode{
			{ID: "t", Type: types.WorkflowTriggerManual},
			{ID: "n", Type: types.WorkflowNotify},
		},
		Edges: []types.WorkflowEdge{{SrcNodeID: "t", DstNodeID: "n"}},
	}
	if err := repo.CreateWorkflow(context.Background(), w); err != nil {
		t.Fatalf("create: %v", err)
	}
	run := &types.WorkflowRun{ID: "run-3", WorkflowID: w.ID, TenantID: 0, Input: json.RawMessage(`{}`)}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	final, err := ex.Execute(context.Background(), w, run)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if final.Status != string(types.WorkflowStatusSucceeded) {
		t.Fatalf("expected succeeded, got %s", final.Status)
	}
}

var _ = time.Now
