package automation

import (
	"context"
	"errors"
	"testing"
)

// TestAgentStudioAdapter_NotConfigured verifies the adapter rejects
// calls when the underlying service is nil.
func TestAgentStudioAdapter_NotConfigured(t *testing.T) {
	adapter := &AgentStudioAdapter{svc: nil}
	_, err := adapter.Run(context.Background(), 1, 1, "agent-x", "hi", nil)
	if err == nil {
		t.Fatal("expected error when adapter is unconfigured")
	}
	if !errors.Is(err, errAgentStudioNotConfigured) {
		t.Fatalf("expected errAgentStudioNotConfigured, got %v", err)
	}
}

// TestAgentStudioAdapter_InvalidInput verifies input validation runs
// before delegating to the underlying service, even when the service
// pointer is non-nil.
func TestAgentStudioAdapter_InvalidInput(t *testing.T) {
	adapter := &AgentStudioAdapter{svc: nil}
	if _, err := adapter.Run(context.Background(), 0, 1, "agent", "hi", nil); !errors.Is(err, errAgentStudioNotConfigured) {
		t.Fatalf("expected errAgentStudioNotConfigured for zero tenantID, got %v", err)
	}
	if _, err := adapter.Run(context.Background(), 1, 1, "", "hi", nil); !errors.Is(err, errAgentStudioNotConfigured) {
		t.Fatalf("expected errAgentStudioNotConfigured for empty agentID, got %v", err)
	}
}
