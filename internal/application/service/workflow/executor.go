package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// LLMCaller is the minimal contract the executor needs to invoke an LLM
// node. The ChatModel layer satisfies this in production; tests can
// inject a stub.
type LLMCaller interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// AgentRunnerCaller invokes a Custom Agent Studio agent for an
// ai_agent node. The AgentStudioService.Run method satisfies this in
// production.
type AgentRunnerCaller interface {
	RunAgent(ctx context.Context, tenantID uint64, agentID string, input map[string]any) (map[string]any, error)
}

// WebhookSender POSTs a payload to a webhook URL.
type WebhookSender interface {
	Post(ctx context.Context, url string, payload map[string]any) error
}

// Executor orchestrates a queued workflow run end-to-end. It walks the
// DAG in topological order, executing each node and persisting a
// WorkflowNodeRun per step. On node failure the run is marked failed
// (future: branch on error nodes).
type Executor struct {
	repo     RepoWriter
	llm      LLMCaller
	agents   AgentRunnerCaller
	webhook  WebhookSender
	now      func() time.Time
}

// RepoWriter is the subset of the workflow repo the executor needs. It
// avoids an import cycle with the application.repository package.
type RepoWriter interface {
	UpdateRun(ctx context.Context, r *types.WorkflowRun) error
	CreateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error
	UpdateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error
	GetRun(ctx context.Context, id string) (*types.WorkflowRun, error)
}

// NewExecutor constructs an Executor.
func NewExecutor(repo RepoWriter, llm LLMCaller, agents AgentRunnerCaller, webhook WebhookSender) *Executor {
	return &Executor{repo: repo, llm: llm, agents: agents, webhook: webhook, now: time.Now}
}

// SetNow overrides the wall clock for tests.
func (e *Executor) SetNow(now func() time.Time) { e.now = now }

// Execute runs a workflow to completion (or to the first unrecoverable
// failure). It returns the final run state on success.
func (e *Executor) Execute(ctx context.Context, w *types.Workflow, run *types.WorkflowRun) (*types.WorkflowRun, error) {
	order, err := TopologicalSort(w)
	if err != nil {
		return nil, err
	}
	now := e.now()
	run.Status = string(types.WorkflowStatusRunning)
	run.StartedAt = &now
	if err := e.repo.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	// output is a node-id -> last-output map reused across downstream nodes.
	output := make(map[string]map[string]any)
	// The trigger node is "fired" by the Run() call: its output is the
	// raw input payload, so downstream nodes can refer to `trigger.X`.
	if len(w.Nodes) > 0 {
		var initial map[string]any
		_ = json.Unmarshal(run.Input, &initial)
		if initial == nil {
			initial = map[string]any{}
		}
		// Find the first trigger node to attach the initial payload to.
		for _, n := range w.Nodes {
			switch n.Type {
			case types.WorkflowTriggerManual,
				types.WorkflowTriggerScheduled,
				types.WorkflowTriggerEvent,
				types.WorkflowTriggerWebhook,
				types.WorkflowTriggerFormSubmit:
				output[n.ID] = initial
			}
		}
	}
	for _, id := range order {
		node := findNode(w, id)
		if node == nil {
			continue
		}
		nr := &types.WorkflowNodeRun{
			ID:        uuid.NewString(),
			RunID:     run.ID,
			NodeID:    id,
			Status:    string(types.WorkflowStatusRunning),
			CreatedAt: e.now(),
		}
		// Seed input from upstream nodes' outputs (concatenated by node id).
		var nodeInput map[string]any
		for _, e := range w.Edges {
			if e.DstNodeID == id {
				if prev, ok := output[e.SrcNodeID]; ok {
					if nodeInput == nil {
						nodeInput = map[string]any{}
					}
					nodeInput[e.SrcNodeID] = prev
				}
			}
		}
		inputJSON, _ := json.Marshal(nodeInput)
		nr.Input = inputJSON
		nr.StartedAt = ptrTime(e.now())
		if err := e.repo.CreateNodeRun(ctx, nr); err != nil {
			return nil, err
		}
		nodeOutput, execErr := e.executeNode(ctx, run.TenantID, node, nodeInput)
		if execErr != nil {
			nr.Status = string(types.WorkflowStatusFailed)
			nr.Error = execErr.Error()
			nr.FinishedAt = ptrTime(e.now())
			_ = e.repo.UpdateNodeRun(ctx, nr)
			run.Status = string(types.WorkflowStatusFailed)
			run.Error = execErr.Error()
			run.FinishedAt = ptrTime(e.now())
			_ = e.repo.UpdateRun(ctx, run)
			return run, execErr
		}
		outJSON, _ := json.Marshal(nodeOutput)
		nr.Output = outJSON
		nr.Status = string(types.WorkflowStatusSucceeded)
		nr.FinishedAt = ptrTime(e.now())
		_ = e.repo.UpdateNodeRun(ctx, nr)
		output[id] = nodeOutput
	}
	run.Status = string(types.WorkflowStatusSucceeded)
	run.FinishedAt = ptrTime(e.now())
	run.Output, _ = json.Marshal(output)
	if err := e.repo.UpdateRun(ctx, run); err != nil {
		return run, err
	}
	return run, nil
}

