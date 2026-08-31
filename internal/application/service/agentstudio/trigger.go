package agentstudio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Trigger errors.
var (
	ErrTriggerInvalidType = errors.New("agentstudio.trigger: invalid trigger type")
	ErrTriggerInvalidName = errors.New("agentstudio.trigger: name must be non-empty + unique per agent")
)

// Trigger manages the lifecycle of agent_triggers. The cron expressions
// follow the standard 5-field format (minute hour day-of-month month
// day-of-week). Event triggers use a simple match: glob-style key paths.
// Webhook triggers fire on inbound POST /api/v1/.../triggers/:id/fire.
type Trigger struct {
	repo   typesRepo
	now    func() time.Time
	cron   *cronParser
}

// NewTrigger wires the trigger manager to the repo. Cron parsing uses the
// lightweight in-process parser — no external cron lib required.
func NewTrigger(repo typesRepo) *Trigger {
	return &Trigger{repo: repo, now: time.Now, cron: newCronParser()}
}

// Create validates and persists a new trigger. Computes the first
// next_fire_at for cron types so the scheduler can pick it up.
func (t *Trigger) Create(ctx context.Context, tenantID, createdBy uint64,
	agentID, triggerType, name, triggerConfigJSON, payloadTemplate string,
) (*types.AgentTrigger, error) {
	if name == "" {
		return nil, ErrTriggerInvalidName
	}
	if !validTriggerType(triggerType) {
		return nil, ErrTriggerInvalidType
	}
	cfg, err := types.AgentStudioParseTriggerConfig(triggerConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("trigger.config: %w", err)
	}
	trig := &types.AgentTrigger{
		TenantID:        tenantID,
		AgentID:         agentID,
		TriggerType:     triggerType,
		Name:            name,
		TriggerConfig:   triggerConfigJSON,
		PayloadTemplate: payloadTemplate,
		Status:          types.AgentTriggerStatusActive,
		CreatedBy:       createdBy,
	}
	if triggerType == types.AgentTriggerTypeCron {
		expr, _ := cfg["cron"].(string)
		next, err := t.cron.nextAfter(expr, t.now())
		if err != nil {
			return nil, fmt.Errorf("trigger.cron: %w", err)
		}
		trig.NextFireAt = &next
	}
	if err := t.repo.CreateTrigger(ctx, trig); err != nil {
		return nil, err
	}
	return trig, nil
}

// Fire marks the trigger as fired (updates last_fired_at + last_fire_status +
// next_fire_at). Called by the scheduler after the run completes.
func (t *Trigger) Fire(ctx context.Context, tenantID uint64, id uint64,
	status string,
) error {
	trig, err := t.repo.GetTrigger(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if trig == nil {
		return fmt.Errorf("trigger.fire: id=%d not found", id)
	}
	now := t.now()
	trig.LastFiredAt = &now
	trig.LastFireStatus = status
	if trig.TriggerType == types.AgentTriggerTypeCron {
		cfg, _ := types.AgentStudioParseTriggerConfig(trig.TriggerConfig)
		expr, _ := cfg["cron"].(string)
		if next, err := t.cron.nextAfter(expr, now); err == nil {
			trig.NextFireAt = &next
		}
	}
	return t.repo.UpdateTrigger(ctx, trig)
}

// Pause flips status to paused; the scheduler skips paused triggers.
func (t *Trigger) Pause(ctx context.Context, tenantID uint64, id uint64) error {
	trig, err := t.repo.GetTrigger(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if trig == nil {
		return fmt.Errorf("trigger.pause: id=%d not found", id)
	}
	trig.Status = types.AgentTriggerStatusPaused
	return t.repo.UpdateTrigger(ctx, trig)
}

// Resume flips status back to active; resets next_fire_at for cron types.
func (t *Trigger) Resume(ctx context.Context, tenantID uint64, id uint64) error {
	trig, err := t.repo.GetTrigger(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if trig == nil {
		return fmt.Errorf("trigger.resume: id=%d not found", id)
	}
	trig.Status = types.AgentTriggerStatusActive
	if trig.TriggerType == types.AgentTriggerTypeCron {
		cfg, _ := types.AgentStudioParseTriggerConfig(trig.TriggerConfig)
		expr, _ := cfg["cron"].(string)
		if next, err := t.cron.nextAfter(expr, t.now()); err == nil {
			trig.NextFireAt = &next
		}
	}
	return t.repo.UpdateTrigger(ctx, trig)
}

// Delete removes the trigger (idempotent on missing).
func (t *Trigger) Delete(ctx context.Context, tenantID uint64, id uint64) error {
	return t.repo.DeleteTrigger(ctx, tenantID, id)
}

// ListByAgent returns all triggers for one agent.
func (t *Trigger) ListByAgent(ctx context.Context, tenantID uint64, agentID string) ([]*types.AgentTrigger, error) {
	return t.repo.ListTriggersByAgent(ctx, tenantID, agentID)
}

// RenderPayload applies the trigger's payload_template to the event data
// and returns a JSON string ready to pass to the agent. Templates use the
// {{.key}} syntax; the data map is the trigger's configured event payload.
func (t *Trigger) RenderPayload(template string, data map[string]any) (string, error) {
	if template == "" {
		// No template — pass the data through as JSON.
		if data == nil {
			return "{}", nil
		}
		b, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	out := template
	for k, v := range data {
		placeholder := "{{." + k + "}}"
		out = strings.ReplaceAll(out, placeholder, fmt.Sprintf("%v", v))
	}
	return out, nil
}

// validTriggerType reports whether s matches one of the canonical types.
func validTriggerType(s string) bool {
	switch s {
	case types.AgentTriggerTypeManual,
		types.AgentTriggerTypeCron,
		types.AgentTriggerTypeEvent,
		types.AgentTriggerTypeWebhook:
		return true
	}
	return false
}
