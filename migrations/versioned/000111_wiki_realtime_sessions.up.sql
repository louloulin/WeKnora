-- v0.7.19 — wiki_realtime_sessions
--
-- Background: companion table to wiki_doc_snapshots. Holds the live
-- awareness/presence state for connected collaborators on a single wiki page.
-- Each row is one (user, client) pair; the application layer sweeps stale
-- rows (last_heartbeat older than 30s) on a background ticker.
--
-- Schema decisions:
--   • id = UUID — one presence identity per (user, clientId) pair
--   • client_id BIGINT — Yjs clientID, the CRDT's per-tab identity
--   • color VARCHAR(16) — presence cursor color (CSS hex or short name)
--   • display_name VARCHAR(128) — denormalized for fast presence panel reads
--   • last_heartbeat — updated every ~10s while WS open; sweep every 30s
--   • Composite index on (page_id, last_heartbeat) for cheap presence joins
--
-- Rollback: 000111_wiki_realtime_sessions.down.sql drops the table.
CREATE TABLE IF NOT EXISTS wiki_realtime_sessions (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    page_id         VARCHAR(36)  NOT NULL,
    user_id         BIGINT       NOT NULL,
    client_id       BIGINT       NOT NULL,
    color           VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    last_heartbeat  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    joined_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, page_id, client_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_realtime_sessions_page
    ON wiki_realtime_sessions (page_id, last_heartbeat DESC);

CREATE INDEX IF NOT EXISTS idx_wiki_realtime_sessions_user
    ON wiki_realtime_sessions (user_id, last_heartbeat DESC);

CREATE INDEX IF NOT EXISTS idx_wiki_realtime_sessions_heartbeat
    ON wiki_realtime_sessions (last_heartbeat);
