package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/wikicachemetrics"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// WikiBacklinksCacheCleanupConfig holds the tunable knobs for the
// Build #22 sweeper. All values come from environment variables read
// once at startup by cmd/server — the service treats them as
// immutable.
//
// Defaults (D1–D7 from the Build #22 brief):
//   - TTL:        30 days (WIKI_CACHE_TTL_DAYS)
//   - Period:     24h     (WIKI_CACHE_CLEANUP_PERIOD_HOURS)
//   - BatchSize:  1000    (WIKI_CACHE_CLEANUP_BATCH_SIZE)
//   - DryRun:     true    (WIKI_CACHE_CLEANUP_DRY_RUN)
//   - MaxRows:    1_000_000 alert threshold (WIKI_CACHE_MAX_ROWS)
//
// Note: the brief said "D4 默认 dry-run". That is intentional — we
// ship a passive cron that only logs counts until an operator flips
// the env var. This avoids surprise data loss on the first deploy
// after Build #22 ships.
type WikiBacklinksCacheCleanupConfig struct {
	TTL       time.Duration // age threshold: rows with updated_at older than now-TTL are stale
	Period    time.Duration // how often the sweeper runs
	BatchSize int           // DELETE / FOR UPDATE batch size
	DryRun    bool          // true = count only, never DELETE
	MaxRows   int64         // gauge alert threshold
}

// DefaultWikiBacklinksCacheCleanupConfig returns the recommended
// defaults. cmd/server calls env-overrides after this.
func DefaultWikiBacklinksCacheCleanupConfig() WikiBacklinksCacheCleanupConfig {
	return WikiBacklinksCacheCleanupConfig{
		TTL:       30 * 24 * time.Hour,
		Period:    24 * time.Hour,
		BatchSize: 1000,
		DryRun:    true,
		MaxRows:   1_000_000,
	}
}

// WikiBacklinksCacheCleanupService is the sweeper. It owns the
// ticker loop and the in-process mutex that prevents two ticks from
// running concurrently on the same instance (e.g. when the period is
// shorter than the run time). Multi-instance safety is handled at the
// repo level via SELECT ... FOR UPDATE SKIP LOCKED.
type WikiBacklinksCacheCleanupService interface {
	// Start launches the sweeper goroutine. The first tick fires
	// after `Period` (not immediately) so the service has time to
	// finish warm-up — see D2. Idempotent: a second Start is a no-op.
	Start(ctx context.Context)

	// RunOnce performs one cleanup pass. Public so an admin endpoint
	// or smoke script can trigger an out-of-band sweep without
	// waiting for the next tick. Returns (deletedRows, dryRun,
	// durationMs, error).
	RunOnce(ctx context.Context) (int64, bool, int64, error)
}

// defaultWikiBacklinksCacheCleanupService is the production
// implementation. Depends on:
//   - the GORM DB (for the SELECT FOR UPDATE SKIP LOCKED transaction)
//   - the cache repo (for DeleteStale + CountRows)
type defaultWikiBacklinksCacheCleanupService struct {
	cfg     WikiBacklinksCacheCleanupConfig
	db      *gorm.DB
	repo    interfaces.WikiBacklinksCacheRepository
	mu      sync.Mutex // in-process: prevents concurrent ticks
	running bool       // Start is idempotent
	stopped chan struct{}
}

// NewWikiBacklinksCacheCleanupService wires the sweeper into the DI
// container. Returns the interface.
func NewWikiBacklinksCacheCleanupService(
	cfg WikiBacklinksCacheCleanupConfig,
	db *gorm.DB,
	repo interfaces.WikiBacklinksCacheRepository,
) WikiBacklinksCacheCleanupService {
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultWikiBacklinksCacheCleanupConfig().TTL
	}
	if cfg.Period <= 0 {
		cfg.Period = DefaultWikiBacklinksCacheCleanupConfig().Period
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultWikiBacklinksCacheCleanupConfig().BatchSize
	}
	if cfg.MaxRows <= 0 {
		cfg.MaxRows = DefaultWikiBacklinksCacheCleanupConfig().MaxRows
	}
	return &defaultWikiBacklinksCacheCleanupService{
		cfg:     cfg,
		db:      db,
		repo:    repo,
		stopped: make(chan struct{}),
	}
}

