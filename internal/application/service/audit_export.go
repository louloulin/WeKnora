package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewAuditExportService wires the audit-export service. The
// AuditLogService is required because every export reads through it;
// the AuditExportRepository is required because the metadata row is
// how an admin can later re-fetch the export.
func NewAuditExportService(
	auditSvc interfaces.AuditLogService,
	repo interfaces.AuditExportRepository,
) interfaces.AuditExportService {
	return &auditExportService{auditSvc: auditSvc, repo: repo}
}

type auditExportService struct {
	auditSvc interfaces.AuditLogService
	repo     interfaces.AuditExportRepository
}

// CreateAndRun validates the request, runs the audit-log scan, encodes
// the payload, and persists the export metadata. The encoded payload
// is returned alongside the AuditExport so the handler can stream it
// straight to the client without an intermediate temp file.
func (s *auditExportService) CreateAndRun(
	ctx context.Context,
	tenantID uint64,
	requestedBy string,
	req types.AuditExportCreateRequest,
) (*types.AuditExport, []byte, error) {
	if err := req.Normalize(); err != nil {
		return nil, nil, err
	}

	// Persist the export row first so we have an ID to surface in any
	// downstream error. We initialise it as "pending" and transition
	// to the terminal status at the end of the scan.
	filterJSON, err := encodeFilter(req.Filter)
	if err != nil {
		return nil, nil, fmt.Errorf("audit export: encode filter: %w", err)
	}

	export := &types.AuditExport{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		RequestedBy: requestedBy,
		Format:      req.Format,
		FilterJSON:  filterJSON,
		Status:      types.AuditExportStatusRunning,
	}
	if err := s.repo.Create(ctx, export); err != nil {
		return nil, nil, fmt.Errorf("audit export: persist metadata: %w", err)
	}

	rows, err := s.scanAuditLog(ctx, tenantID, req.Filter, req.MaxRows)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, export.ID, types.AuditExportStatusFailed, 0, 0, err.Error())
		logger.Errorf(ctx, "audit export: scan failed: id=%s tenant=%d err=%v", export.ID, tenantID, err)
		return nil, nil, err
	}

	payload, err := encodePayload(req.Format, rows)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, export.ID, types.AuditExportStatusFailed, 0, 0, err.Error())
		return nil, nil, err
	}

	if err := s.repo.UpdateStatus(ctx, export.ID, types.AuditExportStatusSucceeded, int64(len(rows)), int64(len(payload)), ""); err != nil {
		// Non-fatal: still return the payload so the caller can stream
		// it. Log so the operator can investigate later.
		logger.Errorf(ctx, "audit export: status update failed: id=%s err=%v", export.ID, err)
	}
	export.RowCount = int64(len(rows))
	export.ByteSize = int64(len(payload))
	export.Status = types.AuditExportStatusSucceeded
	return export, payload, nil
}

// Get returns a previously-created export.
func (s *auditExportService) Get(ctx context.Context, tenantID uint64, id string) (*types.AuditExport, error) {
	row, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, interfaces.ErrAuditExportNotFound
	}
	return row, nil
}

// List returns the most recent exports for a tenant.
func (s *auditExportService) List(ctx context.Context, tenantID uint64, limit int) ([]types.AuditExport, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit)
}

// ComplianceSummary rolls up the audit_logs slice over the trailing
// window and returns the top-action / top-actor breakdowns.
func (s *auditExportService) ComplianceSummary(
	ctx context.Context,
	tenantID uint64,
	windowDays int,
) (*types.AuditExportSummary, error) {
	if windowDays <= 0 {
		windowDays = types.ComplianceReportWindowDays
	}
	end := time.Now()
	start := end.AddDate(0, 0, -windowDays)
	rows, err := s.scanAuditLog(ctx, tenantID, types.AuditExportFilter{
		StartTime: &start,
		EndTime:   &end,
	}, types.AuditExportMaxRows)
	if err != nil {
		return nil, err
	}

	summary := &types.AuditExportSummary{
		TenantID:    tenantID,
		WindowStart: start,
		WindowEnd:   end,
		TotalEvents: int64(len(rows)),
	}

	actionCounts := map[string]int64{}
	actorCounts := map[string]int64{}
	actorSet := map[string]struct{}{}
	for _, row := range rows {
		switch row.Outcome {
		case types.AuditOutcomeSuccess, types.AuditOutcomeAccepted:
			summary.SuccessCount++
		case types.AuditOutcomeDenied:
			summary.DeniedCount++
		case types.AuditOutcomeFailed:
			summary.FailedCount++
		}
		actionCounts[string(row.Action)]++
		if row.ActorUserID != "" {
			actorSet[row.ActorUserID] = struct{}{}
			actorCounts[row.ActorUserID]++
		}
	}
	summary.UniqueActors = len(actorSet)
	summary.TopActions = topActionTallies(actionCounts, 10)
	summary.TopActors = topActorTallies(actorCounts, 10)

	// Compliance status heuristic: a denied / failed ratio above 5%
	// warrants a manual review; a higher-than-10% ratio flags a
	// potential violation. These numbers can be tuned per-tenant
	// later via config.
	total := float64(summary.TotalEvents)
	if total == 0 {
		summary.ComplianceStatus = "ok"
		return summary, nil
	}
	denied := float64(summary.DeniedCount + summary.FailedCount)
	ratio := denied / total
	switch {
	case ratio > 0.10:
		summary.ComplianceStatus = "violation"
	case ratio > 0.05:
		summary.ComplianceStatus = "review"
	default:
		summary.ComplianceStatus = "ok"
	}
	return summary, nil
}

