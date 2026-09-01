// Package doc implements the Tencent Docs Word-style document connector. It
// follows the same architectural pattern as feishu/wiki: the heavy lifting
// (streaming sync, checkpointing, resume) lives in a core/engine.go file that
// is a port of feishu/core/engine.go, and this package only ships:
//
//  1. docOps, a NodeOps[core.Document] adapter that bridges Document to the
//     generic engine. The adapter implements list / token / title / edit-time
//     / fetch / log-tag / cursor-encoding methods.
//  2. Connector, the datasource.Connector + datasource.StreamingConnector
//     facade that ParseConfig / NewClient and dispatches to the engine.
//
// Keeping the engine self-contained inside tencentdocs/core (rather than
// refactoring feishu/core to be generic over the client type) means this
// connector picks up the streaming / checkpoint / resume behaviour without
// regressing the Feishu engine. The engine file is a verbatim copy with the
// Feishu-specific identifiers renamed to TencentDocs equivalents - the
// streaming logic itself is unchanged.
package doc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/datasource"
	tdcore "github.com/Tencent/WeKnora/internal/datasource/connector/tencentdocs/core"
	"github.com/Tencent/WeKnora/internal/types"
)

// Compile-time proof that *Connector satisfies both the Connector and
// StreamingConnector interfaces. Catches signature drift immediately at
// build time rather than at container wiring or runtime.
var (
	_ datasource.Connector         = (*Connector)(nil)
	_ datasource.StreamingConnector = (*Connector)(nil)
)

// Connector implements datasource.Connector for Tencent Docs documents
// (doc type). One Connector per region/tenant; the registry in
// initConnectorRegistry instantiates the canonical one.
//
// clientFactory is an optional seam for tests that need to inject a custom
// *http.Client (e.g. an httptest server's client) so they can mock the
// Tencent Docs API without tripping the production SSRF guard. Production
// callers always use NewConnector, which leaves clientFactory nil and
// falls back to tdcore.NewClient (the SSRF-guarded path).
type Connector struct {
	clientFactory func(*tdcore.Config) *tdcore.Client
}

// NewConnector returns a stateless Tencent Docs document connector. Each
// data source supplies its own credentials at call time via the DataSourceConfig.
func NewConnector() *Connector { return &Connector{} }

// newClient constructs the per-data-source *tdcore.Client, honouring any
// test-injected factory before falling back to the SSRF-guarded default.
func (c *Connector) newClient(cfg *tdcore.Config) *tdcore.Client {
	if c.clientFactory != nil {
		return c.clientFactory(cfg)
	}
	return tdcore.NewClient(cfg)
}

// Type returns the connector type identifier.
func (c *Connector) Type() string { return tdcore.ConnectorTypeTencentDocs }

// Validate verifies the credentials by issuing a lightweight drive listing.
func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	cfg, err := tdcore.ParseConfig(config)
	if err != nil {
		return err
	}
	cli := c.newClient(cfg)
	if err := cli.Ping(ctx); err != nil {
		return fmt.Errorf("tencent_docs connection failed: %w", err)
	}
	return nil
}

// ListResources lists the Tencent Docs resources the picker can show. The
// picker is intentionally flat for v1 (no nested folders) - users select one
// or more document IDs from the drive listing. A non-empty parentID returns
// an empty slice (no children): parentID is reserved for a future folder mode
// without changing the signature now.
func (c *Connector) ListResources(
	ctx context.Context, config *types.DataSourceConfig, parentID string,
) ([]types.Resource, error) {
	if parentID != "" {
		return []types.Resource{}, nil
	}

	cfg, err := tdcore.ParseConfig(config)
	if err != nil {
		return nil, err
	}
	cli := c.newClient(cfg)

	docs, err := cli.ListAllDriveFiles(ctx, 200)
	if err != nil {
		return nil, fmt.Errorf("list tencent_docs files: %w", err)
	}

	resources := make([]types.Resource, 0, len(docs))
	for _, d := range docs {
		resources = append(resources, types.Resource{
			ExternalID:  d.ID,
			Name:        d.Title,
			Type:        d.Type,
			URL:         tdcore.WebDocURL(d.ID),
			HasChildren: false,
			Metadata: map[string]interface{}{
				"doc_id":     d.ID,
				"doc_type":   d.Type,
				"updated_at": d.UpdatedAt,
				"owner":      d.Owner,
				"size_bytes": d.SizeBytes,
				"permission": d.Permission,
			},
		})
	}
	return resources, nil
}

// ResolveResourceAncestors has nothing to do for Tencent Docs in v1: the
// picker is flat so a selection has no ancestors to reveal. The method is
// still implemented to satisfy the Connector interface.
func (c *Connector) ResolveResourceAncestors(
	ctx context.Context, config *types.DataSourceConfig, resourceIDs []string,
) ([]string, error) {
	return []string{}, nil
}

// FetchAll is the defensive full-sync path - the service prefers FetchStream
// when the connector implements StreamingConnector. Routed through the same
// tencentdocs/core engine so the cursor-on-failure (#2136) semantics apply
// here too.
func (c *Connector) FetchAll(
	ctx context.Context, config *types.DataSourceConfig, resourceIDs []string,
) ([]types.FetchedItem, error) {
	cfg, err := tdcore.ParseConfig(config)
	if err != nil {
		return nil, err
	}
	cli := c.newClient(cfg)
	return tdcore.FetchAllEngine(ctx, cli, config, resourceIDs, docOps{})
}