// Start launches the ticker loop. The first tick fires after one
// Period so the service has time to settle. To trigger an immediate
// first pass (e.g. after a deploy that filled the cache table),
// call RunOnce directly — that's what smoke-wiki-cache-cleanup.sh
// does in its force-mode path.
func (s *defaultWikiBacklinksCacheCleanupService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(s.cfg.Period)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[wiki-cache-cleanup] context cancelled, stopping sweeper")
				return
			case <-ticker.C:
				if _, _, _, err := s.RunOnce(ctx); err != nil {
					log.Printf("[wiki-cache-cleanup] RunOnce error: %v", err)
				}
			}
		}
	}()
	log.Printf("[wiki-cache-cleanup] sweeper started: period=%s ttl=%s batch=%d dry_run=%v max_rows=%d",
		s.cfg.Period, s.cfg.TTL, s.cfg.BatchSize, s.cfg.DryRun, s.cfg.MaxRows)
}

// RunOnce performs one cleanup pass. The (deletedRows, dryRun,
// durationMs, error) return lets the smoke script assert outcomes
// without parsing log lines.
//
// Drain loop: we keep calling DeleteStale until RowsAffected < limit
// (meaning the stale set is drained). Each batch is its own
// transaction so a long-running drain doesn't hold one giant tx open.
// Safety: if ctx is cancelled mid-drain, we return immediately with
// the partial count — the next tick will resume.
func (s *defaultWikiBacklinksCacheCleanupService) RunOnce(ctx context.Context) (int64, bool, int64, error) {
	// In-process mutex: even if the ticker fires while a previous
	// RunOnce is still draining (slow DB, big stale set), we wait
	// rather than fork two sweeps. They would both observe a partial
	// stale set and add up to the right answer, but doubling DB load
	// for no reason is wasteful.
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	before := start.Add(-s.cfg.TTL)

	if s.cfg.DryRun {
		// Dry-run path: count via the SELECT FOR UPDATE SKIP LOCKED
		// path but never DELETE. We open a tx to take the locks
		// briefly, count, then rollback — the locks release at
		// rollback and we never touch rows.
		count, err := s.countStale(ctx, before)
		if err != nil {
			// Drain the gauge read best-effort — we still want
			// operators to see the current row count even if the
			// stale count fails.
			s.refreshGauge(ctx)
			return 0, true, time.Since(start).Milliseconds(), err
		}
		s.refreshGauge(ctx)
		metricCleanupDryRunTotal.Inc()
		log.Printf("[wiki-cache-cleanup] DRY-RUN: %d stale rows would be deleted (ttl=%s before=%s)",
			count, s.cfg.TTL, before.Format(time.RFC3339))
		return int64(count), true, time.Since(start).Milliseconds(), nil
	}

	// Real cleanup path: drain in batches until RowsAffected < batch
	// size or ctx is cancelled.
	//
	// Build #23 — every batch that actually deletes rows bumps
	// wiki_cache_invalidations_total{op=cleanup_sweep} (so Prom and
	// the audit log stay aligned with the existing 7 write-path ops)
	// and writes a best-effort row to wiki_backlinks_cache_invalidation_log.
	// The KbID "system" + Slug "*" sentinels mark these rows as
	// sweeper-originated; operators can grep for op=cleanup_sweep to
	// separate them from page-write invalidations. Failures from
	// LogInvalidation are logged but never abort the sweep — losing
	// one audit row must not block row deletion.
	var batchIndex int
	var totalDeleted int64
	for {
		if err := ctx.Err(); err != nil {
			return totalDeleted, false, time.Since(start).Milliseconds(), err
		}
		deleted, err := s.repo.DeleteStale(ctx, before, s.cfg.BatchSize)
		if err != nil {
			s.refreshGauge(ctx)
			return totalDeleted, false, time.Since(start).Milliseconds(), err
		}
		totalDeleted += deleted
		metricCleanupDeletedTotal.Add(float64(deleted))
		if deleted > 0 {
			metricCacheInvalidationsTotal.
				WithLabelValues(string(types.BacklinkCacheInvalidateSweep)).Inc()
			detailsJSON, _ := json.Marshal(map[string]any{
				"ttl_seconds":    s.cfg.TTL.Seconds(),
				"period_seconds": s.cfg.Period.Seconds(),
				"batch_size":     s.cfg.BatchSize,
				"batch_index":    batchIndex,
				"before":         before.Format(time.RFC3339),
			})
			entry := &types.WikiBacklinksCacheInvalidationLogEntry{
				KbID: "system",
				Slug: "*",
				Op:   string(types.BacklinkCacheInvalidateSweep),
				// Build #25 — sweeper runs without an HTTP request, so
				// ctx has no X-Request-ID. Stamping a stable
				// `sweep:sweeper` keeps every row this loop writes
				// joined under one correlation_id; the audit drawer
				// surfaces them as one group instead of N unjoined
				// rows. The jobID literal is the loop's identity —
				// `WithBackgroundCorrelationID` will append a fresh
				// UUID instead if we ever pass an empty string.
				CorrelationID: types.CorrelationIDFromContext(types.WithBackgroundCorrelationID(ctx, types.BackgroundCorrelationSweep, "sweeper")),
				ActorUserID:   wikiActorUserIDFromContext(ctx),
				AffectedCount: int(deleted),
				Details:       string(detailsJSON),
			}
			if logErr := s.repo.LogInvalidation(ctx, entry); logErr != nil {
				log.Printf("[wiki-cache-cleanup] audit log insert failed (batch=%d deleted=%d): %v",
					batchIndex, deleted, logErr)
			}
			batchIndex++
		}
		if deleted < int64(s.cfg.BatchSize) {
			break
		}
	}
	s.refreshGauge(ctx)
	metricCleanupDurationSeconds.Observe(time.Since(start).Seconds())
	log.Printf("[wiki-cache-cleanup] deleted %d stale rows (ttl=%s before=%s duration=%dms)",
		totalDeleted, s.cfg.TTL, before.Format(time.RFC3339), time.Since(start).Milliseconds())
	return totalDeleted, false, time.Since(start).Milliseconds(), nil
}

