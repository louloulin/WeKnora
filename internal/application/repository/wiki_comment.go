package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewWikiCommentRepository returns a GORM-backed wiki comment repo.
// The caller (container wiring) holds the only *gorm.DB reference so
// tests can swap a fake repository without rewiring the service.
func NewWikiCommentRepository(db *gorm.DB) interfaces.WikiCommentRepository {
	return &wikiCommentRepository{db: db}
}

type wikiCommentRepository struct {
	db *gorm.DB
}

// Create inserts a new comment row. Sets CreatedAt + UpdatedAt before
// delegating to GORM so the caller doesn't have to remember.
func (r *wikiCommentRepository) Create(ctx context.Context, comment *types.WikiComment) error {
	now := time.Now()
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = now
	}
	comment.UpdatedAt = now
	if err := r.db.WithContext(ctx).Create(comment).Error; err != nil {
		if isWikiCommentDuplicateKey(err) {
			return interfaces.ErrWikiCommentConflict
		}
		return err
	}
	return nil
}

// GetByID returns a single comment by id. Returns (nil, nil) when the
// row is missing; the service translates nil into ErrWikiCommentNotFound.
func (r *wikiCommentRepository) GetByID(ctx context.Context, id string) (*types.WikiComment, error) {
	var c types.WikiComment
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ListByPage returns every comment on a single page, sorted by
// (parent_id, created_at) so parents appear before replies.
func (r *wikiCommentRepository) ListByPage(ctx context.Context, kbID string, slug string) ([]types.WikiComment, error) {
	var comments []types.WikiComment
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND page_slug = ?", kbID, slug).
		Order("COALESCE(parent_id, id) ASC, created_at ASC").
		Find(&comments).Error
	return comments, err
}

// Update applies a body + mentions patch and refreshes UpdatedAt.
func (r *wikiCommentRepository) Update(ctx context.Context, id string, body string, mentionsJSON string) (*types.WikiComment, error) {
	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(&types.WikiComment{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"body":       body,
			"mentions":   mentionsJSON,
			"updated_at": now,
		}).Error; err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// SetResolved toggles resolved_at + resolved_by. Resolved=false clears
// both fields so the row reads as open again.
func (r *wikiCommentRepository) SetResolved(ctx context.Context, id string, resolved bool, resolvedBy string) (*types.WikiComment, error) {
	updates := map[string]interface{}{"updated_at": time.Now()}
	if resolved {
		now := time.Now()
		updates["resolved_at"] = now
		updates["resolved_by"] = resolvedBy
	} else {
		updates["resolved_at"] = nil
		updates["resolved_by"] = ""
	}
	if err := r.db.WithContext(ctx).
		Model(&types.WikiComment{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// Delete removes the row. The FK ON DELETE CASCADE on parent_id handles
// the reply tree automatically.
func (r *wikiCommentRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.WikiComment{}).Error
}

// CountByPage returns the (open, resolved, reply) tally for the stats
// panel. A reply is any row with parent_id != NULL.
func (r *wikiCommentRepository) CountByPage(ctx context.Context, kbID string, slug string) (int, int, int, error) {
	type row struct {
		Open     int
		Resolved int
		Replies  int
	}
	var out row
	err := r.db.WithContext(ctx).
		Raw(`SELECT
			COALESCE(SUM(CASE WHEN resolved_at IS NULL THEN 1 ELSE 0 END), 0) AS open,
			COALESCE(SUM(CASE WHEN resolved_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS resolved,
			COALESCE(SUM(CASE WHEN parent_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS replies
		    FROM wiki_page_comments
		    WHERE knowledge_base_id = ? AND page_slug = ?`,
			kbID, slug).
		Scan(&out).Error
	return out.Open, out.Resolved, out.Replies, err
}

// isWikiCommentDuplicateKey recognises unique-constraint violations on
// the id column so the repo can translate them into
// ErrWikiCommentConflict.
func isWikiCommentDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	// MySQL specific (1052 / 1062)
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		if me.Number == 1062 {
			return true
		}
	}
	msg := err.Error()
	if len(msg) > 0 {
		// SQLite + Postgres simple-string check.
		if containsCI(msg, "duplicate key") || containsCI(msg, "unique constraint") || containsCI(msg, "UNIQUE constraint failed") {
			return true
		}
	}
	return false
}

func wikiCommentContainsCI(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	// Tiny ASCII case-fold; sufficient for these driver messages.
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Compile-time assertions.
var _ driver.Value = (*types.WikiComment)(nil)
var _ interfaces.WikiCommentRepository = (*wikiCommentRepository)(nil)

// MarshalMentions serialises the mention slice into the JSON string
// stored in the mentions column. Empty slices are normalised to "[]".
func MarshalMentions(m []types.WikiCommentMention) (string, error) {
	if m == nil {
		return "[]", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalMentions is the inverse of MarshalMentions.
func UnmarshalMentions(s string) ([]types.WikiCommentMention, error) {
	if s == "" {
		return []types.WikiCommentMention{}, nil
	}
	var out []types.WikiCommentMention
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []types.WikiCommentMention{}
	}
	return out, nil
}
