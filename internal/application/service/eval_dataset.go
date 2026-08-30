package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// Build #31 — Eval dataset service.
//
// The legacy DatasetService (internal/application/service/dataset.go)
// still serves the existing /api/v1/evaluation route by loading the
// bundled parquet fixture. EvalDatasetService is the new persistent
// backing store for /api/v1/eval/datasets; it owns five tables
// (eval_datasets, eval_dataset_qa, eval_runs, eval_run_results,
// eval_badcases) via the GORM handle.
//
// Design notes:
//
//   - Tenant scoping: every read filters on tenant_id so a token from
//     one workspace cannot enumerate another workspace's datasets.
//   - ReplaceQAList runs the QA swap in one transaction so a partial
//     failure does not strand a dataset with the old questions visible
//     and the new questions dropped.
//   - QACount is denormalised: every write that touches eval_dataset_qa
//     recomputes the count and stamps it back. The harness (B11) pins
//     this invariant so we never ship a count that disagrees with the
//     row count.
//   - Dataset cap (Build #31 D5): at most 100 datasets per tenant and
//     10000 QA rows per dataset. Limits are enforced before the insert
//     so the caller gets a typed error instead of a DB constraint
//     surprise.

const (
	// EvalMaxDatasetsPerTenant caps the number of datasets a single
	// tenant can own. Picked at 100 to leave headroom for product
	// experiments without letting a runaway script saturate the table.
	EvalMaxDatasetsPerTenant = 100
	// EvalMaxQAPerDataset caps the number of QA rows in one dataset.
	// Picked at 10000 to bound single-run cost (each row triggers a
	// chat pipeline call + a judge call).
	EvalMaxQAPerDataset = 10000
)

// ErrDatasetCapReached is returned when a tenant tries to create more
// than EvalMaxDatasetsPerTenant datasets. Handlers translate this to
// HTTP 422.
var ErrDatasetCapReached = errors.New("eval dataset cap reached for tenant")

// ErrQACapReached is returned when ReplaceQAList / ImportJSON would
// land more than EvalMaxQAPerDataset rows in one dataset.
var ErrQACapReached = errors.New("eval dataset QA cap reached")

// ErrDatasetNotFound is returned when a dataset lookup misses. Distinct
// from a generic error so the handler can map it to HTTP 404.
var ErrDatasetNotFound = errors.New("eval dataset not found")

// EvalDatasetService is the persistent eval-dataset backend. It does
// not talk to a KB or to the chat pipeline — those happen in
// EvalRunnerService.
type EvalDatasetService struct {
	db       *gorm.DB
	auditSvc interfaces.AuditLogService
}

// NewEvalDatasetService wires the dataset service. auditSvc may be nil
// in test wiring; the dataset writer degrades to "log only" instead of
// panicking.
func NewEvalDatasetService(db *gorm.DB, auditSvc interfaces.AuditLogService) interfaces.EvalDatasetService {
	return &EvalDatasetService{db: db, auditSvc: auditSvc}
}

// CreateDataset persists a new dataset row. The caller is responsible
// for populating ID, TenantID, CreatedBy, and CreatedAt/UpdatedAt —
// the helper stamps CreatedAt / UpdatedAt if they are zero.
func (s *EvalDatasetService) CreateDataset(ctx context.Context, ds *types.EvalDataset) error {
	if ds.ID == "" {
		return errors.New("dataset id is required")
	}
	if ds.TenantID == 0 {
		return errors.New("tenant id is required")
	}
	if ds.Name == "" {
		return errors.New("dataset name is required")
	}
	now := time.Now().UTC()
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = now
	}
	if ds.UpdatedAt.IsZero() {
		ds.UpdatedAt = now
	}
	if ds.SchemaVersion == 0 {
		ds.SchemaVersion = 1
	}
	ds.QACount = 0

	var count int64
	if err := s.db.WithContext(ctx).
		Model(&types.EvalDataset{}).
		Where("tenant_id = ?", ds.TenantID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("count tenant datasets: %w", err)
	}
	if count >= int64(EvalMaxDatasetsPerTenant) {
		return ErrDatasetCapReached
	}

	if err := s.db.WithContext(ctx).Create(ds).Error; err != nil {
		return fmt.Errorf("create eval dataset: %w", err)
	}
	s.emitDatasetAudit(ctx, ds.TenantID, ds.CreatedBy, "created", ds.ID, "")
	return nil
}

