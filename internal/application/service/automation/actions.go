package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/formula"
	"github.com/Tencent/WeKnora/internal/types"
)

// ActionContext is the read-only snapshot an action receives when
// it runs. Fields are populated as the DAG executes: each step's
// output is keyed by step id so later steps can reference earlier
// results.
type ActionContext struct {
	TenantID   uint64
	DatabaseID string
	RowID      string
	UserID     uint64
	Trigger    types.AutomationTriggerType

	// Output is a map of step id -> arbitrary JSON. Later steps can
	// reference earlier outputs via the {{steps.<id>.field}} syntax
	// in their config.
	Output map[string]any

	// Now allows tests to freeze time.
	Now func() time.Time
}

// ActionResult is the success payload written to the run record.
type ActionResult struct {
	Output map[string]any
}

// Action is the runtime interface every step type implements.
type Action interface {
	Kind() types.AutomationActionType
	Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error)
}

// ActionRegistry holds the registered actions keyed by kind. The
// Service composes a registry at construction time; tests can build
// their own minimal registries.
type ActionRegistry struct {
	actions map[types.AutomationActionType]Action
}

// NewActionRegistry returns an empty registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: map[types.AutomationActionType]Action{}}
}

// Register adds an action to the registry. Overwrites any prior
// action for the same kind.
func (r *ActionRegistry) Register(a Action) {
	r.actions[a.Kind()] = a
}

// Get returns the action for kind, or nil if none is registered.
func (r *ActionRegistry) Get(kind types.AutomationActionType) Action {
	return r.actions[kind]
}

// Kinds returns the list of registered action kinds. Used by the
// frontend to populate the picker.
func (r *ActionRegistry) Kinds() []types.AutomationActionType {
	out := make([]types.AutomationActionType, 0, len(r.actions))
	for k := range r.actions {
		out = append(out, k)
	}
	return out
}

// --- Built-in actions ---

// UpdateFieldAction writes a value into a row column. The value can
// be a literal or a {{formula expression}} (forwarded to the
// formula engine when the value is a string starting with "=").
type UpdateFieldAction struct{}

func (UpdateFieldAction) Kind() types.AutomationActionType { return types.AutomationActionUpdateField }

// UpdateFieldConfig is the parsed config for UpdateFieldAction.
type UpdateFieldConfig struct {
	ColumnID   string `json:"column_id"`
	ColumnName string `json:"column_name,omitempty"`
	Value      any    `json:"value"`
	// ValueFormula, when set, is evaluated against ac.Output and
	// the resulting value is used instead of Value. This lets a step
	// reference earlier step outputs.
	ValueFormula string `json:"value_formula,omitempty"`
}

func (UpdateFieldAction) Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error) {
	cfg, err := parseConfig[UpdateFieldConfig](step.Config)
	if err != nil {
		return nil, err
	}
	if cfg.ColumnID == "" && cfg.ColumnName == "" {
		return nil, errors.New("update_field: column_id or column_name required")
	}
	value := cfg.Value
	if cfg.ValueFormula != "" {
		expr, err := formula.Evaluate(cfg.ValueFormula, formula.NewContext(toFormulaValues(ac.Output)))
		if err != nil {
			return nil, fmt.Errorf("update_field: formula: %w", err)
		}
		value = expr.AsString()
	}
	return &ActionResult{
		Output: map[string]any{
			"column":   firstNonEmpty(cfg.ColumnID, cfg.ColumnName),
			"value":    value,
			"row_id":   ac.RowID,
		},
	}, nil
}

// CreateRowAction appends a new row to the database. The output
// contains the new row id, which the caller can persist back into
// the database row repo.
type CreateRowAction struct{}

func (CreateRowAction) Kind() types.AutomationActionType { return types.AutomationActionCreateRow }

// CreateRowConfig is the parsed config for CreateRowAction.
type CreateRowConfig struct {
	Values map[string]any `json:"values"`
}

func (CreateRowAction) Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error) {
	cfg, err := parseConfig[CreateRowConfig](step.Config)
	if err != nil {
		return nil, err
	}
	if len(cfg.Values) == 0 {
		return nil, errors.New("create_row: values required")
	}
	return &ActionResult{
		Output: map[string]any{
			"database_id": ac.DatabaseID,
			"values":      cfg.Values,
		},
	}, nil
}

// SendWebhookAction POSTs a JSON payload to a configured URL. Uses
// the standard net/http client with a 10s timeout.
type SendWebhookAction struct {
	HTTPClient *http.Client
}

