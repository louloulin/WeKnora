package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// wikiAclRepository implements the storage surface WikiAclService needs.
// The matching interface lives in service.WikiAclRepo; we let Go's
// structural typing enforce the match at compile time so repository can
// stay free of any service-package import (and vice versa).
type wikiAclRepository struct {
	db *gorm.DB
}

// NewWikiAclRepository wires the production ACL storage. The returned
// concrete type satisfies service.WikiAclRepo; the DI container wires it
// in directly.
func NewWikiAclRepository(db *gorm.DB) *wikiAclRepository {
	return &wikiAclRepository{db: db}
}

// aclColumnProjection returns the SELECT list used by both GetAclBySlug
// and the revision-check inside UpdateAclWithRevision. Centralized so the
// two queries stay structurally aligned.
const aclColumnProjection = "acl, acl_revision"

// aclRow mirrors the projected columns. Stays private: the repo never
// returns this struct to callers — service-layer methods receive the
// typed *types.WikiPageAcl directly.
type aclRow struct {
	ACL         types.WikiPageAcl `gorm:"column:acl;type:json"`
	ACLRevision int64             `gorm:"column:acl_revision"`
}

// GetAclBySlug returns the per-page ACL row for (kbID, slug). Returns
// (nil, nil) when the page exists but the ACL column is NULL (legacy
// inherit) — the service layer normalizes NULL to inherit/rev=0.
//
// Both the JSON payload and the sibling acl_revision column are read in a
// single SELECT so callers see the same revision the next writer will
// compare against.
func (r *wikiAclRepository) GetAclBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error) {
	var row aclRow
	scanErr := r.db.WithContext(ctx).
		Table("wiki_pages").
		Select(aclColumnProjection).
		Where("knowledge_base_id = ? AND slug = ? AND deleted_at IS NULL", kbID, slug).
		Scan(&row).Error
	if scanErr != nil {
		if isNoRows(scanErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("wiki_acl repo: get acl: %w", scanErr)
	}
	// Empty ACL row (JSON column was NULL on the page) → legacy inherit.
	// The JSON column serializes an empty object as "{}"; the only way
	// we observe mode="" is when the column itself was NULL. Treat as
	// "no row" so the service layer normalizes to inherit/rev=0.
	if row.ACL.Mode == "" {
		return nil, nil
	}
	row.ACL.Revision = row.ACLRevision
	return &row.ACL, nil
}

// UpdateAclWithRevision atomically replaces the ACL after verifying the
// stored revision matches expectedRevision. Returns
// types.ErrWikiPageAclRevisionConflict on mismatch.
//
// The audit row is written in the same transaction so forensics never drift
// from the live ACL row even if the process crashes between the two
// writes.
func (r *wikiAclRepository) UpdateAclWithRevision(
	ctx context.Context,
	kbID string, slug string,
	newAcl types.WikiPageAcl,
	expectedRevision int64,
	actorUserID string, actorRole string, action string,
) (*types.WikiPageAcl, error) {
	var (
		updated *types.WikiPageAcl
		opErr   error
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Capture the before-state + current revision under the
		// transaction so the optimistic lock and the audit row see the
		// same snapshot.
		var before types.WikiPageAcl
		var beforeHadValue bool
		var beforeRevision int64
		row := tx.Table("wiki_pages").
			Select("acl, acl_revision").
			Where("knowledge_base_id = ? AND slug = ? AND deleted_at IS NULL", kbID, slug).
			Row()
		var beforeRaw []byte
		if scanErr := row.Scan(&beforeRaw, &beforeRevision); scanErr != nil {
			if isNoRows(scanErr) {
				return types.ErrWikiPageAclRevisionConflict
			}
			return fmt.Errorf("wiki_acl repo: lock acl: %w", scanErr)
		}
		if len(beforeRaw) > 0 {
			if scanErr := before.Scan(beforeRaw); scanErr != nil {
				return fmt.Errorf("wiki_acl repo: scan before: %w", scanErr)
			}
			beforeHadValue = true
		}
		if beforeRevision != expectedRevision {
			return types.ErrWikiPageAclRevisionConflict
		}

		nextRevision := beforeRevision + 1
		now := time.Now().UTC()
		newAcl.Revision = nextRevision
		newAcl.UpdatedAt = now.Format(time.RFC3339)

		// GORM treats JSON columns as opaque text, so we marshal once and
		// pass the []byte through gorm.Expr("?::json", ...) for pg +
		// ? for sqlite — the plain assignment works on both backends.
		aclJSON, marshalErr := newAcl.Value()
		if marshalErr != nil {
			return fmt.Errorf("wiki_acl repo: marshal acl: %w", marshalErr)
		}
		aclBytes, ok := aclJSON.([]byte)
		if !ok {
			return fmt.Errorf("wiki_acl repo: expected []byte from acl.Value, got %T", aclJSON)
		}

		// Cast to ::json on PG so the implicit text→json cast is explicit.
		// SQLite ignores the cast and stores the raw bytes into the TEXT
		// column underneath.
		updateRes := tx.Table("wiki_pages").
			Where("knowledge_base_id = ? AND slug = ? AND acl_revision = ?", kbID, slug, beforeRevision).
			Updates(map[string]any{
				"acl":          gorm.Expr("?::json", string(aclBytes)),
				"acl_revision": nextRevision,
				"updated_at":   now,
			})
		if updateRes.Error != nil {
			return fmt.Errorf("wiki_acl repo: write acl: %w", updateRes.Error)
		}
		if updateRes.RowsAffected == 0 {
			// Another writer won the race between SELECT and UPDATE;
			// same shape as a revision mismatch from the caller's view.
			return types.ErrWikiPageAclRevisionConflict
		}

		// Build the audit row. The before_acl column is NULL when the
		// legacy row had no prior ACL value (beforeHadValue=false),
		// matching the on-disk shape and giving reviewers a real "first
		// write" signal in the audit trail.
		auditFields := map[string]any{
			"wiki_page_id":      nil, // legacy pages have UUID id, not bigint — leave NULL
			"knowledge_base_id": kbID,
			"slug":              slug,
			"actor_user_id":     actorUserID,
			"actor_role":        actorRole,
			"action":            action,
			"created_at":        now,
		}
		if beforeHadValue {
			beforeJSON, _ := before.Value()
			if beforeBytes, ok := beforeJSON.([]byte); ok {
				auditFields["before_acl"] = gorm.Expr("?::jsonb", string(beforeBytes))
			}
		}
		afterJSON, _ := newAcl.Value()
		if afterBytes, ok := afterJSON.([]byte); ok {
			auditFields["after_acl"] = gorm.Expr("?::jsonb", string(afterBytes))
		}

		auditInsert := tx.Table("wiki_page_acl_audit").Insert(auditFields)
		if auditInsert.Error != nil {
			return fmt.Errorf("wiki_acl repo: write audit: %w", auditInsert.Error)
		}

		updated = &newAcl
		return nil
	})
	if err != nil {
		opErr = err
	}
	if opErr != nil {
		return nil, opErr
	}
	return updated, nil
}

