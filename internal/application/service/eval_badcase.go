package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// Build #31 — Eval badcase service.
//
// Three write paths back the badcase library:
//
//   - FlagAuto: runner inserts an auto-flagged row below threshold.
//   - Promote: admin raises severity (or promotes a previously-passing
//     QA) — sets flag_source=human_promote, promoted_by + promoted_at.
//   - Resolve: admin marks the row closed (resolved | wontfix) — sets
//     resolved_at so B22 cleanup cron can sweep after 90 days.
//
// Read path: ListBadcases paginates newest-first with optional
// status / severity / flag_source filters. Audit row
// (eval.badcase_flagged or eval.run_reviewed) fires on every write.

// ErrBadcaseNotFound is returned when a badcase id misses. Distinct
// from a generic error so the handler can map it to HTTP 404.
var ErrBadcaseNotFound = errors.New("eval badcase not found")

type evalBadcaseService struct {
	db       *gorm.DB
	auditSvc interfaces.AuditLogService
}

// NewEvalBadcaseService wires the service. auditSvc may be nil in
// test wiring; the service degrades to warn-log.
func NewEvalBadcaseService(db *gorm.DB, auditSvc interfaces.AuditLogService) interfaces.EvalBadcaseService {
	return &evalBadcaseService{db: db, auditSvc: auditSvc}
}

// ListBadcases returns rows newest-first with optional filters. Empty
// filter fields are treated as "no filter" so callers can pass a
// zero-value EvalBadcaseFilter to get everything.
func (s *evalBadcaseService) ListBadcases(ctx context.Context, tenantID uint64, filter interfaces.EvalBadcaseFilter) ([]*types.EvalBadcase, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.FlagSource != "" {
		q = q.Where("flag_source = ?", filter.FlagSource)
	}
	var total int64
	if err := q.Model(&types.EvalBadcase{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count eval badcases: %w", err)
	}
	var rows []*types.EvalBadcase
	if err := q.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list eval badcases: %w", err)
	}
	return rows, int(total), nil
}

// FlagAuto inserts an auto-flagged row from the runner. The runner
// calls this only when avg(scores) < threshold (Build #31 D3). Idempotent
// per (run_id, qid, flag_source): a duplicate insert returns the
// existing row instead of erroring so a re-run (or a retry after a
// transient failure) does not produce two rows.
func (s *evalBadcaseService) FlagAuto(ctx context.Context, tenantID uint64, runID string, qid int, severity types.EvalSeverity, reason string) (*types.EvalBadcase, error) {
	if !validSeverity(severity) {
		return nil, fmt.Errorf("invalid severity: %q", severity)
	}
	existing, err := s.findExisting(ctx, tenantID, runID, qid, types.EvalBadcaseFlagSourceAuto)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	row := &types.EvalBadcase{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		RunID:      runID,
		QID:        qid,
		FlagSource: types.EvalBadcaseFlagSourceAuto,
		Severity:   severity,
		Status:     types.EvalBadcaseStatusOpen,
		Notes:      reason,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("create eval badcase: %w", err)
	}
	s.emitBadcaseAudit(ctx, tenantID, "", types.AuditActionEvalBadcaseFlagged, row, "auto")
	return row, nil
}

// Promote either updates the severity of an existing auto-flagged row
// (severity escalation) or creates a new human_promote row when the
// caller promotes a previously-passing QA. Emits eval.run_reviewed
// because promotion is an admin action.
func (s *evalBadcaseService) Promote(ctx context.Context, tenantID uint64, runID string, qid int, severity types.EvalSeverity, notes, promotedBy string) (*types.EvalBadcase, error) {
	if !validSeverity(severity) {
		return nil, fmt.Errorf("invalid severity: %q", severity)
	}
	if promotedBy == "" {
		return nil, errors.New("promoted_by is required")
	}
	// Try to update the auto row first; if none exists, create new.
	existing, err := s.findExisting(ctx, tenantID, runID, qid, types.EvalBadcaseFlagSourceAuto)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if existing != nil {
		// Upgrade severity only — never downgrade through Promote.
		if !severityHigher(severity, existing.Severity) {
			severity = existing.Severity
		}
		updates := map[string]any{
			"severity":    severity,
			"status":      types.EvalBadcaseStatusTriaged,
			"notes":       mergeNotes(existing.Notes, notes),
			"promoted_by": promotedBy,
			"promoted_at": now,
		}
		if err := s.db.WithContext(ctx).
			Model(&types.EvalBadcase{}).
			Where("id = ?", existing.ID).
			Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update eval badcase: %w", err)
		}
		existing.Severity = severity
		existing.Status = types.EvalBadcaseStatusTriaged
		existing.Notes = mergeNotes(existing.Notes, notes)
		existing.PromotedBy = promotedBy
		existing.PromotedAt = &now
		s.emitBadcaseAudit(ctx, tenantID, promotedBy, types.AuditActionEvalRunReviewed, existing, "promote")
		return existing, nil
	}
	// No auto row — create a human-promote row directly.
	row := &types.EvalBadcase{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		RunID:      runID,
		QID:        qid,
		FlagSource: types.EvalBadcaseFlagSourceHumanPromote,
		Severity:   severity,
		Status:     types.EvalBadcaseStatusTriaged,
		Notes:      notes,
		PromotedBy: promotedBy,
		PromotedAt: &now,
		CreatedAt:  now,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("create eval badcase promote: %w", err)
	}
	s.emitBadcaseAudit(ctx, tenantID, promotedBy, types.AuditActionEvalRunReviewed, row, "promote")
	return row, nil
}

