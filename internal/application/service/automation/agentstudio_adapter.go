// Package automation provides the AgentStudio adapter so the RunAgent
// action can invoke a real agent without importing the studio package
// directly (which would cause a circular dependency).
package automation

import (
	"context"
	"errors"
	"strconv"

	agentstudio "github.com/Tencent/WeKnora/internal/application/service/agentstudio"
)

// Sentinel errors for the AgentStudio adapter.
var (
	errAgentStudioNotConfigured = errors.New("automation: agent studio runner not wired")
	errAgentStudioInvalidInput  = errors.New("automation: tenant_id and agent_id are required")
	errAgentStudioNilRun        = errors.New("automation: agent studio returned a nil run")
)

// AgentStudioAdapter implements the AgentRunner interface by delegating
// to the underlying AgentStudioService.Run. The adapter is the bridge
// between the Build #33 Automation engine and the Build #21 Custom
// Agent Studio, mirroring Notion's "Button -> Agent" pattern and Coda's
// "Pack Agent invocation".
type AgentStudioAdapter struct {
	svc *agentstudio.AgentStudioService
}

// NewAgentStudioAdapter constructs the adapter.
func NewAgentStudioAdapter(svc *agentstudio.AgentStudioService) *AgentStudioAdapter {
	return &AgentStudioAdapter{svc: svc}
}

// Run invokes the agent identified by agentID with the supplied input +
// structured payload, then returns the created run's ID (as a string) so
// the automation engine can persist it on AutomationStepRun.Result.
func (a *AgentStudioAdapter) Run(
	ctx context.Context,
	tenantID, userID uint64,
	agentID, input string,
	payload map[string]any,
) (string, error) {
	if a.svc == nil {
		return "", errAgentStudioNotConfigured
	}
	if tenantID == 0 || agentID == "" {
		return "", errAgentStudioInvalidInput
	}
	merged := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		merged[k] = v
	}
	if input != "" {
		merged["input"] = input
	}
	opts := agentstudio.RunOpts{
		TenantID:      tenantID,
		AgentID:       agentID,
		TriggeredBy:   "automation",
		TriggeredUser: &userID,
		Input:         merged,
	}
	run, err := a.svc.Run(ctx, opts)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", errAgentStudioNilRun
	}
	return strconv.FormatUint(run.ID, 10), nil
}