// countStale opens a transaction, takes the row-level locks via
// SELECT ... FOR UPDATE SKIP LOCKED, and counts the rows it would
// touch. Rollback releases the locks. This is identical to what a
// real sweep would touch, so the count is authoritative.
func (s *defaultWikiBacklinksCacheCleanupService) countStale(
	ctx context.Context,
	before time.Time,
) (int64, error) {
	if s.db == nil {
		return 0, errors.New("wiki-cache-cleanup: nil db (set GORM DB on service)")
	}
	var count int64
	err := s.db.WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			keys, err := s.repo.ListStaleForUpdate(ctx, tx, before, s.cfg.BatchSize)
			if err != nil {
				return err
			}
			count = int64(len(keys))
			return nil
		})
	return count, err
}

// refreshGauge updates the cache_rows_remaining + backref_rows_remaining
// gauges with the current table sizes. Best-effort: a failure here is
// logged but does not abort the cleanup pass.
func (s *defaultWikiBacklinksCacheCleanupService) refreshGauge(ctx context.Context) {
	if s.repo == nil {
		return
	}
	count, err := s.repo.CountRows(ctx)
	if err != nil {
		log.Printf("[wiki-cache-cleanup] gauge refresh failed: %v", err)
		return
	}
	metricCacheRowsRemaining.Set(float64(count))
	// Build #26 — also refresh the backref gauge. The repo updates
	// this incrementally on Upsert / Delete / DeleteByKB / DeleteStale,
	// but a drift (e.g. rolled-back tx, manual SQL) gets corrected at
	// the next sweep.
	backrefCount, err := s.repo.CountBackrefRows(ctx)
	if err != nil {
		log.Printf("[wiki-cache-cleanup] backref gauge refresh failed: %v", err)
		return
	}
	wikicachemetrics.BackrefRows.Set(float64(backrefCount))
}
