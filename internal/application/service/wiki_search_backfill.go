package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// WikiSearchBackfillFunc is the function signature cmd/server uses to
// invoke the content_ts_zh backfill loop. Returning a struct of stats keeps
// the synchronous path observable in tests; the async path returns zero and
// logs the final tally via a separate goroutine.
type WikiSearchBackfillFunc func(ctx context.Context, async bool) WikiSearchBackfillStats

// WikiSearchBackfillStats reports what one backfill run did.
type WikiSearchBackfillStats struct {
	Total    int
	Updated  int64
	Skipped  int64
	Failed   int64
	Duration time.Duration
}

// WikiSearchBackfillRepository is the minimal surface the backfill loop
// needs. `WikiPageRepository` already satisfies it, but defining a tiny
// interface here keeps the helper testable with an in-memory fake and
// prevents the backfill loop from accidentally growing into a "call every
// repo method" shape.
type WikiSearchBackfillRepository interface {
	FindPagesMissingTSZh(ctx context.Context, limit int) ([]*types.WikiPage, error)
	UpdateContentTSZh(ctx context.Context, id string, tsZh string) error
}

// BackfillContentTSZh walks every wiki_pages row whose `content_ts_zh` is
// empty/NULL and re-tokenizes title+content via gojieba. Designed to be
// called once from cmd/server after migrations have applied 000096.
//
// Loop characteristics:
//   - 200 rows per batch (matches the existing wiki batch-job batch size so
//     backfill throughput is comparable to ingest).
//   - 2 concurrent workers (same as the wiki batch-job pool — anything
//     higher risks starving request handlers that share the connection
//     pool).
//   - jieba is a single global thread-safe instance; no extra coordination
//     needed.
//   - Per-row errors are logged but never abort the loop: a single bad row
//     should not strand the rest of the wiki unsearchable in Chinese.
//
// When `async` is true the function returns immediately with zero stats and
// the actual work happens in a background goroutine; the final tally is
// logged when the goroutine exits. When false the call blocks until the
// table has been fully drained — used by tests.
func BackfillContentTSZh(
	ctx context.Context,
	repo WikiSearchBackfillRepository,
	async bool,
) WikiSearchBackfillStats {
	start := time.Now()
	const pageSize = 200

	if repo == nil {
		logger.Warnf(ctx, "[wiki-search-backfill] nil repo, skipping")
		return WikiSearchBackfillStats{}
	}

	if async {
		go func() {
			stats := drainContentTSZh(ctx, repo, pageSize, start)
			logger.Infof(context.Background(),
				"[wiki-search-backfill] async done total=%d updated=%d skipped=%d failed=%d duration=%s",
				stats.Total,
				stats.Updated,
				stats.Skipped,
				stats.Failed,
				stats.Duration,
			)
		}()
		return WikiSearchBackfillStats{}
	}

	return drainContentTSZh(ctx, repo, pageSize, start)
}

// drainContentTSZh is the synchronous core. Returns when the table has no
// rows left with content_ts_zh == NULL/empty OR the context is cancelled.
func drainContentTSZh(
	ctx context.Context,
	repo WikiSearchBackfillRepository,
	pageSize int,
	start time.Time,
) WikiSearchBackfillStats {
	var updated, skipped, failed, total int64

	for {
		select {
		case <-ctx.Done():
			return WikiSearchBackfillStats{
				Total:    int(atomic.LoadInt64(&total)),
				Updated:  atomic.LoadInt64(&updated),
				Skipped:  atomic.LoadInt64(&skipped),
				Failed:   atomic.LoadInt64(&failed),
				Duration: time.Since(start),
			}
		default:
		}

		pages, err := repo.FindPagesMissingTSZh(ctx, pageSize)
		if err != nil {
			atomic.AddInt64(&failed, 1)
			logger.Warnf(ctx, "[wiki-search-backfill] find batch failed: %v", err)
			break
		}
		if len(pages) == 0 {
			break
		}
		atomic.AddInt64(&total, int64(len(pages)))

		for _, p := range pages {
			tokens := JiebaSegmentForSearch(p.Title, p.Content)
			if tokens == "" {
				atomic.AddInt64(&skipped, 1)
				continue
			}
			if err := repo.UpdateContentTSZh(ctx, p.ID, tokens); err != nil {
				atomic.AddInt64(&failed, 1)
				logger.Warnf(ctx, "[wiki-search-backfill] update %s failed: %v", p.ID, err)
				continue
			}
			atomic.AddInt64(&updated, 1)
		}

		if len(pages) < pageSize {
			break
		}
	}

	return WikiSearchBackfillStats{
		Total:    int(atomic.LoadInt64(&total)),
		Updated:  atomic.LoadInt64(&updated),
		Skipped:  atomic.LoadInt64(&skipped),
		Failed:   atomic.LoadInt64(&failed),
		Duration: time.Since(start),
	}
}
