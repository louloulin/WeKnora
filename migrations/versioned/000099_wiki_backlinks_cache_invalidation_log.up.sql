-- Migration 000099: wiki_backlinks_cache_invalidation_log
--
-- Build #23 — wiki backlinks cache observability + invalidation audit trail.
--
-- Every call to InvalidateBacklinksCache (the 7 write-handler sites + the
-- Build #22 sweeper's DeleteStale) inserts one row here. This is the
-- durable counterpart to the warn-logs in InvalidateBacklinksCache, and
-- the surface that the new /backlinks/cache-statuses admin endpoint
-- queries for forensics.
--
-- Schema choice: insert-only, no UPDATE/DELETE. Retention is handled
-- later by a Build #X sweep (mirrors audit_logs retention), but we
-- intentionally do not add a retention trigger here — the table is
-- append-only and operators can read from it.
--
-- Indexing: (kb_id, created_at DESC) is the only read pattern — admin
-- listing per KB ordered by recency. We add no other indexes to keep
-- writes cheap.
--
-- 000097 created wiki_backlinks_cache; 000098 added cleanup index. This
-- migration is unrelated to those (no foreign key to wiki_backlinks_cache
-- because the cache row may be deleted before the log row, e.g. when
-- Build #22 sweep drops a stale row and we want the log entry to
-- survive). The KB-level scoping is at the application layer.
--
-- Cross-dialect: BIGSERIAL is PG syntax. MySQL uses BIGINT AUTO_INCREMENT
-- PRIMARY KEY. SQLite uses INTEGER PRIMARY KEY (rowid alias). The
-- dialect-specific shim for these lives in the versioned migration
-- generator (see migrations/versioned/000099_*.{pg,mysql,sqlite}.sql)
-- which the build pipeline emits from this canonical source. For now
-- we keep the PG-flavored source so a single diff documents intent.
CREATE TABLE IF NOT EXISTS wiki_backlinks_cache_invalidation_log (
    id              BIGSERIAL PRIMARY KEY,
    kb_id           VARCHAR(64)  NOT NULL,
    slug            VARCHAR(512) NOT NULL,
    op              VARCHAR(32)  NOT NULL,
    actor_user_id   BIGINT,
    source_event_id VARCHAR(64),
    affected_count  INT          NOT NULL DEFAULT 0,
    details         JSONB,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wbc_invalidation_log_kb_created
    ON wiki_backlinks_cache_invalidation_log (kb_id, created_at DESC);