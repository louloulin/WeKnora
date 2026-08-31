package weknora

import (
	"context"

)

// AgentStudioService exposes the Custom Agent Studio (/agents) surface.
type AgentStudioService struct{ c *Client }

// NewAgentStudioService constructs an AgentStudioService.
func NewAgentStudioService(c *Client) *AgentStudioService {
	return &AgentStudioService{c: c}
}

// Create inserts a new agent.
func (s *AgentStudioService) Create(ctx context.Context, in  AgentInput) (* Agent, error) {
	var out  Agent
	if err := s.c.Do(ctx, "POST", "/agents", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns all agents.
func (s *AgentStudioService) List(ctx context.Context) ([] Agent, error) {
	var out [] Agent
	if err := s.c.Do(ctx, "GET", "/agents", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Run kicks off an agent run synchronously and returns the persisted AgentRun.
func (s *AgentStudioService) Run(ctx context.Context, agentID string, in  AgentRunRequest) (* AgentRun, error) {
	var out  AgentRun
	if err := s.c.Do(ctx, "POST", "/agents/"+agentID+"/runs", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