// scanAuditLog fetches audit_log rows under the supplied filter. The
// existing AuditLogRepository.List already supports Action / Outcome /
// ScopeType / ScopeID / ActorUserID; this layer adds the time window
// (StartTime / EndTime) by paginating with AfterID.
func (s *auditExportService) scanAuditLog(
	ctx context.Context,
	tenantID uint64,
	filter types.AuditExportFilter,
	maxRows int,
) ([]*types.AuditLog, error) {
	if maxRows <= 0 || maxRows > types.AuditExportMaxRows {
		maxRows = types.AuditExportMaxRows
	}
	out := make([]*types.AuditLog, 0, 256)
	var afterID uint64
	for {
		remaining := maxRows - len(out)
		if remaining <= 0 {
			break
		}
		// Page size capped at 1000 to keep the SQL plan tidy.
		pageSize := remaining
		if pageSize > 1000 {
			pageSize = 1000
		}
		q := &interfaces.AuditLogQuery{
			AfterID:     afterID,
			Limit:       pageSize,
			Action:      types.AuditAction(filter.Action),
			Outcome:     types.AuditOutcome(filter.Outcome),
			ActorUserID: filter.ActorID,
			ScopeType:   filter.ScopeType,
			ScopeID:     filter.ScopeID,
		}
		page, err := s.auditSvc.List(ctx, tenantID, q)
		if err != nil {
			return nil, fmt.Errorf("audit export: list page: %w", err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			if filter.StartTime != nil && row.CreatedAt.Before(*filter.StartTime) {
				continue
			}
			if filter.EndTime != nil && row.CreatedAt.After(*filter.EndTime) {
				continue
			}
			out = append(out, row)
			if len(out) >= maxRows {
				break
			}
		}
		if len(page) < pageSize {
			break
		}
		afterID = page[len(page)-1].ID
	}
	return out, nil
}

// encodeFilter JSON-encodes the filter so we can persist it on the
// export row without inventing a per-field schema.
func encodeFilter(f types.AuditExportFilter) (string, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// encodePayload turns the audit-log rows into the requested wire
// format. CSV uses standard excel-friendly escaping; JSON is a
// pretty-printed array of objects so it round-trips through jq.
func encodePayload(format types.AuditExportFormat, rows []*types.AuditLog) ([]byte, error) {
	switch format {
	case types.AuditExportFormatCSV:
		return encodeCSV(rows)
	case types.AuditExportFormatJSON:
		return encodeJSON(rows)
	default:
		return nil, fmt.Errorf("audit export: unsupported format %q", format)
	}
}

func encodeCSV(rows []*types.AuditLog) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{
		"id", "tenant_id", "actor_user_id", "actor_role",
		"action", "outcome", "scope_type", "scope_id",
		"target_type", "target_id", "target_user_id",
		"request_method", "request_path",
		"correlation_id", "created_at",
	}); err != nil {
		return nil, err
	}
	for _, r := range rows {
		_ = w.Write([]string{
			fmt.Sprintf("%d", r.ID),
			fmt.Sprintf("%d", r.TenantID),
			r.ActorUserID,
			r.ActorRole,
			string(r.Action),
			string(r.Outcome),
			r.ScopeType,
			r.ScopeID,
			r.TargetType,
			r.TargetID,
			r.TargetUserID,
			r.RequestMethod,
			r.RequestPath,
			r.CorrelationID,
			r.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJSON(rows []*types.AuditLog) ([]byte, error) {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]interface{}{
			"id":             r.ID,
			"tenant_id":      r.TenantID,
			"actor_user_id":  r.ActorUserID,
			"actor_role":     r.ActorRole,
			"action":         r.Action,
			"outcome":        r.Outcome,
			"scope_type":     r.ScopeType,
			"scope_id":       r.ScopeID,
			"target_type":    r.TargetType,
			"target_id":      r.TargetID,
			"target_user_id": r.TargetUserID,
			"request_method": r.RequestMethod,
			"request_path":   r.RequestPath,
			"correlation_id": r.CorrelationID,
			"created_at":     r.CreatedAt.UTC().Format(time.RFC3339),
			"details":        r.Details,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

// topActionTallies returns the top-N actions sorted by count desc.
func topActionTallies(counts map[string]int64, n int) []types.AuditActionTally {
	type pair struct {
		action string
		count  int64
	}
	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
	if n > len(pairs) {
		n = len(pairs)
	}
	out := make([]types.AuditActionTally, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, types.AuditActionTally{
			Action: types.AuditAction(pairs[i].action),
			Count:  pairs[i].count,
		})
	}
	return out
}

// topActorTallies returns the top-N actors sorted by count desc.
func topActorTallies(counts map[string]int64, n int) []types.AuditActorTally {
	type pair struct {
		actor string
		count int64
	}
	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
	if n > len(pairs) {
		n = len(pairs)
	}
	out := make([]types.AuditActorTally, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, types.AuditActorTally{
			ActorUserID: pairs[i].actor,
			Count:       pairs[i].count,
		})
	}
	return out
}

// Sentinel used by handler for 502 / 500 mapping. Defined here so the
// service surfaces a single typed error when GORM is unreachable.
var errAuditExportTransient = errors.New("audit export: transient failure")

// Compile-time assertion.
var _ interfaces.AuditExportService = (*auditExportService)(nil)

// Ensure strings is referenced so gofmt doesn't strip an unused
// import if this file's contents shift during a future refactor.
var _ = strings.TrimSpace
var _ = errAuditExportTransient
