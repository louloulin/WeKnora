package weknora

import (
	"context"

)

// ConnectorService exposes the AI Connector framework.
type ConnectorService struct{ c *Client }

// NewConnectorService constructs a ConnectorService.
func NewConnectorService(c *Client) *ConnectorService { return &ConnectorService{c: c} }

// Install installs a connector.
func (s *ConnectorService) Install(ctx context.Context, in  ConnectorInput) (* Connector, error) {
	var out  Connector
	if err := s.c.Do(ctx, "POST", "/connectors", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns installed connectors.
func (s *ConnectorService) List(ctx context.Context) ([] Connector, error) {
	var out [] Connector
	if err := s.c.Do(ctx, "GET", "/connectors", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Sync triggers a sync run for a connector.
func (s *ConnectorService) Sync(ctx context.Context, connectorID string) error {
	return s.c.Do(ctx, "POST", "/connectors/"+connectorID+"/sync", nil, nil, nil)
}
