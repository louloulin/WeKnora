-- Migration 000097: wiki_backlinks_cache
--
-- Build #21 — wiki backlinks graph cache (write-time invalidation).
-- Persists the 4-section payload returned by GET /pages/:slug/backlinks/graph
-- (Build #20) so repeat reads are ~5ms instead of recomputing Jaccard +
-- 2-hop indirect + broken detection on every panel open.
--
-- Each row = one (kb_id, slug) page's precomputed graph. Payload is stored
-- as JSON columns — sections are small (typical: 50 indirect, 10 related,
-- 20 broken) and never queried row-by-row, so JSON columns win over a
-- child-row schema. source_event_id records which wiki_event produced
-- this snapshot (debug / staleness diagnostics).
--
-- Stale rows are cleared by wikiPageService.InvalidateBacklinksCache on
-- every CreatePage / UpdatePage / DeletePage / MovePage and every
-- WikiBatch* completed event. The read path uses ListBacklinkGraph
-- (cache-first): hit → return, miss → recompute + upsert.

CREATE TABLE IF NOT EXISTS wiki_backlinks_cache (
    kb_id          VARCHAR(64)  NOT NULL,
    slug           VARCHAR(512) NOT NULL,
    direct_json    TEXT         NOT NULL,
    indirect_json  TEXT         NOT NULL,
    related_json   TEXT         NOT NULL,
    broken_json    TEXT         NOT NULL,
    stats_json     TEXT         NOT NULL,
    source_event_id VARCHAR(64),
    computed_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (kb_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_wiki_backlinks_cache_kb_updated
    ON wiki_backlinks_cache (kb_id, updated_at);