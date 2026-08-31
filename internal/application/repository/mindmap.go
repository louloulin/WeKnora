// Package repository — Build #43 MindMap persistence layer.
//
// Cross-dialect raw SQL (sqlite + postgres + mysql). All write paths go
// through GORM Save / Updates; reads use Where(). The MapID → TenantID
// guard prevents cross-tenant leaks even if the URL is guessed.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// mindMapRepository implements MindMapRepository using GORM.
type mindMapRepository struct {
	db *gorm.DB
}

// NewMindMapRepository wires the repository to the GORM handle.
func NewMindMapRepository(db *gorm.DB) interfaces.MindMapRepository {
	return &mindMapRepository{db: db}
}

// Create persists a new MindMap.
func (r *mindMapRepository) Create(ctx context.Context, m *types.MindMap) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// Get fetches one MindMap scoped to (tenant, id).
func (r *mindMapRepository) Get(ctx context.Context, tenantID uint64, id string) (*types.MindMap, error) {
	var m types.MindMap
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("mindmap get: %w", err)
	}
	return &m, nil
}

// Update applies a partial patch.
func (r *mindMapRepository) Update(ctx context.Context, tenantID uint64, id string, patch types.UpdateMindMapRequest) (*types.MindMap, error) {
	updates := map[string]any{}
	if patch.Title != nil {
		updates["title"] = *patch.Title
	}
	if patch.Layout != nil {
		if !types.ValidMindMapLayouts[*patch.Layout] {
			return nil, types.ErrMindMapInvalid("layout is invalid")
		}
		updates["layout"] = *patch.Layout
	}
	if patch.Theme != nil {
		updates["theme"] = *patch.Theme
	}
	if patch.Visibility != nil {
		updates["visibility"] = *patch.Visibility
	}
	if patch.RootNodeID != nil {
		updates["root_node_id"] = *patch.RootNodeID
	}
	if len(updates) == 0 {
		return r.Get(ctx, tenantID, id)
	}
	res := r.db.WithContext(ctx).
		Model(&types.MindMap{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("mindmap update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Either the row doesn't exist or the patch is identical.
		existing, err := r.Get(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, nil
		}
		return existing, nil
	}
	return r.Get(ctx, tenantID, id)
}

// Delete removes a MindMap and its nodes (transactional).
func (r *mindMapRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete nodes first.
		if err := tx.Where("tenant_id = ? AND map_id = ?", tenantID, id).
			Delete(&types.MindMapNode{}).Error; err != nil {
			return fmt.Errorf("mindmap delete nodes: %w", err)
		}
		// Then the map row.
		res := tx.Where("tenant_id = ? AND id = ?", tenantID, id).
			Delete(&types.MindMap{})
		if res.Error != nil {
			return fmt.Errorf("mindmap delete: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return types.ErrMindMapInvalid("mindmap not found")
		}
		return nil
	})
}

// List returns mindmaps with filters.
func (r *mindMapRepository) List(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) ([]*types.MindMap, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.OwnerUserID != 0 {
		q = q.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	q = q.Order("updated_at DESC").Limit(filter.Limit).Offset(filter.Offset)
	var rows []*types.MindMap
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mindmap list: %w", err)
	}
	return rows, nil
}

// Count returns the count for the same filters as List.
func (r *mindMapRepository) Count(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&types.MindMap{}).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.OwnerUserID != 0 {
		q = q.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, fmt.Errorf("mindmap count: %w", err)
	}
	return n, nil
}

// CreateNode persists a new MindMapNode.
func (r *mindMapRepository) CreateNode(ctx context.Context, n *types.MindMapNode) error {
	if err := n.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(n).Error
}

// GetNode fetches one node scoped to (tenant, map, node).
func (r *mindMapRepository) GetNode(ctx context.Context, tenantID uint64, mapID, nodeID string) (*types.MindMapNode, error) {
	var n types.MindMapNode
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND map_id = ? AND id = ?", tenantID, mapID, nodeID).
		First(&n).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("mindmap_node get: %w", err)
	}
	return &n, nil
}