func (SendWebhookAction) Kind() types.AutomationActionType { return types.AutomationActionSendWebhook }

// SendWebhookConfig is the parsed config for SendWebhookAction.
type SendWebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // POST (default) / PUT
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body"`
	TimeoutSec int             `json:"timeout_sec,omitempty"`
}

func (s SendWebhookAction) Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error) {
	cfg, err := parseConfig[SendWebhookConfig](step.Config)
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("send_webhook: url required")
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}
	body, err := json.Marshal(cfg.Body)
	if err != nil {
		return nil, fmt.Errorf("send_webhook: marshal body: %w", err)
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.TimeoutSec > 0 {
		client = &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send_webhook: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("send_webhook: status=%d body=%q", resp.StatusCode, string(respBody))
	}
	return &ActionResult{Output: map[string]any{
		"status": resp.StatusCode,
		"url":    cfg.URL,
	}}, nil
}

// RunAgentAction enqueues an agent run. The actual execution is
// delegated to the agentstudio service which the caller provides
// via the registry.
type RunAgentAction struct {
	Runner AgentRunner
}

func (RunAgentAction) Kind() types.AutomationActionType { return types.AutomationActionRunAgent }

// RunAgentConfig is the parsed config for RunAgentAction.
type RunAgentConfig struct {
	AgentID  string `json:"agent_id"`
	Input    string `json:"input"`
	InputMap map[string]any `json:"input_map,omitempty"`
}

// AgentRunner abstracts the agent studio so the action package
// does not import the studio service directly.
type AgentRunner interface {
	Run(ctx context.Context, tenantID, userID uint64, agentID, input string, payload map[string]any) (runID string, err error)
}

func (a RunAgentAction) Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error) {
	if a.Runner == nil {
		return nil, errors.New("run_agent: no runner wired")
	}
	cfg, err := parseConfig[RunAgentConfig](step.Config)
	if err != nil {
		return nil, err
	}
	if cfg.AgentID == "" {
		return nil, errors.New("run_agent: agent_id required")
	}
	runID, err := a.Runner.Run(ctx, ac.TenantID, ac.UserID, cfg.AgentID, cfg.Input, cfg.InputMap)
	if err != nil {
		return nil, err
	}
	return &ActionResult{Output: map[string]any{"run_id": runID, "agent_id": cfg.AgentID}}, nil
}

// NotifyAction records a notification intent. The actual delivery
// is left to the notification service.
type NotifyAction struct{}

func (NotifyAction) Kind() types.AutomationActionType { return types.AutomationActionNotify }

// NotifyConfig is the parsed config for NotifyAction.
type NotifyConfig struct {
	UserID  uint64 `json:"user_id"`
	Message string `json:"message"`
	Channel string `json:"channel,omitempty"` // mention / email / im
}

func (NotifyAction) Run(ctx context.Context, ac *ActionContext, step *types.AutomationStep) (*ActionResult, error) {
	cfg, err := parseConfig[NotifyConfig](step.Config)
	if err != nil {
		return nil, err
	}
	if cfg.UserID == 0 {
		cfg.UserID = ac.UserID
	}
	if cfg.Message == "" {
		return nil, errors.New("notify: message required")
	}
	return &ActionResult{Output: map[string]any{
		"user_id": cfg.UserID,
		"channel": firstNonEmpty(cfg.Channel, "mention"),
		"message": cfg.Message,
	}}, nil
}

// --- helpers ---

// parseConfig unmarshals the step's JSON config into the typed
// shape, returning a zero value when the config is empty.
func parseConfig[T any](raw types.JSON) (T, error) {
	var cfg T
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// toFormulaValues converts a map[string]any into a formula
// context-friendly shape. Only primitive types survive; complex
// objects are JSON-stringified.
func toFormulaValues(in map[string]any) map[string]formula.Value {
	out := make(map[string]formula.Value, len(in))
	for k, v := range in {
		out[k] = toFormulaValue(v)
	}
	return out
}

func toFormulaValue(v any) formula.Value {
	switch t := v.(type) {
	case string:
		return formula.Value{Kind: formula.ValueString, Str: t}
	case bool:
		return formula.Value{Kind: formula.ValueBool, Bool: t}
	case float64:
		return formula.Value{Kind: formula.ValueNumber, Num: t}
	case int:
		return formula.Value{Kind: formula.ValueNumber, Num: float64(t)}
	case int64:
		return formula.Value{Kind: formula.ValueNumber, Num: float64(t)}
	default:
		b, _ := json.Marshal(v)
		return formula.Value{Kind: formula.ValueString, Str: string(b)}
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