// Resolve stamps resolved_at + status. The B22 cleanup cron picks up
// resolved rows after 90 days (Build #31 D4). Emits eval.run_reviewed.
func (s *evalBadcaseService) Resolve(ctx context.Context, tenantID uint64, badcaseID, notes string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).
		Model(&types.EvalBadcase{}).
		Where("id = ? AND tenant_id = ?", badcaseID, tenantID).
		Updates(map[string]any{
			"status":      types.EvalBadcaseStatusResolved,
			"resolved_at": now,
			"notes":       notes,
		})
	if res.Error != nil {
		return fmt.Errorf("resolve eval badcase: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrBadcaseNotFound
	}
	var row types.EvalBadcase
	if err := s.db.WithContext(ctx).Where("id = ?", badcaseID).First(&row).Error; err == nil {
		s.emitBadcaseAudit(ctx, tenantID, "", types.AuditActionEvalRunReviewed, &row, "resolve")
	}
	return nil
}

// findExisting returns the auto-flagged row for (run_id, qid) when
// one exists, or nil + nil if not. Used for the idempotency check.
func (s *evalBadcaseService) findExisting(ctx context.Context, tenantID uint64, runID string, qid int, source types.EvalBadcaseFlagSource) (*types.EvalBadcase, error) {
	var row types.EvalBadcase
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND run_id = ? AND qid = ? AND flag_source = ?",
			tenantID, runID, qid, source).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find existing badcase: %w", err)
	}
	return &row, nil
}

func validSeverity(s types.EvalSeverity) bool {
	switch s {
	case types.EvalSeverityLow, types.EvalSeverityMedium, types.EvalSeverityHigh, types.EvalSeverityCritical:
		return true
	}
	return false
}

// severityHigher reports whether a is strictly more severe than b.
// Used by Promote to never downgrade through an admin action.
func severityHigher(a, b types.EvalSeverity) bool {
	rank := map[types.EvalSeverity]int{
		types.EvalSeverityLow:      0,
		types.EvalSeverityMedium:   1,
		types.EvalSeverityHigh:     2,
		types.EvalSeverityCritical: 3,
	}
	return rank[a] > rank[b]
}

// mergeNotes concatenates the existing auto-flag reason with the
// admin's promotion note so the audit row carries both.
func mergeNotes(existing, added string) string {
	if existing == "" {
		return added
	}
	if added == "" {
		return existing
	}
	return existing + "\n-- promote -- " + added
}

// emitBadcaseAudit writes one row. Failure is non-fatal.
func (s *evalBadcaseService) emitBadcaseAudit(ctx context.Context, tenantID uint64, actorUserID string, action types.AuditAction, row *types.EvalBadcase, verb string) {
	details := map[string]any{
		"badcase_id":  row.ID,
		"run_id":      row.RunID,
		"qid":         row.QID,
		"severity":    string(row.Severity),
		"status":      string(row.Status),
		"flag_source": string(row.FlagSource),
		"verb":        verb,
	}
	detailJSON, _ := json.Marshal(details)
	entry := &types.AuditLog{
		TenantID:      tenantID,
		ActorUserID:   actorUserID,
		Action:        action,
		ScopeType:     "eval_badcase",
		ScopeID:       row.ID,
		TargetType:    "eval_badcase",
		TargetID:      row.ID,
		Outcome:       types.AuditOutcomeSuccess,
		Details:       types.JSON(detailJSON),
		CorrelationID: types.CorrelationIDFromContext(ctx),
	}
	if s.auditSvc == nil {
		logger.Warnf(ctx, "[eval_badcase] audit service unavailable; dropping badcase_id=%s action=%s",
			row.ID, action)
		return
	}
	if err := s.auditSvc.Log(ctx, entry); err != nil {
		logger.Warnf(ctx, "[eval_badcase] audit write failed badcase_id=%s action=%s: %v",
			row.ID, action, err)
	}
	_ = strconv.Itoa // keep strconv import for future row id formatting
}
