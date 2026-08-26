package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wikiBatchJobRepository persists wiki_batch_jobs rows. Plain GORM CRUD
// — the workers in WikiBatchJobService do the lifecycle transitions, this
// file owns the storage.
//
// Build #13.
type wikiBatchJobRepository struct {
	db *gorm.DB
}

// NewWikiBatchJobRepository wires the concrete repository. Returns the
// interface so consumers depend on contracts only.
func NewWikiBatchJobRepository(db *gorm.DB) interfaces.WikiBatchJobRepository {
	return &wikiBatchJobRepository{db: db}
}

// Create inserts a new job row. Caller is responsible for assigning ID
// (uuid.NewString) and CreatedAt.
func (r *wikiBatchJobRepository) Create(ctx context.Context, job *types.WikiBatchJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

// GetByID fetches by primary key. Returns ErrWikiBatchJobNotFound on
// the standard GORM "rows affected" sentinel.
func (r *wikiBatchJobRepository) GetByID(ctx context.Context, id string) (*types.WikiBatchJob, error) {
	var job types.WikiBatchJob
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrWikiBatchJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

// Update rewrites the row. Workers use this to advance state and write
// result/started_at/finished_at/expires_at as the job lifecycle plays
// out. We do not bump any version column — wiki_batch_jobs is append-
// only from the user's perspective; concurrent updates between workers
// are serialised by the queued→running claim below.
func (r *wikiBatchJobRepository) Update(ctx context.Context, job *types.WikiBatchJob) error {
	return r.db.WithContext(ctx).Save(job).Error
}

// ClaimNextQueued atomically advances a single queued row to running in
// one transaction. Multiple workers call this concurrently; the FOR
// UPDATE SKIP LOCKED clause (Postgres) makes each worker see a distinct
// row so the channel can fan out without a coordinator. SQLite falls
// back to a simpler "first queued" claim because the in-process pool
// is single-instance there.
//
// Returns ErrWikiBatchJobNone if nothing is queued (workers idle).
func (r *wikiBatchJobRepository) ClaimNextQueued(ctx context.Context) (*types.WikiBatchJob, error) {
	if r.db.Dialector != nil && r.db.Dialector.Name() == "sqlite" {
		return r.claimNextQueuedSQLite(ctx)
	}
	return r.claimNextQueuedPostgres(ctx)
}

func (r *wikiBatchJobRepository) claimNextQueuedPostgres(ctx context.Context) (*types.WikiBatchJob, error) {
	var job types.WikiBatchJob
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock and pick the oldest queued row.
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("state = ?", types.WikiBatchJobStateQueued).
			Order("created_at ASC").
			First(&job).Error; err != nil {
			return err
		}
		job.State = types.WikiBatchJobStateRunning
		job.StartedAt = &now
		return tx.Save(&job).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrWikiBatchJobNone
		}
		return nil, err
	}
	return &job, nil
}

// claimNextQueuedSQLite mirrors claimNextQueuedPostgres but without
// SKIP LOCKED (SQLite serialises writers anyway via the database lock).
// Only used when running tests with the SQLite driver.
func (r *wikiBatchJobRepository) claimNextQueuedSQLite(ctx context.Context) (*types.WikiBatchJob, error) {
	var job types.WikiBatchJob
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("state = ?", types.WikiBatchJobStateQueued).
			Order("created_at ASC").
			First(&job).Error; err != nil {
			return err
		}
		job.State = types.WikiBatchJobStateRunning
		job.StartedAt = &now
		return tx.Save(&job).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrWikiBatchJobNone
		}
		return nil, err
	}
	return &job, nil
}

// ListExpired returns finished jobs whose undo window has elapsed.
// limit caps the result set; pass 0 for no cap.
func (r *wikiBatchJobRepository) ListExpired(
	ctx context.Context, now time.Time, limit int,
) ([]*types.WikiBatchJob, error) {
	q := r.db.WithContext(ctx).
		Where("state IN ?", []types.WikiBatchJobState{
			types.WikiBatchJobStateSucceeded,
			types.WikiBatchJobStateFailed,
			types.WikiBatchJobStatePartial,
		}).
		Where("expires_at IS NOT NULL AND expires_at < ?", now).
		Order("expires_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var jobs []*types.WikiBatchJob
	if err := q.Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// DeleteByID hard-removes a job row. Cleanup cron only.
func (r *wikiBatchJobRepository) DeleteByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.WikiBatchJob{}).Error
}