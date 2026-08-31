// Package repository — v0.7.25 collab_doc implementation.
//
// The repo trio mirrors the wiki_realtime trio but keyed on (tenant, doc_id)
// rather than (tenant, kb_id, page_id). The metadata repo is new: wiki pages
// are addressed by URL, collab docs are addressed by UUID with explicit kind.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type collabDocRepository struct {
	db *gorm.DB
}

// NewCollabDocRepository wires the metadata repo to the supplied GORM handle.
func NewCollabDocRepository(db *gorm.DB) interfaces.CollabDocRepository {
	return &collabDocRepository{db: db}
}

func (r *collabDocRepository) Create(ctx context.Context, d *types.CollaborativeDoc) error {
	if d.ID == "" {
		return types.ErrCollabDocInvalid("id is required")
	}
	if !types.ValidCollaborativeDocKinds[d.DocKind] {
		return types.ErrCollabDocInvalid("doc_kind is invalid")
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	d.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *collabDocRepository) Get(ctx context.Context, tenantID uint64, id string) (*types.CollaborativeDoc, error) {
	var d types.CollaborativeDoc
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("collab_doc get: %w", err)
	}
	return &d, nil
}

func (r *collabDocRepository) Update(ctx context.Context, d *types.CollaborativeDoc) error {
	d.UpdatedAt = time.Now().UTC()
	res := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", d.TenantID, d.ID).
		Updates(map[string]interface{}{
			"title":       d.Title,
			"visibility":  d.Visibility,
			"share_token": d.ShareToken,
			"updated_at":  d.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return types.ErrCollabDocInvalid("collab doc not found")
	}
	return nil
}

func (r *collabDocRepository) Archive(ctx context.Context, tenantID uint64, id string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&types.CollaborativeDoc{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{"archived_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return types.ErrCollabDocInvalid("collab doc not found")
	}
	return nil
}

func (r *collabDocRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.CollaborativeDoc{}).Error
}

func (r *collabDocRepository) List(ctx context.Context, tenantID uint64, filter types.ListCollaborativeDocsFilter) ([]*types.CollaborativeDoc, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.DocKind != "" {
		q = q.Where("doc_kind = ?", filter.DocKind)
	}
	if filter.Archived {
		q = q.Where("archived_at IS NOT NULL")
	} else {
		q = q.Where("archived_at IS NULL")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q = q.Order("updated_at DESC").Limit(limit).Offset(filter.Offset)
	var out []*types.CollaborativeDoc
	if err := q.Find(&out).Error; err != nil {
		return nil, fmt.Errorf("collab_doc list: %w", err)
	}
	return out, nil
}

func (r *collabDocRepository) Count(ctx context.Context, tenantID uint64, filter types.ListCollaborativeDocsFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&types.CollaborativeDoc{}).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.DocKind != "" {
		q = q.Where("doc_kind = ?", filter.DocKind)
	}
	if filter.Archived {
		q = q.Where("archived_at IS NOT NULL")
	} else {
		q = q.Where("archived_at IS NULL")
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

var _ interfaces.CollabDocRepository = (*collabDocRepository)(nil)

// -----------------------------------------------------------------------------
// Snapshot repo

type collabDocSnapshotRepository struct {
	db *gorm.DB
}

// NewCollabDocSnapshotRepository wires the snapshot repo.
func NewCollabDocSnapshotRepository(db *gorm.DB) interfaces.CollabDocSnapshotRepository {
	return &collabDocSnapshotRepository{db: db}
}

func (r *collabDocSnapshotRepository) Upsert(ctx context.Context, in types.CollabDocSnapshotUpsert) (*types.CollabDocSnapshot, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("collab doc snapshot upsert invalid: %w", err)
	}
	_ = in // values are inlined below; the row struct is kept as documentation
	dialect := r.db.Dialector.Name()
	switch dialect {
	case "sqlite":
		err := r.db.WithContext(ctx).Exec(
			`INSERT OR REPLACE INTO collab_doc_snapshots
			 (tenant_id, doc_id, doc_kind, schema_version, ydoc_state, vector_clock, version, size_bytes, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, COALESCE((SELECT version FROM collab_doc_snapshots WHERE tenant_id=? AND doc_id=?), 0)+1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			in.TenantID, in.DocID, in.DocKind, in.SchemaVersion, in.YDocState, in.VectorClock,
			in.TenantID, in.DocID,
			in.SizeBytes,
		).Error
		if err != nil {
			return nil, fmt.Errorf("collab doc snapshot upsert (sqlite): %w", err)
		}
	default: // postgres + others
		err := r.db.WithContext(ctx).Exec(
			`INSERT INTO collab_doc_snapshots
			 (tenant_id, doc_id, doc_kind, schema_version, ydoc_state, vector_clock, version, size_bytes)
			 VALUES (?, ?, ?, ?, ?, ?, 1, ?)
			 ON CONFLICT (tenant_id, doc_id) DO UPDATE SET
			   doc_kind = EXCLUDED.doc_kind,
			   schema_version = EXCLUDED.schema_version,
			   ydoc_state = EXCLUDED.ydoc_state,
			   vector_clock = EXCLUDED.vector_clock,
			   size_bytes = EXCLUDED.size_bytes,
			   version = collab_doc_snapshots.version + 1`,
			in.TenantID, in.DocID, in.DocKind, in.SchemaVersion, in.YDocState, in.VectorClock, in.SizeBytes,
		).Error
		if err != nil {
			return nil, fmt.Errorf("collab doc snapshot upsert (pg): %w", err)
		}
	}
	return r.Get(ctx, in.TenantID, in.DocID)
}

func (r *collabDocSnapshotRepository) Get(ctx context.Context, tenantID uint64, docID string) (*types.CollabDocSnapshot, error) {
	var row types.CollabDocSnapshot
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ?", tenantID, docID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("collab doc snapshot get: %w", err)
	}
	return &row, nil
}

func (r *collabDocSnapshotRepository) Delete(ctx context.Context, tenantID uint64, docID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ?", tenantID, docID).
		Delete(&types.CollabDocSnapshot{}).Error
}

var _ interfaces.CollabDocSnapshotRepository = (*collabDocSnapshotRepository)(nil)

// -----------------------------------------------------------------------------
// Session repo

type collabDocSessionRepository struct {
	db *gorm.DB
}

// NewCollabDocSessionRepository wires the session repo.
func NewCollabDocSessionRepository(db *gorm.DB) interfaces.CollabDocSessionRepository {
	return &collabDocSessionRepository{db: db}
}

func (r *collabDocSessionRepository) Upsert(ctx context.Context, s *types.CollabDocSession) error {
	if s.ID == "" {
		return types.ErrCollabDocInvalid("session id is required")
	}
	if s.LastHeartbeat.IsZero() {
		s.LastHeartbeat = time.Now().UTC()
	}
	if s.JoinedAt.IsZero() {
		s.JoinedAt = s.LastHeartbeat
	}
	dialect := r.db.Dialector.Name()
	switch dialect {
	case "sqlite":
		return r.db.WithContext(ctx).Exec(
			`INSERT OR REPLACE INTO collab_doc_sessions
			 (id, tenant_id, doc_id, user_id, client_id, color, display_name, last_heartbeat, joined_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.TenantID, s.DocID, s.UserID, s.ClientID, s.Color, s.DisplayName, s.LastHeartbeat, s.JoinedAt,
		).Error
	default:
		return r.db.WithContext(ctx).Exec(
			`INSERT INTO collab_doc_sessions
			 (id, tenant_id, doc_id, user_id, client_id, color, display_name, last_heartbeat, joined_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (tenant_id, doc_id, client_id) DO UPDATE SET
			   color = EXCLUDED.color,
			   display_name = EXCLUDED.display_name,
			   last_heartbeat = EXCLUDED.last_heartbeat`,
			s.ID, s.TenantID, s.DocID, s.UserID, s.ClientID, s.Color, s.DisplayName, s.LastHeartbeat, s.JoinedAt,
		).Error
	}
}

func (r *collabDocSessionRepository) ListByDoc(ctx context.Context, tenantID uint64, docID string, since time.Time) ([]*types.CollabDocSession, error) {
	var out []*types.CollabDocSession
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ? AND last_heartbeat >= ?", tenantID, docID, since).
		Order("last_heartbeat DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *collabDocSessionRepository) DeleteByClient(ctx context.Context, tenantID uint64, docID string, clientID uint64) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ? AND client_id = ?", tenantID, docID, clientID).
		Delete(&types.CollabDocSession{}).Error
}

func (r *collabDocSessionRepository) SweepOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("last_heartbeat < ?", cutoff).
		Delete(&types.CollabDocSession{})
	if res.Error != nil {
		return 0, res.Error
	}
	if r := res.RowsAffected; r > 0 {
		logger.Infof(ctx, "[CollabDoc] swept %d stale presence rows", r)
	}
	return res.RowsAffected, nil
}

var _ interfaces.CollabDocSessionRepository = (*collabDocSessionRepository)(nil)

// collabDocFileRepository persists .docx/.pptx/.xlsx byte payloads keyed
// by (tenant, doc_id). One row per save; the row with the highest version
// is the canonical "open this doc" target. Mirrors the wiki realtime
// snapshot repo for the binary side-channel.
type collabDocFileRepository struct {
	db *gorm.DB
}

// NewCollabDocFileRepository wires the binary repo.
func NewCollabDocFileRepository(db *gorm.DB) interfaces.CollabDocFileRepository {
	return &collabDocFileRepository{db: db}
}

func (r *collabDocFileRepository) SaveFile(ctx context.Context, in types.CollabDocFileUpsert) (*types.CollabDocFile, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	row := &types.CollabDocFile{
		TenantID:  in.TenantID,
		DocID:     in.DocID,
		Format:    in.Format,
		Content:   in.Content,
		SizeBytes: len(in.Content),
		Version:   in.Version,
		CreatedAt: time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("collab_doc_file save: %w", err)
	}
	return row, nil
}

func (r *collabDocFileRepository) GetLatestFile(ctx context.Context, tenantID uint64, docID string) (*types.CollabDocFile, error) {
	var f types.CollabDocFile
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ?", tenantID, docID).
		Order("version DESC").
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("collab_doc_file latest: %w", err)
	}
	return &f, nil
}

func (r *collabDocFileRepository) GetFileByVersion(ctx context.Context, tenantID uint64, docID string, version int) (*types.CollabDocFile, error) {
	var f types.CollabDocFile
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ? AND version = ?", tenantID, docID, version).
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("collab_doc_file by_version: %w", err)
	}
	return &f, nil
}

func (r *collabDocFileRepository) CurrentVersion(ctx context.Context, tenantID uint64, docID string) (int, error) {
	var max sql.NullInt64
	err := r.db.WithContext(ctx).
		Model(&types.CollabDocFile{}).
		Where("tenant_id = ? AND doc_id = ?", tenantID, docID).
		Select("MAX(version)").
		Scan(&max).Error
	if err != nil {
		return 0, fmt.Errorf("collab_doc_file current_version: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64), nil
}

func (r *collabDocFileRepository) DeleteByDoc(ctx context.Context, tenantID uint64, docID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND doc_id = ?", tenantID, docID).
		Delete(&types.CollabDocFile{}).Error
}
