package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubAuditLogService is an in-memory AuditLogService that returns a
// configurable slice of audit rows. The unit tests use it to drive the
// export + compliance-summary paths without standing up a database.
type stubAuditLogService struct {
	rows []*types.AuditLog
}

func (s *stubAuditLogService) Log(ctx context.Context, entry *types.AuditLog) error {
	s.rows = append(s.rows, entry)
	return nil
}

func (s *stubAuditLogService) LogDenied(ctx context.Context, c interface{}, tenantID uint64, actorUserID, actorRole string, requiredRole types.TenantRole) error {
	s.rows = append(s.rows, &types.AuditLog{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		Action:      types.AuditActionAccessDenied,
		Outcome:     types.AuditOutcomeDenied,
	})
	return nil
}

func (s *stubAuditLogService) List(ctx context.Context, tenantID uint64, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
	out := make([]*types.AuditLog, 0, len(s.rows))
	for _, r := range s.rows {
		if r.TenantID != tenantID {
			continue
		}
		out = append(out, r)
	}
	if q != nil && q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (s *stubAuditLogService) Purge(ctx context.Context, retentionDays int) (int64, error) {
	return 0, nil
}

// stubAuditExportRepo is an in-memory AuditExportRepository.
type stubAuditExportRepo struct {
	rows map[string]*types.AuditExport
}

func newStubAuditExportRepo() *stubAuditExportRepo {
	return &stubAuditExportRepo{rows: map[string]*types.AuditExport{}}
}

func (r *stubAuditExportRepo) Create(ctx context.Context, export *types.AuditExport) error {
	cp := *export
	r.rows[export.ID] = &cp
	return nil
}

func (r *stubAuditExportRepo) GetByID(ctx context.Context, tenantID uint64, id string) (*types.AuditExport, error) {
	if e, ok := r.rows[id]; ok && e.TenantID == tenantID {
		cp := *e
		return &cp, nil
	}
	return nil, nil
}

func (r *stubAuditExportRepo) ListByTenant(ctx context.Context, tenantID uint64, limit int) ([]types.AuditExport, error) {
	out := []types.AuditExport{}
	for _, e := range r.rows {
		if e.TenantID == tenantID {
			out = append(out, *e)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *stubAuditExportRepo) UpdateStatus(ctx context.Context, id string, status types.AuditExportStatus, rowCount int64, byteSize int64, errMsg string) error {
	e, ok := r.rows[id]
	if !ok {
		return nil
	}
	e.Status = status
	e.RowCount = rowCount
	e.ByteSize = byteSize
	e.Error = errMsg
	if status == types.AuditExportStatusSucceeded || status == types.AuditExportStatusFailed {
		now := time.Now()
		e.FinishedAt = &now
	}
	return nil
}

func TestAuditExport_CreateAndRun_HappyCSV(t *testing.T) {
	logSvc := &stubAuditLogService{
		rows: []*types.AuditLog{
			{ID: 1, TenantID: 1, ActorUserID: "u-1", Action: types.AuditActionMemberAdded, Outcome: types.AuditOutcomeSuccess, CreatedAt: time.Now()},
			{ID: 2, TenantID: 1, ActorUserID: "u-2", Action: types.AuditActionAccessDenied, Outcome: types.AuditOutcomeDenied, CreatedAt: time.Now()},
		},
	}
	repo := newStubAuditExportRepo()
	svc := NewAuditExportService(logSvc, repo)

	export, payload, err := svc.CreateAndRun(context.Background(), 1, "admin-1", types.AuditExportCreateRequest{
		Format: types.AuditExportFormatCSV,
	})
	if err != nil {
		t.Fatalf("CreateAndRun: %v", err)
	}
	if export.Status != types.AuditExportStatusSucceeded {
		t.Fatalf("status = %v, want succeeded", export.Status)
	}
	if export.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", export.RowCount)
	}
	if len(payload) == 0 {
		t.Fatal("empty payload")
	}
	// CSV header sanity check.
	s := string(s)
	if !contains(payload, "id,tenant_id,actor_user_id") {
		t.Fatalf("missing CSV header in payload")
	}
}

func TestAuditExport_CreateAndRun_JSON(t *testing.T) {
	logSvc := &stubAuditLogService{rows: []*types.AuditLog{
		{ID: 1, TenantID: 1, ActorUserID: "u-1", Action: types.AuditActionMemberAdded, Outcome: types.AuditOutcomeSuccess, CreatedAt: time.Now()},
	}}
	repo := newStubAuditExportRepo()
	svc := NewAuditExportService(logSvc, repo)

	_, payload, err := svc.CreateAndRun(context.Background(), 1, "admin-1", types.AuditExportCreateRequest{
		Format: types.AuditExportFormatJSON,
	})
	if err != nil {
		t.Fatalf("CreateAndRun: %v", err)
	}
	s := string(payload)
	if !contains(s, "\"action\"") {
		t.Fatalf("JSON payload missing action field")
	}
	if !contains(s, "u-1") {
		t.Fatalf("JSON payload missing actor")
	}
}

func TestAuditExport_CreateAndRun_RejectsBadFormat(t *testing.T) {
	logSvc := &stubAuditLogService{}
	repo := newStubAuditExportRepo()
	svc := NewAuditExportService(logSvc, repo)

	_, _, err := svc.CreateAndRun(context.Background(), 1, "admin-1", types.AuditExportCreateRequest{
		Format: types.AuditExportFormat("xml"),
	})
	if err != types.ErrAuditExportBadInput {
		t.Fatalf("expected ErrAuditExportBadInput, got %v", err)
	}
}

func TestAuditExport_ComplianceSummary_OK(t *testing.T) {
	now := time.Now()
	logSvc := &stubAuditLogService{rows: []*types.AuditLog{
		{TenantID: 1, ActorUserID: "u-1", Action: types.AuditActionMemberAdded, Outcome: types.AuditOutcomeSuccess, CreatedAt: now},
		{TenantID: 1, ActorUserID: "u-2", Action: types.AuditActionMemberAdded, Outcome: types.AuditOutcomeSuccess, CreatedAt: now},
		{TenantID: 1, ActorUserID: "u-1", Action: types.AuditActionMemberRemoved, Outcome: types.AuditOutcomeSuccess, CreatedAt: now},
	}}
	repo := newStubAuditExportRepo()
	svc := NewAuditExportService(logSvc, repo)

	summary, err := svc.ComplianceSummary(context.Background(), 1, 30)
	if err != nil {
		t.Fatalf("ComplianceSummary: %v", err)
	}
	if summary.TotalEvents != 3 {
		t.Fatalf("total = %d, want 3", summary.TotalEvents)
	}
	if summary.ComplianceStatus != "ok" {
		t.Fatalf("status = %s, want ok", summary.ComplianceStatus)
	}
	if summary.UniqueActors != 2 {
		t.Fatalf("unique actors = %d, want 2", summary.UniqueActors)
	}
}

func TestAuditExport_ComplianceSummary_Violation(t *testing.T) {
	now := time.Now()
	rows := []*types.AuditLog{}
	// 8 success + 2 denied -> denied ratio > 10% -> violation
	for i := 0; i < 8; i++ {
		rows = append(rows, &types.AuditLog{TenantID: 1, Action: types.AuditActionMemberAdded, Outcome: types.AuditOutcomeSuccess, CreatedAt: now})
	}
	for i := 0; i < 2; i++ {
		rows = append(rows, &types.AuditLog{TenantID: 1, Action: types.AuditActionAccessDenied, Outcome: types.AuditOutcomeDenied, CreatedAt: now})
	}
	logSvc := &stubAuditLogService{rows: rows}
	svc := NewAuditExportService(logSvc, newStubAuditExportRepo())

	summary, err := svc.ComplianceSummary(context.Background(), 1, 30)
	if err != nil {
		t.Fatalf("ComplianceSummary: %v", err)
	}
	if summary.ComplianceStatus != "violation" {
		t.Fatalf("status = %s, want violation", summary.ComplianceStatus)
	}
	if summary.DeniedCount != 2 {
		t.Fatalf("denied = %d, want 2", summary.DeniedCount)
	}
}

// contains is a tiny string contains helper. The service package has
// its own contains() helper but we redeclare here to avoid a name
// collision when other test files redeclare the same symbol.
func contains(b []byte, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(b) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(b); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if b[i+j] != sub[j] {
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
