package repository

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// wikiSyncBlockRefRepository tracks embedded synced-block references.
//
// Refs are denormalized from page content's [[sync:UUID]] markers; on
// every page save the service rewrites this table to reflect the current
// set of embedded blocks for the page.
type wikiSyncBlockRefRepository struct {
	db *gorm.DB
}

// NewWikiSyncBlockRefRepository wires the ref repo to the supplied handle.
func NewWikiSyncBlockRefRepository(db *gorm.DB) interfaces.WikiSyncBlockRefRepository {
	return &wikiSyncBlockRefRepository{db: db}
}

func (r *wikiSyncBlockRefRepository) Upsert(ctx context.Context, ref *types.WikiSyncBlockRef) error {
	if ref == nil || ref.TenantID == 0 || ref.BlockID == "" || ref.PageID == "" {
		return fmt.Errorf("sync block ref upsert: missing required fields")
	}
	if ref.AnchorSlug == "" {
		ref.AnchorSlug = ""
	}
	dialect := r.db.Dialector.Name()
	switch dialect {
	case "sqlite":
		err := r.db.WithContext(ctx).Exec(
			`INSERT OR REPLACE INTO wiki_sync_block_refs
			 (tenant_id, kb_id, block_id, page_id, anchor_slug, content_version, rendered_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, COALESCE((SELECT created_at FROM wiki_sync_block_refs WHERE tenant_id=? AND block_id=? AND page_id=? AND anchor_slug=?), CURRENT_TIMESTAMP))`,
			ref.TenantID, ref.KBID, ref.BlockID, ref.PageID, ref.AnchorSlug, ref.ContentVersion,
			ref.TenantID, ref.BlockID, ref.PageID, ref.AnchorSlug,
		).Error
		if err != nil {
			logger.Errorf(ctx, "sync block ref upsert failed: tenant=%d block=%s page=%s err=%v",
				ref.TenantID, ref.BlockID, ref.PageID, err)
			return fmt.Errorf("upsert sync block ref: %w", err)
		}
	default:
		err := r.db.WithContext(ctx).Exec(
			`INSERT INTO wiki_sync_block_refs (tenant_id, kb_id, block_id, page_id, anchor_slug, content_version, rendered_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
			 ON CONFLICT (tenant_id, block_id, page_id, anchor_slug)
			 DO UPDATE SET content_version = EXCLUDED.content_version, rendered_at = NOW()`,
			ref.TenantID, ref.KBID, ref.BlockID, ref.PageID, ref.AnchorSlug, ref.ContentVersion,
		).Error
		if err != nil {
			logger.Errorf(ctx, "sync block ref upsert failed: tenant=%d block=%s page=%s err=%v",
				ref.TenantID, ref.BlockID, ref.PageID, err)
			return fmt.Errorf("upsert sync block ref: %w", err)
		}
	}
	return nil
}

func (r *wikiSyncBlockRefRepository) ListByBlock(ctx context.Context, tenantID uint64, blockID string) ([]*types.WikiSyncBlockRef, error) {
	var rows []*types.WikiSyncBlockRef
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, kb_id, block_id, page_id, anchor_slug, content_version, rendered_at, created_at
		 FROM wiki_sync_block_refs WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list by block: %w", err)
	}
	return rows, nil
}

func (r *wikiSyncBlockRefRepository) ListByPage(ctx context.Context, tenantID uint64, pageID string) ([]*types.WikiSyncBlockRef, error) {
	var rows []*types.WikiSyncBlockRef
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, kb_id, block_id, page_id, anchor_slug, content_version, rendered_at, created_at
		 FROM wiki_sync_block_refs WHERE tenant_id = ? AND page_id = ?`,
		tenantID, pageID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list by page: %w", err)
	}
	return rows, nil
}

func (r *wikiSyncBlockRefRepository) DeleteByPage(ctx context.Context, tenantID uint64, pageID string) error {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM wiki_sync_block_refs WHERE tenant_id = ? AND page_id = ?`,
		tenantID, pageID,
	)
	if res.Error != nil {
		return fmt.Errorf("delete by page: %w", res.Error)
	}
	return nil
}

func (r *wikiSyncBlockRefRepository) DeleteByBlock(ctx context.Context, tenantID uint64, blockID string) error {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM wiki_sync_block_refs WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	)
	if res.Error != nil {
		return fmt.Errorf("delete by block: %w", res.Error)
	}
	return nil
}

func (r *wikiSyncBlockRefRepository) MarkRendered(ctx context.Context, tenantID uint64, blockID, pageID, anchorSlug string, version int64) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE wiki_sync_block_refs SET content_version = ?, rendered_at = CURRENT_TIMESTAMP
		 WHERE tenant_id = ? AND block_id = ? AND page_id = ? AND anchor_slug = ?`,
		version, tenantID, blockID, pageID, anchorSlug,
	)
	if res.Error != nil {
		return fmt.Errorf("mark rendered: %w", res.Error)
	}
	return nil
}

var _ interfaces.WikiSyncBlockRefRepository = (*wikiSyncBlockRefRepository)(nil)
