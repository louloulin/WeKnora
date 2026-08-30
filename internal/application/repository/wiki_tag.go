package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewWikiTagRepository returns a GORM-backed WikiTagRepository. The
// caller (container wiring) holds the only *gorm.DB reference so tests
// can swap a fake repository without rewiring the service.
func NewWikiTagRepository(db *gorm.DB) interfaces.WikiTagRepository {
	return &wikiTagRepository{db: db}
}

type wikiTagRepository struct {
	db *gorm.DB
}

// List returns every tag in the KB sorted by (sort_order ASC, name ASC).
func (r *wikiTagRepository) List(ctx context.Context, kbID string) ([]types.WikiTag, error) {
	var tags []types.WikiTag
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", kbID).
		Order("sort_order ASC, name ASC").
		Find(&tags).Error
	return tags, err
}

// ListWithCount runs a LEFT JOIN + GROUP BY so each row carries the
// current page_count. SQLite + Postgres + MySQL all agree on the SQL.
func (r *wikiTagRepository) ListWithCount(ctx context.Context, kbID string) ([]types.WikiTagWithCount, error) {
	var rows []types.WikiTagWithCount
	err := r.db.WithContext(ctx).
		Table("wiki_tags AS t").
		Select("t.*, COUNT(p.wiki_page_id) AS page_count").
		Joins("LEFT JOIN wiki_page_tags AS p ON p.wiki_tag_id = t.id").
		Where("t.knowledge_base_id = ?", kbID).
		Group("t.id").
		Order("t.sort_order ASC, t.name ASC").
		Scan(&rows).Error
	return rows, err
}

// GetByID returns a single tag scoped to the KB. Returns (nil, nil)
// when the row is missing — the service translates nil into
// ErrWikiTagNotFound so the repo does not need to import the sentinel.
func (r *wikiTagRepository) GetByID(ctx context.Context, kbID string, tagID string) (*types.WikiTag, error) {
	var tag types.WikiTag
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND id = ?", kbID, tagID).
		First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tag, nil
}

// Create inserts a new tag. The service pre-validates name and color;
// the repo only translates the DB UNIQUE violation into
// ErrWikiTagConflict so the handler can return 409.
func (r *wikiTagRepository) Create(ctx context.Context, tag *types.WikiTag) error {
	if err := r.db.WithContext(ctx).Create(tag).Error; err != nil {
		if isWikiTagNameConflict(err) {
			return types.ErrWikiTagConflict
		}
		return err
	}
	return nil
}

// isWikiTagNameConflict detects the (knowledge_base_id, name) UNIQUE
// violation across Postgres / MySQL / SQLite. Postgres reports SQLSTATE
// 23505; MySQL reports error 1062; SQLite reports the constraint name
// we declared ("idx_wiki_tags_kb_name"). Checking by string match keeps
// the import surface tiny.
func isWikiTagNameConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "duplicate key value violates unique constraint") {
		return true // postgres
	}
	if strings.Contains(msg, "Duplicate entry") {
		return true // mysql
	}
	if strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "idx_wiki_tags_kb_name") {
		return true // sqlite
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	return false
}

// Update applies the non-nil fields from patch. Only the columns the
// caller pointed at get written. The repo returns the updated row.
func (r *wikiTagRepository) Update(ctx context.Context, kbID string, tagID string, patch types.WikiTagUpdateRequest) (*types.WikiTag, error) {
	updates := map[string]interface{}{}
	if patch.Name != nil {
		updates["name"] = strings.TrimSpace(*patch.Name)
	}
	if patch.Color != nil {
		updates["color"] = *patch.Color
	}
	if patch.SortOrder != nil {
		updates["sort_order"] = *patch.SortOrder
	}
	if len(updates) == 0 {
		return r.GetByID(ctx, kbID, tagID)
	}
	updates["updated_at"] = gorm.Expr("NOW()")
	res := r.db.WithContext(ctx).
		Model(&types.WikiTag{}).
		Where("knowledge_base_id = ? AND id = ?", kbID, tagID).
		Updates(updates)
	if res.Error != nil {
		if isWikiTagNameConflict(res.Error) {
			return nil, types.ErrWikiTagConflict
		}
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, kbID, tagID)
}