// PageOwnerAndAdmin returns the page owner's user id and whether the
// caller is a KB-level admin. "Owner" is currently sourced from
// knowledge_bases.creator_id (every wiki page inherits the owning user of
// its KB); the wiki_pages table itself has no per-page owner column.
//
// Admin is resolved from the in-context TenantRole: Owner/Admin always
// get allow, Contributor/Viewer fall through to the ACL column.
func (r *wikiAclRepository) PageOwnerAndAdmin(
	ctx context.Context,
	kbID string, slug string, callerUserID string,
) (string, bool, error) {
	var creatorID string
	row := r.db.WithContext(ctx).
		Table("knowledge_bases").
		Select("creator_id").
		Where("id = ? AND deleted_at IS NULL", kbID).
		Row()
	scanErr := row.Scan(&creatorID)
	if scanErr != nil && !isNoRows(scanErr) {
		return "", false, fmt.Errorf("wiki_acl repo: lookup kb owner: %w", scanErr)
	}
	// A missing KB row means caller is at best a stale viewer — we
	// simply return empty owner and let the ACL column decide (it can't
	// match an absent page anyway).

	role := types.TenantRoleFromContext(ctx)
	isAdmin := role == types.TenantRoleOwner || role == types.TenantRoleAdmin
	if isAdmin {
		logger.Debugf(ctx, "wiki acl resolve: caller %s has admin role %s for kb=%s", callerUserID, role, kbID)
	}
	return creatorID, isAdmin, nil
}

