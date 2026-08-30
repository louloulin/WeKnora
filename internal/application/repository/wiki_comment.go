package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// ErrWikiPageCommentNotFound is returned when no comment matches the
// requested id. Handlers translate this to HTTP 404.
var ErrWikiPageCommentNotFound = errors.New("wiki page comment not found")

// wikiCommentRepository is the persistence layer for wiki_page_comments.
// Lives alongside wikiPageRepository and follows the same conventions
// (constructor returns the interface, gorm errors mapped to typed sentinels).
type WikiCommentRepository struct {
	db *gorm.DB
}

// NewWikiCommentRepository wires the wiki-page-comment repository.
func NewWikiCommentRepository(db *gorm.DB) *WikiCommentRepository {
	return &WikiCommentRepository{db: db}
}

// Create persists a new comment. The caller is responsible for setting
// ID, timestamps, and tenant_id before calling.
func (r *WikiCommentRepository) Create(ctx context.Context, c *types.WikiPageComment) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// GetByID fetches a single comment by primary key. Soft-deleted rows
// (deleted_at != NULL) are excluded so deleted comments don't leak into
// the response unless the caller asks for them via GetByIDIncludingDeleted.
func (r *WikiCommentRepository) GetByID(ctx context.Context, id string) (*types.WikiPageComment, error) {
	var c types.WikiPageComment
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWikiPageCommentNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ListByPage returns the visible (non-deleted) comment thread for a
// page, oldest-first so the reader can scan top-down. Soft cap limit
// prevents a malicious client from streaming the whole table.
func (r *WikiCommentRepository) ListByPage(ctx context.Context, pageID string, limit, offset int) ([]*types.WikiPageComment, int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var comments []*types.WikiPageComment
	q := r.db.WithContext(ctx).
		Where("wiki_page_id = ? AND deleted_at IS NULL", pageID).
		Order("created_at ASC").
		Limit(limit).
		Offset(offset)
	if err := q.Find(&comments).Error; err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&types.WikiPageComment{}).
		Where("wiki_page_id = ? AND deleted_at IS NULL", pageID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// Update rewrites body + mentions + updated_at. The handler is
// responsible for permission checks (only author / KB admin can edit).
func (r *WikiCommentRepository) Update(ctx context.Context, c *types.WikiPageComment) error {
	return r.db.WithContext(ctx).
		Model(&types.WikiPageComment{}).
		Where("id = ? AND deleted_at IS NULL", c.ID).
		Updates(map[string]interface{}{
			"body":       c.Body,
			"mentions":   c.Mentions,
			"updated_at": c.UpdatedAt,
		}).Error
}

// SetResolved toggles the resolved_at / resolved_by columns. When
// resolved=false the columns are cleared so the thread reopens.
func (r *WikiCommentRepository) SetResolved(ctx context.Context, id string, resolved bool, resolverID string) error {
	updates := map[string]interface{}{"updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
	if resolved {
		updates["resolved_at"] = gorm.Expr("CURRENT_TIMESTAMP")
		updates["resolved_by"] = resolverID
	} else {
		updates["resolved_at"] = nil
		updates["resolved_by"] = nil
	}
	return r.db.WithContext(ctx).
		Model(&types.WikiPageComment{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates).Error
}

// SoftDelete sets deleted_at = NOW(); the row stays in the table so the
// audit log stays intact (and so a future "show deleted comments" UI can
// re-surface them).
func (r *WikiCommentRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&types.WikiPageComment{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}
