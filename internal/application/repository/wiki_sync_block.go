package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// wikiSyncBlockRepository is the SQL-backed implementation of the synced
// block repo. Raw SQL keeps cross-dialect quirks out of the way (JSONB on
// Postgres vs TEXT on SQLite, returning * vs last-insert-rowid).
type wikiSyncBlockRepository struct {
	db *gorm.DB
}

// NewWikiSyncBlockRepository wires the repo to the supplied GORM handle.
func NewWikiSyncBlockRepository(db *gorm.DB) interfaces.WikiSyncBlockRepository {
	return &wikiSyncBlockRepository{db: db}
}

func (r *wikiSyncBlockRepository) Upsert(ctx context.Context, in types.WikiSyncBlockUpsert) (*types.WikiSyncBlock, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("sync block upsert invalid: %w", err)
	}
	dialect := r.db.Dialector.Name()
	contentJSON := []byte(in.ContentJSON)
	if len(contentJSON) == 0 {
		contentJSON = json.RawMessage("{}")
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("begin tx: %w", tx.Error)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	now := time.Now().UTC()
	var id uint64
	var createdAt, updatedAt time.Time

	switch dialect {
	case "sqlite":
		// INSERT OR REPLACE — preserves id, but we manage version manually.
		err := tx.Exec(
			`INSERT OR REPLACE INTO wiki_sync_blocks
			 (tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, COALESCE((SELECT version FROM wiki_sync_blocks WHERE tenant_id=? AND block_id=?), 0)+1, ?, COALESCE((SELECT created_at FROM wiki_sync_blocks WHERE tenant_id=? AND block_id=?), ?), ?)`,
			in.TenantID, in.KBID, in.BlockID, in.Title, string(contentJSON), in.ContentMD,
			in.TenantID, in.BlockID,
			in.OwnerID,
			in.TenantID, in.BlockID, now,
			now,
		).Error
		if err != nil {
			tx.Rollback()
			logger.Errorf(ctx, "sync block upsert failed: tenant=%d block=%s err=%v", in.TenantID, in.BlockID, err)
			return nil, fmt.Errorf("upsert sync block: %w", err)
		}
		// Re-read for accurate id + version.
		var row types.WikiSyncBlock
		if err := tx.Raw(
			`SELECT id, tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at
			 FROM wiki_sync_blocks WHERE tenant_id = ? AND block_id = ?`,
			in.TenantID, in.BlockID,
		).Scan(&row).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("re-read sync block: %w", err)
		}
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("commit sync block: %w", err)
		}
		return &row, nil
	default: // postgres + others
		err := tx.Exec(
			`INSERT INTO wiki_sync_blocks (tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 1, ?, NOW(), NOW())
			 ON CONFLICT (tenant_id, block_id)
			 DO UPDATE SET title = EXCLUDED.title, content_json = EXCLUDED.content_json, content_md = EXCLUDED.content_md,
			               version = wiki_sync_blocks.version + 1, updated_at = NOW()`,
			in.TenantID, in.KBID, in.BlockID, in.Title, string(contentJSON), in.ContentMD, in.OwnerID,
		).Error
		if err != nil {
			tx.Rollback()
			logger.Errorf(ctx, "sync block upsert failed: tenant=%d block=%s err=%v", in.TenantID, in.BlockID, err)
			return nil, fmt.Errorf("upsert sync block: %w", err)
		}
		var row types.WikiSyncBlock
		if err := tx.Raw(
			`SELECT id, tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at
			 FROM wiki_sync_blocks WHERE tenant_id = $1 AND block_id = $2`,
			in.TenantID, in.BlockID,
		).Scan(&row).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("re-read sync block: %w", err)
		}
		id = row.ID
		createdAt = row.CreatedAt
		updatedAt = row.UpdatedAt
		_ = id
		_ = createdAt
		_ = updatedAt
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("commit sync block: %w", err)
		}
		return &row, nil
	}
}

func (r *wikiSyncBlockRepository) Get(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlock, error) {
	var row types.WikiSyncBlock
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at
		 FROM wiki_sync_blocks WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	).Scan(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if row.ID == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("get sync block: %w", err)
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *wikiSyncBlockRepository) List(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.WikiSyncBlock, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.WikiSyncBlock
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, kb_id, block_id, title, content_json, content_md, version, owner_id, created_at, updated_at
		 FROM wiki_sync_blocks WHERE tenant_id = ? AND kb_id = ?
		 ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		tenantID, kbID, limit, offset,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list sync blocks: %w", err)
	}
	return rows, nil
}

func (r *wikiSyncBlockRepository) Delete(ctx context.Context, tenantID uint64, blockID string) error {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM wiki_sync_blocks WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	)
	if res.Error != nil {
		return fmt.Errorf("delete sync block: %w", res.Error)
	}
	return nil
}

func (r *wikiSyncBlockRepository) Stats(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlockRefStats, error) {
	out := &types.WikiSyncBlockRefStats{BlockID: blockID}
	// Read canonical version first — if zero the block doesn't exist.
	var version int64
	if err := r.db.WithContext(ctx).Raw(
		`SELECT version FROM wiki_sync_blocks WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	).Scan(&version).Error; err != nil {
		return nil, nil
	}
	if version == 0 {
		return nil, nil
	}
	out.CurrentVersion = version
	// Ref counts via three cheap SELECTs. Avoid the aggregate-with-CASE
	// form so each query is dialect-portable.
	_ = r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM wiki_sync_block_refs WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	).Scan(&out.RefCount).Error
	_ = r.db.WithContext(ctx).Raw(
		`SELECT COUNT(DISTINCT page_id) FROM wiki_sync_block_refs WHERE tenant_id = ? AND block_id = ?`,
		tenantID, blockID,
	).Scan(&out.PagesCount).Error
	_ = r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM wiki_sync_block_refs WHERE tenant_id = ? AND block_id = ? AND content_version < ?`,
		tenantID, blockID, version,
	).Scan(&out.StaleRefCount).Error
	return out, nil
}

var _ interfaces.WikiSyncBlockRepository = (*wikiSyncBlockRepository)(nil)

// keep imports live for the strconv usage in fallback paths
var _ = strconv.Itoa
