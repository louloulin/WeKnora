package weknora

import (
	"context"
	"net/url"

)

// AutomationService exposes the Build #33 Automation / Button engine.
type AutomationService struct{ c *Client }

// NewAutomationService constructs an AutomationService.
func NewAutomationService(c *Client) *AutomationService {
	return &AutomationService{c: c}
}

// Create inserts a new automation into the database.
func (s *AutomationService) Create(ctx context.Context, kbID string, in  AutomationInput) (* Automation, error) {
	var out  Automation
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/databases/"+in.DatabaseID+"/automations", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns automations for a database.
func (s *AutomationService) List(ctx context.Context, kbID, databaseID string) ([] Automation, error) {
	var out [] Automation
	if err := s.c.Do(ctx, "GET", "/knowledge-bases/"+kbID+"/databases/"+databaseID+"/automations", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Run manually triggers an automation.
func (s *AutomationService) Run(ctx context.Context, kbID, automationID string, payload map[string]any) (* AutomationRun, error) {
	var out  AutomationRun
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/automations/"+automationID+"/run", nil, payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Runs iterates over the run history of a single automation. The page token
// is supplied via query string to match the OpenAPI spec.
func (s *AutomationService) Runs(ctx context.Context, kbID, automationID, pageToken string) ( Page[ AutomationRun], error) {
	q := url.Values{}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var out  Page[ AutomationRun]
	if err := s.c.DoQuery(ctx, "GET", "/knowledge-bases/"+kbID+"/automations/"+automationID+"/runs", q, &out); err != nil {
		return  Page[ AutomationRun]{}, err
	}
	return out, nil
}
