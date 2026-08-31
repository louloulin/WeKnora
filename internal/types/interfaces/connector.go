package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// IngestConnectorRepository persists AI connector registrations.
type IngestConnectorRepository interface {
	Create(ctx context.Context, c *types.IngestConnector) error
	Update(ctx context.Context, c *types.IngestConnector) error
	Get(ctx context.Context, tenantID string, id uint64) (*types.IngestConnector, error)
	List(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestConnector, int, error)
	Delete(ctx context.Context, tenantID string, id uint64) error
	TouchSync(ctx context.Context, id uint64, lastSyncAt time.Time, lastErr string) error
}

// IngestJobRepository persists sync-job audit rows.
type IngestJobRepository interface {
	Create(ctx context.Context, job *types.IngestJob) error
	UpdateJob(ctx context.Context, job *types.IngestJob) error
	Get(ctx context.Context, id uint64) (*types.IngestJob, error)
	ListByConnector(ctx context.Context, tenantID string, connectorID uint64, limit, offset int) ([]*types.IngestJob, int, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestJob, int, error)
}

// Connector is the runtime implementation behind each registered
// IngestConnector. Implementations live in
// internal/application/service/connector/.
//
// Fetch is called by the IngestService when a connector is
// triggered (manually via API or by the periodic scheduler).
// It must NOT mutate any external state — it only reads. Side
// effects (Slack reactions, read-receipts) are explicitly out of
// scope for v0.7.24 and live behind separate endpoints.
type Connector interface {
	Kind() types.ConnectorKind
	Fetch(ctx context.Context, cfg ConnectorRuntimeConfig) ([]types.ConnectorMessage, error)
}

// ConnectorRuntimeConfig carries everything a Connector needs to
// fetch messages: the parsed config JSON plus identifiers used for
// audit / dedup.
type ConnectorRuntimeConfig struct {
	ConnectorID uint64
	TenantID    string
	Kind        types.ConnectorKind
	ConfigJSON  string // raw JSON column — parser is the Connector's job
}