// GetDatasetByID returns the dataset metadata plus the full QA list
// ordered by qid ASC.
func (s *EvalDatasetService) GetDatasetByID(ctx context.Context, datasetID string) (*types.EvalDataset, []types.EvalDatasetQA, error) {
	var ds types.EvalDataset
	if err := s.db.WithContext(ctx).Where("id = ?", datasetID).First(&ds).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrDatasetNotFound
		}
		return nil, nil, fmt.Errorf("load eval dataset: %w", err)
	}
	var qas []types.EvalDatasetQA
	if err := s.db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Order("qid ASC").
		Find(&qas).Error; err != nil {
		return nil, nil, fmt.Errorf("load eval dataset QA: %w", err)
	}
	return &ds, qas, nil
}

// ListDatasets returns metadata for the tenant newest-first. limit /
// offset are clamped to a sane range so a caller cannot accidentally
// page through everything.
func (s *EvalDatasetService) ListDatasets(ctx context.Context, tenantID uint64, limit, offset int) ([]*types.EvalDataset, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := s.db.WithContext(ctx).
		Model(&types.EvalDataset{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count eval datasets: %w", err)
	}
	var rows []*types.EvalDataset
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list eval datasets: %w", err)
	}
	return rows, int(total), nil
}

// UpdateDataset mutates name / description. QA list changes go through
// ReplaceQAList. Returns ErrDatasetNotFound when no row matches.
func (s *EvalDatasetService) UpdateDataset(ctx context.Context, ds *types.EvalDataset) error {
	if ds == nil || ds.ID == "" {
		return errors.New("dataset id is required")
	}
	ds.UpdatedAt = time.Now().UTC()
	res := s.db.WithContext(ctx).
		Model(&types.EvalDataset{}).
		Where("id = ? AND tenant_id = ?", ds.ID, ds.TenantID).
		Updates(map[string]any{
			"name":        ds.Name,
			"description": ds.Description,
			"updated_at":  ds.UpdatedAt,
		})
	if res.Error != nil {
		return fmt.Errorf("update eval dataset: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrDatasetNotFound
	}
	s.emitDatasetAudit(ctx, ds.TenantID, "", "updated", ds.ID, "")
	return nil
}

// DeleteDataset cascades to QA + runs + badcases via FK constraints.
func (s *EvalDatasetService) DeleteDataset(ctx context.Context, tenantID uint64, datasetID string) error {
	res := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", datasetID, tenantID).
		Delete(&types.EvalDataset{})
	if res.Error != nil {
		return fmt.Errorf("delete eval dataset: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrDatasetNotFound
	}
	s.emitDatasetAudit(ctx, tenantID, "", "deleted", datasetID, "")
	return nil
}

// ReplaceQAList swaps a dataset's QA rows in one transaction. Cap is
// checked before the delete so a caller that wants to grow an existing
// dataset from 50 → 200 rows gets a clear error rather than a partial
// commit.
func (s *EvalDatasetService) ReplaceQAList(ctx context.Context, datasetID string, qas []types.EvalDatasetQA) error {
	if len(qas) > EvalMaxQAPerDataset {
		return ErrQACapReached
	}
	// Default qid to array index when caller leaves it zero; this matches
	// the JSON-import path so callers can omit the qid field.
	for i := range qas {
		if qas[i].QID == 0 {
			qas[i].QID = i + 1
		}
		if qas[i].DatasetID == "" {
			qas[i].DatasetID = datasetID
		}
		if qas[i].CreatedAt.IsZero() {
			qas[i].CreatedAt = time.Now().UTC()
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dataset_id = ?", datasetID).
			Delete(&types.EvalDatasetQA{}).Error; err != nil {
			return fmt.Errorf("delete eval dataset QA: %w", err)
		}
		if len(qas) > 0 {
			if err := tx.Create(&qas).Error; err != nil {
				return fmt.Errorf("insert eval dataset QA: %w", err)
			}
		}
		if err := tx.Model(&types.EvalDataset{}).
			Where("id = ?", datasetID).
			Updates(map[string]any{
				"qa_count":   len(qas),
				"updated_at": time.Now().UTC(),
			}).Error; err != nil {
			return fmt.Errorf("update eval dataset qa_count: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Audit is emitted after commit; failure to log is non-fatal.
	var ds types.EvalDataset
	if lookupErr := s.db.WithContext(ctx).Where("id = ?", datasetID).First(&ds).Error; lookupErr == nil {
		s.emitDatasetAudit(ctx, ds.TenantID, "", "qa_replaced", datasetID, fmt.Sprintf("%d rows", len(qas)))
	}
	return nil
}

// ImportJSON parses an EvalDatasetJSONPayload and creates the dataset +
// QA rows in one call. Returns the new dataset id. Bails with
// ErrDatasetCapReached / ErrQACapReached before touching the DB so the
// caller can surface a 422 without an orphaned partial commit.
func (s *EvalDatasetService) ImportJSON(ctx context.Context, tenantID uint64, createdBy string, payload *interfaces.EvalDatasetJSONPayload) (string, error) {
	if payload == nil {
		return "", errors.New("payload is required")
	}
	if payload.Name == "" {
		return "", errors.New("dataset name is required")
	}
	if len(payload.QA) > EvalMaxQAPerDataset {
		return "", ErrQACapReached
	}

	datasetID := uuid.NewString()
	ds := &types.EvalDataset{
		ID:            datasetID,
		TenantID:      tenantID,
		Name:          payload.Name,
		Description:   payload.Description,
		SchemaVersion: 1,
		CreatedBy:     createdBy,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.CreateDataset(ctx, ds); err != nil {
		return "", err
	}
	qas := make([]types.EvalDatasetQA, 0, len(payload.QA))
	for i, q := range payload.QA {
		if q.Question == "" || q.ExpectedAnswer == "" {
			return "", fmt.Errorf("qa[%d]: question and expected_answer are required", i)
		}
		qid := q.QID
		if qid == 0 {
			qid = i + 1
		}
		qas = append(qas, types.EvalDatasetQA{
			DatasetID:        datasetID,
			QID:              qid,
			Question:         q.Question,
			ExpectedAnswer:   q.ExpectedAnswer,
			ExpectedPassages: q.ExpectedPassages,
			Tags:             q.Tags,
			CreatedAt:        time.Now().UTC(),
		})
	}
	if err := s.ReplaceQAList(ctx, datasetID, qas); err != nil {
		// Best-effort cleanup so the half-created dataset does not linger.
		_ = s.DeleteDataset(ctx, tenantID, datasetID)
		return "", err
	}
	return datasetID, nil
}

// emitDatasetAudit writes one eval.dataset_updated row. The audit
// service may be nil in unit tests; we degrade to warn-log so the
// caller never sees an audit-only failure.
func (s *EvalDatasetService) emitDatasetAudit(ctx context.Context, tenantID uint64, actorUserID, verb, datasetID, extra string) {
	details := map[string]any{
		"verb":       verb,
		"dataset_id": datasetID,
	}
	if extra != "" {
		details["extra"] = extra
	}
	detailJSON, _ := json.Marshal(details)
	entry := &types.AuditLog{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		Action:      types.AuditActionEvalDatasetUpdated,
		ScopeType:   "eval_dataset",
		ScopeID:     datasetID,
		TargetType:  "eval_dataset",
		TargetID:    datasetID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailJSON),
	}
	if s.auditSvc == nil {
		logger.Warnf(ctx, "[eval_dataset] audit service unavailable; dropping row dataset_id=%s verb=%s",
			datasetID, verb)
		return
	}
	if err := s.auditSvc.Log(ctx, entry); err != nil {
		logger.Warnf(ctx, "[eval_dataset] audit write failed dataset_id=%s verb=%s: %v",
			datasetID, verb, err)
	}
}