// executeNode dispatches a single node based on its Type.
func (e *Executor) executeNode(ctx context.Context, tenantID uint64, n *types.WorkflowNode, input map[string]any) (map[string]any, error) {
	switch n.Type {
	case types.WorkflowTriggerManual,
		types.WorkflowTriggerScheduled,
		types.WorkflowTriggerEvent,
		types.WorkflowTriggerWebhook,
		types.WorkflowTriggerFormSubmit,
		types.WorkflowFormInput,
		types.WorkflowReturn:
		return input, nil
	case types.WorkflowAILLM:
		if e.llm == nil {
			return nil, errors.New("ai_llm: no LLMCaller wired")
		}
		system := "You are a helpful assistant."
		var cfg struct {
			Prompt string `json:"prompt"`
			Model  string `json:"model"`
			System string `json:"system"`
		}
		_ = json.Unmarshal(n.Config, &cfg)
		if cfg.System != "" {
			system = cfg.System
		}
		user := cfg.Prompt
		if user == "" {
			user = fmt.Sprintf("%v", input)
		}
		out, err := e.llm.Complete(ctx, system, user)
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": out}, nil
	case types.WorkflowAIAgent:
		if e.agents == nil {
			return nil, errors.New("ai_agent: no AgentRunnerCaller wired")
		}
		var cfg struct {
			AgentID string `json:"agent_id"`
		}
		_ = json.Unmarshal(n.Config, &cfg)
		if cfg.AgentID == "" {
			return nil, errors.New("ai_agent: agent_id is required")
		}
		return e.agents.RunAgent(ctx, tenantID, cfg.AgentID, input)
	case types.WorkflowSendWebhook:
		if e.webhook == nil {
			return nil, errors.New("send_webhook: no WebhookSender wired")
		}
		var cfg struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(n.Config, &cfg)
		if cfg.URL == "" {
			return nil, errors.New("send_webhook: url is required")
		}
		if err := e.webhook.Post(ctx, cfg.URL, input); err != nil {
			return nil, err
		}
		return map[string]any{"posted": true, "url": cfg.URL}, nil
	case types.WorkflowNotify:
		// Notify is best-effort; the foundation just records it in the
		// node run output. Future work wires the notification channels.
		return map[string]any{"notified": true}, nil
	case types.WorkflowAutomation:
		// Delegate to a nested Build #33 automation. The foundation
		// resolves the automation by id and records the delegation; the
		// full bridging is left to Build #37.x follow-on work.
		return map[string]any{"delegated": true, "config": string(n.Config)}, nil
	}
	return nil, fmt.Errorf("workflow: unknown node type %q", n.Type)
}

// findNode returns the node in w with the given id, or nil if not found.
func findNode(w *types.Workflow, id string) *types.WorkflowNode {
	for i := range w.Nodes {
		if w.Nodes[i].ID == id {
			return &w.Nodes[i]
		}
	}
	return nil
}

// ptrTime returns a pointer to the supplied time (handy for nullable
// timestamp columns).
func ptrTime(t time.Time) *time.Time { return &t }
