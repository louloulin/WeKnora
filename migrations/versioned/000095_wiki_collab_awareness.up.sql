-- Migration 000095: wiki_collab_awareness — y-protocol presence cache
--
-- Stores a 24h rolling window of awareness frames per (room_key,
-- client_id) so newly-joined clients can render "who was here
-- recently" before the live awareness channel populates. The frame
-- payload is preserved verbatim (for future y-protocol-level replay)
-- alongside a denormalized {display_name, color} summary that the
-- "recent collaborators" list reads without parsing.
--
-- Forward-only contract:
--   * up creates the table on all supported dialects (SQLite / MySQL /
--     Postgres). JSONB on Postgres / MySQL 8; TEXT on SQLite.
--   * down drops the table. Awareness rows are pure cache — losing
--     them only resets the "recent collaborators" UI to empty until
--     the live channel populates again.

CREATE TABLE IF NOT EXISTS wiki_collab_awareness (
    room_key      TEXT      NOT NULL,
    client_id     TEXT      NOT NULL,
    user_id       TEXT      NOT NULL,
    display_name  TEXT      NOT NULL DEFAULT '',
    color         TEXT      NOT NULL DEFAULT '',
    payload       TEXT      NOT NULL,           -- SQLite/Postgres TEXT (Postgres ::jsonb at query time)
    last_seen_at  TIMESTAMP NOT NULL,
    expires_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (room_key, client_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_collab_awareness_recent
    ON wiki_collab_awareness (room_key, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_wiki_collab_awareness_sweep
    ON wiki_collab_awareness (expires_at);
