// Package interfaces — MindMap repository interfaces.
//
// The repository layer hides the dialect (sqlite / postgres / mysql) and
// returns typed MindMap / MindMapNode values. The service layer composes
// nodes into export shapes (Markdown / OPML / .xmind) and runs the
// auto-layout algorithm.
package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// MindMapRepository persists MindMap aggregates.
type MindMapRepository interface {
	Create(ctx context.Context, m *types.MindMap) error
	Get(ctx context.Context, tenantID uint64, id string) (*types.MindMap, error)
	Update(ctx context.Context, tenantID uint64, id string, patch types.UpdateMindMapRequest) (*types.MindMap, error)
	Delete(ctx context.Context, tenantID uint64, id string) error
	List(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) ([]*types.MindMap, error)
	Count(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) (int64, error)

	// Nodes
	CreateNode(ctx context.Context, n *types.MindMapNode) error
	GetNode(ctx context.Context, tenantID uint64, mapID, nodeID string) (*types.MindMapNode, error)
	UpdateNode(ctx context.Context, tenantID uint64, mapID, nodeID string, patch types.UpdateMindMapNodeRequest) (*types.MindMapNode, error)
	DeleteNode(ctx context.Context, tenantID uint64, mapID, nodeID string) error
	ListNodesByMap(ctx context.Context, tenantID uint64, mapID string) ([]*types.MindMapNode, error)

	// DeleteByKB removes all mindmaps + nodes for a KB (used on KB delete).
	DeleteByKB(ctx context.Context, tenantID uint64, kbID string) (int64, error)
}

// Ensure interface compliance at compile time.
var _ MindMapRepository = (MindMapRepository)(nil)

// MindMapAuditLog is a lightweight audit row for governance (#46 compatibility).
type MindMapAuditLog struct {
	ID        uint64    `json:"id"`
	TenantID  uint64    `json:"tenant_id"`
	MapID     string    `json:"map_id"`
	UserID    uint64    `json:"user_id"`
	Action    string    `json:"action"` // create / update / delete / export
	Detail    string    `json:"detail"`
	Timestamp time.Time `json:"timestamp"`
}
