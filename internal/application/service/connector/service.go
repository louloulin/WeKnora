// Package connector implements the v0.7.24 AI Connector framework.
// Each registered IngestConnector is mapped to a runtime
// Connector implementation (Slack, Email, Webhook, ...) at the
// moment a sync is triggered. The framework is intentionally
// minimal: it owns no goroutines, no scheduler. Sync is fired by
// the REST handler (manual) or, in v0.7.25, by the periodic
// scheduler; the rest of the platform just calls Trigger.
package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors.
var (
	ErrConnectorNotFound = errors.New("connector: not found")
	ErrConnectorDisabled = errors.New("connector: disabled")
	ErrUnknownKind       = errors.New("connector: unknown kind")
	ErrEmptyName         = errors.New("connector: name required")
)

// KnowledgeIngester is the minimal contract the IngestService
// needs from the KB layer. Splitting this out lets the tests
// inject a stub instead of pulling in the full knowledge pipeline.
type KnowledgeIngester interface {
	Ingest(ctx context.Context, tenantID, kbID, title, content, author, url string, ts time.Time) error
}

// Service is the runtime façade.
type Service struct {
	repo     interfaces.IngestConnectorRepository
	jobs     interfaces.IngestJobRepository
	registry map[types.ConnectorKind]interfaces.Connector
	ingester KnowledgeIngester
}

// NewService wires the service. Callers register concrete
// Connector implementations via Register before any sync fires.
func NewService(
	repo interfaces.IngestConnectorRepository,
	jobs interfaces.IngestJobRepository,
	ingester KnowledgeIngester,
) *Service {
	return &Service{
		repo:     repo,
		jobs:     jobs,
		registry: map[types.ConnectorKind]interfaces.Connector{},
		ingester: ingester,
	}
}

// Register makes a Connector implementation available for a kind.
func (s *Service) Register(c interfaces.Connector) {
	s.registry[c.Kind()] = c
}

// Kinds returns the list of registered connector kinds. Used by
// the admin UI to populate the kind selector.
func (s *Service) Kinds() []types.ConnectorKind {
	out := make([]types.ConnectorKind, 0, len(s.registry))
	for k := range s.registry {
		out = append(out, k)
	}
	return out
}

// Create persists a new connector. Validates that the kind is
// registered and the name is non-empty.
func (s *Service) Create(ctx context.Context, c *types.IngestConnector) error {
	if strings.TrimSpace(c.Name) == "" {
		return ErrEmptyName
	}
	if _, ok := s.registry[c.Kind]; !ok {
		return fmt.Errorf("%w: kind=%q", ErrUnknownKind, c.Kind)
	}
	return s.repo.Create(ctx, c)
}

// Update mutates an existing connector.
func (s *Service) Update(ctx context.Context, c *types.IngestConnector) error {
	if c.ID == 0 {
		return ErrConnectorNotFound
	}
	if _, ok := s.registry[c.Kind]; !ok {
		return fmt.Errorf("%w: kind=%q", ErrUnknownKind, c.Kind)
	}
	return s.repo.Update(ctx, c)
}

// Get returns one connector.
func (s *Service) Get(ctx context.Context, tenantID string, id uint64) (*types.IngestConnector, error) {
	return s.repo.Get(ctx, tenantID, id)
}

// List returns tenant connectors.
func (s *Service) List(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestConnector, int, error) {
	return s.repo.List(ctx, tenantID, limit, offset)
}

// Delete soft-deletes a connector.
func (s *Service) Delete(ctx context.Context, tenantID string, id uint64) error {
	return s.repo.Delete(ctx, tenantID, id)
}

// ListJobs returns recent ingest jobs for one connector.
func (s *Service) ListJobs(ctx context.Context, tenantID string, connectorID uint64, limit, offset int) ([]*types.IngestJob, int, error) {
	return s.jobs.ListByConnector(ctx, tenantID, connectorID, limit, offset)
}

// ListTenantJobs returns recent ingest jobs across all connectors
// for the tenant — used by the cross-connector health view.
func (s *Service) ListTenantJobs(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestJob, int, error) {
	return s.jobs.ListByTenant(ctx, tenantID, limit, offset)
}

// Trigger fires one sync run. Idempotent at the job-row level
// (every run creates one row) but the connector-level dedup
// (message-id) is the connector implementation's job.
//
// Flow:
//   1. Look up the connector; refuse if disabled.
//   2. Create a job row in queued state.
//   3. Mark running, call the registered Connector.Fetch.
//   4. For each returned message, hand off to the ingester.
//   5. Mark succeeded (or failed), update last_sync_at + last_error.
//
// Returning an error from Trigger is reserved for setup-level
// failures (no connector / no ingester / unknown kind). Per-message
// failures do NOT abort the run; they are logged and the job's
// result_count reflects successful ingests only.
func (s *Service) Trigger(ctx context.Context, tenantID string, id uint64) (*types.IngestJob, error) {
	conn, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrConnectorNotFound
	}
	if !conn.Enabled {
		return nil, ErrConnectorDisabled
	}
	impl, ok := s.registry[conn.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: kind=%q", ErrUnknownKind, conn.Kind)
	}

	job := &types.IngestJob{
		TenantID:    tenantID,
		ConnectorID: conn.ID,
		Status:      types.IngestJobQueued,
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("connector: create job: %w", err)
	}

	now := time.Now()
	job.Status = types.IngestJobRunning
	job.StartedAt = &now
	_ = s.jobs.UpdateJob(ctx, job)

	messages, fetchErr := impl.Fetch(ctx, interfaces.ConnectorRuntimeConfig{
		ConnectorID: conn.ID,
		TenantID:    tenantID,
		Kind:        conn.Kind,
		ConfigJSON:  conn.Config,
	})

	if fetchErr != nil {
		job.Status = types.IngestJobFailed
		job.Error = fetchErr.Error()
		finished := time.Now()
		job.FinishedAt = &finished
		_ = s.jobs.UpdateJob(ctx, job)
		_ = s.repo.TouchSync(ctx, conn.ID, finished, fetchErr.Error())
		return job, nil
	}

	count := 0
	for _, m := range messages {
		if s.ingester == nil {
			break
		}
		title := m.Title
		if title == "" {
			title = m.ID
		}
		if err := s.ingester.Ingest(ctx, tenantID, conn.KnowledgeBaseID,
			title, m.Content, m.Author, m.URL, m.Timestamp,
		); err != nil {
			logger.Errorf(ctx, "[Connector] ingest message %s failed: %v", m.ID, err)
			continue
		}
		count++
	}
	finished := time.Now()
	job.ResultCount = count
	job.FinishedAt = &finished
	job.Status = types.IngestJobSucceeded
	_ = s.jobs.UpdateJob(ctx, job)
	_ = s.repo.TouchSync(ctx, conn.ID, finished, "")
	return job, nil
}

// Helper: marshal config to JSON for storage. Exported so handlers
// can validate JSON config at the request boundary.
func MarshalConfig(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