// UpdateNode applies a partial patch.
func (r *mindMapRepository) UpdateNode(ctx context.Context, tenantID uint64, mapID, nodeID string, patch types.UpdateMindMapNodeRequest) (*types.MindMapNode, error) {
	updates := map[string]any{}
	if patch.ParentID != nil {
		updates["parent_id"] = *patch.ParentID
	}
	if patch.NodeType != nil {
		if !types.ValidMindMapNodeTypes[*patch.NodeType] {
			return nil, types.ErrMindMapInvalid("node_type is invalid")
		}
		updates["node_type"] = *patch.NodeType
	}
	if patch.Label != nil {
		updates["label"] = *patch.Label
	}
	if patch.Body != nil {
		updates["body"] = *patch.Body
	}
	if patch.X != nil {
		updates["x"] = *patch.X
	}
	if patch.Y != nil {
		updates["y"] = *patch.Y
	}
	if patch.Width != nil {
		updates["width"] = *patch.Width
	}
	if patch.Height != nil {
		updates["height"] = *patch.Height
	}
	if patch.Color != nil {
		updates["color"] = *patch.Color
	}
	if patch.Icon != nil {
		updates["icon"] = *patch.Icon
	}
	if patch.DocRef != nil {
		updates["doc_ref"] = *patch.DocRef
	}
	if patch.KBRef != nil {
		updates["kb_ref"] = *patch.KBRef
	}
	if patch.TaskRef != nil {
		updates["task_ref"] = *patch.TaskRef
	}
	if patch.Formula != nil {
		updates["formula"] = *patch.Formula
	}
	if patch.OrderHint != nil {
		updates["order_hint"] = *patch.OrderHint
	}
	if len(updates) == 0 {
		return r.GetNode(ctx, tenantID, mapID, nodeID)
	}
	res := r.db.WithContext(ctx).
		Model(&types.MindMapNode{}).
		Where("tenant_id = ? AND map_id = ? AND id = ?", tenantID, mapID, nodeID).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("mindmap_node update: %w", res.Error)
	}
	return r.GetNode(ctx, tenantID, mapID, nodeID)
}

// DeleteNode removes a single node.
func (r *mindMapRepository) DeleteNode(ctx context.Context, tenantID uint64, mapID, nodeID string) error {
	res := r.db.WithContext(ctx).
		Where("tenant_id = ? AND map_id = ? AND id = ?", tenantID, mapID, nodeID).
		Delete(&types.MindMapNode{})
	if res.Error != nil {
		return fmt.Errorf("mindmap_node delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return types.ErrMindMapInvalid("node not found")
	}
	return nil
}

// ListNodesByMap returns every node for a map, ordered by parent + order_hint.
func (r *mindMapRepository) ListNodesByMap(ctx context.Context, tenantID uint64, mapID string) ([]*types.MindMapNode, error) {
	var rows []*types.MindMapNode
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND map_id = ?", tenantID, mapID).
		Order("parent_id ASC, order_hint ASC, created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("mindmap_node list: %w", err)
	}
	return rows, nil
}

// DeleteByKB removes all maps and nodes for a KB (called from KB delete).
func (r *mindMapRepository) DeleteByKB(ctx context.Context, tenantID uint64, kbID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("tenant_id = ? AND map_id IN (?)",
			tenantID,
			tx.Model(&types.MindMap{}).
				Select("id").
				Where("tenant_id = ? AND kb_id = ?", tenantID, kbID),
		).Delete(&types.MindMapNode{})
		if res.Error != nil {
			return res.Error
		}
		total += res.RowsAffected
		res2 := tx.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
			Delete(&types.MindMap{})
		if res2.Error != nil {
			return res2.Error
		}
		total += res2.RowsAffected
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "[MindMap] DeleteByKB tenant=%d kb=%s err=%v", tenantID, kbID, err)
		return 0, err
	}
	return total, nil
}