// Delete removes the tag definition. wiki_page_tags rows cascade at the
// DB level (ON DELETE CASCADE); the service still calls ClearPageTags
// for pages that are themselves soft-deleted (their wiki_page_id lives
// only in the join table now).
func (r *wikiTagRepository) Delete(ctx context.Context, kbID string, tagID string) error {
	res := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND id = ?", kbID, tagID).
		Delete(&types.WikiTag{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return types.ErrWikiTagNotFound
	}
	return nil
}

// GetPageTags returns the tags attached to a single page. The join is
// (wiki_tags.knowledge_base_id = ?) AND (wiki_page_id = ?) so cross-KB
// leakage is impossible.
func (r *wikiTagRepository) GetPageTags(ctx context.Context, kbID string, slug string) ([]types.WikiTag, error) {
	var tags []types.WikiTag
	err := r.db.WithContext(ctx).
		Table("wiki_tags AS t").
		Joins("INNER JOIN wiki_page_tags AS p ON p.wiki_tag_id = t.id").
		Joins("INNER JOIN wiki_pages AS w ON w.id = p.wiki_page_id").
		Where("t.knowledge_base_id = ? AND w.knowledge_base_id = ? AND w.slug = ?", kbID, kbID, slug).
		Order("t.sort_order ASC, t.name ASC").
		Scan(&tags).Error
	return tags, err
}

// SetPageTags replaces the join rows for one page in a single Tx. The
// slug → page_id lookup uses wiki_pages.knowledge_base_id = kbID so a
// KB-mismatched slug cannot reach the join table.
func (r *wikiTagRepository) SetPageTags(ctx context.Context, kbID string, slug string, tagIDs []string) ([]types.WikiTag, error) {
	var resolved []types.WikiTag
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pageID string
		if err := tx.Raw(
			"SELECT id FROM wiki_pages WHERE knowledge_base_id = ? AND slug = ?",
			kbID, slug,
		).Row().Scan(&pageID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrWikiTagNotFound
			}
			return err
		}
		if err := tx.Exec(
			"DELETE FROM wiki_page_tags WHERE wiki_page_id = ?",
			pageID,
		).Error; err != nil {
			return err
		}
		if len(tagIDs) == 0 {
			return nil
		}
		// Verify every tagID belongs to the same KB. Refuse the whole
		// transaction when even one tag is foreign — keeps SetPageTags
		// all-or-nothing.
		var matched int64
		if err := tx.Model(&types.WikiTag{}).
			Where("knowledge_base_id = ? AND id IN ?", kbID, tagIDs).
			Count(&matched).Error; err != nil {
			return err
		}
		if int(matched) != len(tagIDs) {
			return types.ErrWikiTagNotFound
		}
		rows := make([][]interface{}, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			rows = append(rows, []interface{}{tagID, pageID})
		}
		if err := tx.Exec(
			"INSERT INTO wiki_page_tags (wiki_tag_id, wiki_page_id) VALUES (?, ?)"+
				strings.Repeat(", (?, ?)", len(tagIDs)-1),
			flattenRows(rows)...,
		).Error; err != nil {
			return err
		}
		if err := tx.Table("wiki_tags AS t").
			Joins("INNER JOIN wiki_page_tags AS p ON p.wiki_tag_id = t.id").
			Where("p.wiki_page_id = ?", pageID).
			Order("t.sort_order ASC, t.name ASC").
			Scan(&resolved).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// flattenRows turns [][]T into []T for Exec variadic args.
func flattenRows(rows [][]interface{}) []interface{} {
	out := make([]interface{}, 0, len(rows)*len(rows[0]))
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

// AddTagToPages inserts (tagID, pageID) for every slug in slugs that
// lives in this KB. ON CONFLICT DO NOTHING semantics so re-running the
// same request is idempotent.
func (r *wikiTagRepository) AddTagToPages(ctx context.Context, kbID string, slugs []string, tagID string) ([]string, []types.WikiPageBatchFailure, error) {
	return r.applyBatchToPages(ctx, kbID, slugs, tagID, "add")
}

// RemoveTagFromPages deletes (tagID, pageID) for every slug in slugs
// that lives in this KB. "row absent" is silent success — pages that
// don't carry the tag are reported as succeeded (no row to delete).
func (r *wikiTagRepository) RemoveTagFromPages(ctx context.Context, kbID string, slugs []string, tagID string) ([]string, []types.WikiPageBatchFailure, error) {
	return r.applyBatchToPages(ctx, kbID, slugs, tagID, "remove")
}

// applyBatchToPages is the shared implementation of Add / Remove. op is
// 'add' or 'remove'. Failures are reported with stable codes the
// frontend can render.
func (r *wikiTagRepository) applyBatchToPages(ctx context.Context, kbID string, slugs []string, tagID string, op string) ([]string, []types.WikiPageBatchFailure, error) {
	// Verify tag belongs to this KB up front.
	var tag types.WikiTag
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND id = ?", kbID, tagID).
		First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, types.ErrWikiTagNotFound
		}
		return nil, nil, err
	}
	succeeded := make([]string, 0, len(slugs))
	failed := []types.WikiPageBatchFailure{}
	for _, slug := range slugs {
		var pageID string
		row := r.db.WithContext(ctx).Raw(
			"SELECT id FROM wiki_pages WHERE knowledge_base_id = ? AND slug = ?",
			kbID, slug,
		).Row()
		if err := row.Scan(&pageID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "not_found", Error: "page not found"})
				continue
			}
			failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "internal", Error: err.Error()})
			continue
		}
		switch op {
		case "add":
			// ON CONFLICT DO NOTHING — Postgres / SQLite support the
			// clause natively; MySQL uses INSERT IGNORE.
			err := r.db.WithContext(ctx).Exec(
				insertIgnoreWikiPageTags(tagID, pageID, r.db.Dialector.Name()),
			).Error
			if err != nil {
				failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "internal", Error: err.Error()})
				continue
			}
		case "remove":
			err := r.db.WithContext(ctx).Exec(
				"DELETE FROM wiki_page_tags WHERE wiki_tag_id = ? AND wiki_page_id = ?",
				tagID, pageID,
			).Error
			if err != nil {
				failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "internal", Error: err.Error()})
				continue
			}
		default:
			return nil, nil, types.ErrWikiTagInvalidName // reuse sentinel; service guards earlier
		}
		succeeded = append(succeeded, slug)
	}
	return succeeded, failed, nil
}

// insertIgnoreWikiPageTags returns the dialect-appropriate upsert SQL.
// One helper avoids spreading driver detection across the service.
func insertIgnoreWikiPageTags(tagID, pageID, dialect string) string {
	if isMySQL(dialect) {
		return "INSERT IGNORE INTO wiki_page_tags (wiki_tag_id, wiki_page_id) VALUES ('" + tagID + "','" + pageID + "')"
	}
	return "INSERT INTO wiki_page_tags (wiki_tag_id, wiki_page_id) VALUES ('" + tagID + "','" + pageID + "') ON CONFLICT DO NOTHING"
}

// isMySQL reports whether the dialect driver is MySQL.
func isMySQL(name string) bool {
	return name == "mysql"
}

// ClearPageTags wipes the join rows for one page. Called by the wiki_page
// DeletePage path so soft-deleted pages do not leave orphan rows
// (wiki_page_tags.wiki_page_id has no FK on wiki_pages.id).
func (r *wikiTagRepository) ClearPageTags(ctx context.Context, pageID string) error {
	return r.db.WithContext(ctx).
		Where("wiki_page_id = ?", pageID).
		Delete(&types.WikiPageTag{}).Error
}