// GroupMembers returns the union of user IDs belonging to any of the
// supplied group ids. WeKnora does not currently model a user-groups
// table — the AllowGroupIDs column on WikiPageAcl is reserved for a
// future expansion and is always empty in production. We return an empty
// slice (never an error) so the resolve path treats group expansion as
// a no-op until the table is introduced.
func (r *wikiAclRepository) GroupMembers(ctx context.Context, tenantID uint64, groupIDs []string) ([]string, error) {
	if len(groupIDs) == 0 {
		return []string{}, nil
	}
	// Intentional no-op: groups are not yet modelled. Logging the call
	// so we notice when real groups start showing up in the ACL column.
	logger.Debugf(ctx, "wiki_acl repo: group expansion called with %d group ids but groups table is not modelled", len(groupIDs))
	return []string{}, nil
}

// ListAudit returns audit rows for one KB, newest-first, with an
// optional Since lower bound on created_at. Implementation notes:
//
//   - The migration's (knowledge_base_id, slug, created_at DESC)
//     covering index makes (kb, created_at DESC) lookups cheap; we
//     rely on Postgres/MySQL planner to collapse the second WHERE
//     clause into the same index range scan.
//   - Total count uses a SELECT COUNT(*) over the same WHERE clause.
//     For the audit-events fan-out the merge service relies on the
//     sum of per-source totals — a single COUNT here costs an extra
//     round-trip but is bounded by the same index. We accept the cost
//     to keep the API contract uniform across all four sources.
//   - pageSize is enforced server-side: caller-supplied pageSize < 1
//     falls back to 50; > 200 is clamped to 200 (matching the handler
//     cap so a rogue caller cannot exhaust the connection pool).
//   - Empty KB / empty result returns ([]*WikiAclAuditEntry{}, 0, nil)
//     so the merge service can render the other three sources without
//     a special-case branch.
//
// Build #24 B3 — first read path for wiki_page_acl_audit (was
// write-only since migration 000091).
func (r *wikiAclRepository) ListAudit(ctx context.Context, kbID string, since time.Time, page, pageSize int) ([]*types.WikiAclAuditEntry, int64, error) {
	if kbID == "" {
		return nil, 0, errors.New("wiki_acl repo: list audit: kb_id is required")
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var rows []*types.WikiAclAuditEntry
	findErr := r.db.WithContext(ctx).
		Table("wiki_page_acl_audit").
		Where("knowledge_base_id = ? AND created_at >= ?", kbID, since).
		Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&rows).Error
	if findErr != nil {
		return nil, 0, fmt.Errorf("wiki_acl repo: list audit find: %w", findErr)
	}

	var total int64
	countErr := r.db.WithContext(ctx).
		Table("wiki_page_acl_audit").
		Where("knowledge_base_id = ? AND created_at >= ?", kbID, since).
		Count(&total).Error
	if countErr != nil {
		return nil, 0, fmt.Errorf("wiki_acl repo: list audit count: %w", countErr)
	}

	if rows == nil {
		rows = []*types.WikiAclAuditEntry{}
	}
	return rows, total, nil
}

// isNoRows reports whether err is the driver-agnostic "no rows in result
// set" error. Keeps the repo free of database/sql imports.
func isNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}