// FetchIncremental is the defensive incremental-sync path. Routed through the
// same tencentdocs/core engine so the incremental fast-path (skip nodes
// whose recorded edit time is unchanged) applies here too.
func (c *Connector) FetchIncremental(
	ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor,
) ([]types.FetchedItem, *types.SyncCursor, error) {
	cfg, err := tdcore.ParseConfig(config)
	if err != nil {
		return nil, nil, err
	}
	cli := c.newClient(cfg)
	ops := docOps{}
	if len(config.ResourceIDs) == 0 {
		return nil, nil, errors.New(ops.EmptyResourceIDsError())
	}
	return tdcore.FetchIncrementalEngine(ctx, cli, config, cursor, ops)
}

// FetchStream performs a resumable, memory-bounded sync. The per-document
// loop lives in the shared engine (tencentdocs/core/engine.go); this shell
// only wires the doc NodeOps adapter.
func (c *Connector) FetchStream(
	ctx context.Context, config *types.DataSourceConfig,
	cursor *types.SyncCursor, h datasource.StreamHandler,
) (*types.SyncCursor, error) {
	cfg, err := tdcore.ParseConfig(config)
	if err != nil {
		return nil, err
	}
	cli := c.newClient(cfg)
	ops := docOps{}
	if len(config.ResourceIDs) == 0 {
		return nil, errors.New(ops.EmptyResourceIDsError())
	}
	return tdcore.FetchStreamEngine(ctx, cli, config, cursor, h, ops)
}

// docOps adapts the doc Connector to the shared sync engine. It is
// stateless and encodes/decodes the Tencent Docs cursor wire format so the
// engine can stay format-agnostic. The engine imports tdcore for
// FetchStreamEngine etc.; the docOps type here only needs to satisfy
// tdcore.NodeOps[tdcore.Document].
type docOps struct{}

// List resolves a resourceID (drive folder ID for the picker, or a single
// document ID for incremental) to the documents it contains. Tencent Docs
// exposes a /drive/v2/files endpoint that lists documents by type and parent.
func (o docOps) List(ctx context.Context, client *tdcore.Client, resourceID string) ([]tdcore.Document, error, error) {
	if tdcore.IsSingleDocumentID(resourceID) {
		d, err := client.GetDocument(ctx, resourceID)
		if err != nil {
			return nil, nil, err
		}
		return []tdcore.Document{d}, nil, nil
	}
	docs, err := client.ListAllDriveFiles(ctx, 200)
	if err != nil {
		return nil, nil, err
	}
	return docs, nil, nil
}

func (o docOps) Token(d tdcore.Document) string   { return d.ID }
func (o docOps) Title(d tdcore.Document) string   { return d.Title }
func (o docOps) ObjType(d tdcore.Document) string { return d.Type }

// EditTime is the change-detection timestamp: drives the cursor comparison,
// NOT FetchedItem.UpdatedAt. Empty when the document has no recorded
// update time so the engine treats it as "changed" on first sync.
func (o docOps) EditTime(d tdcore.Document) string { return d.EditTime() }

// Fetch retrieves one document's content and converts it to a slice of
// FetchedItems. For text-friendly doc types it returns a single Markdown
// item; for binary types it returns the raw bytes wrapped as an attachment.
func (o docOps) Fetch(ctx context.Context, client *tdcore.Client, d tdcore.Document, resourceID string, multimodal bool) ([]*types.FetchedItem, error) {
	content, contentType, err := client.FetchAnyContent(ctx, d)
	if err != nil {
		return nil, err
	}

	item := &types.FetchedItem{
		ExternalID:       d.ID,
		Title:            d.Title,
		SourceResourceID: resourceID,
		Content:          []byte(content),
		ContentType:      contentType,
		URL:              tdcore.WebDocURL(d.ID),
		UpdatedAt:        d.UpdatedAt,
		Metadata: map[string]string{
			"doc_id":   d.ID,
			"doc_type": d.Type,
			"owner":    d.Owner,
		},
	}
	return []*types.FetchedItem{item}, nil
}

func (o docOps) ListFailureItems(resourceID string, partial error) []types.FetchedItem {
	// Tencent Docs ListDriveFilesRecursive is a single page - there is no
	// partial-listing failure mode today. Return nil so the engine treats
	// any non-nil partial as no-op. If we later add a recursive walk with
	// per-folder sub-requests, surface those failures here.
	return nil
}

func (o docOps) ResourceNoun() string { return "documents" }

func (o docOps) EmptyResourceIDsError() string {
	return "no resource IDs (tencent_docs document IDs) configured"
}

func (o docOps) LogTag() string { return "[TencentDocs]" }

func (o docOps) DecodeCursorTimes(m map[string]interface{}) map[string]map[string]string {
	return tdcore.DecodeCursorTimes(m)
}

func (o docOps) EncodeCursor(times map[string]map[string]string, lastSync time.Time) *types.SyncCursor {
	return tdcore.EncodeCursor(times, lastSync)
}
