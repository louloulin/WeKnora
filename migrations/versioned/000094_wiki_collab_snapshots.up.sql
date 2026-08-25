-- Migration 000094: wiki_collab_snapshots — Y.js CRDT snapshot persistence
--
-- Stores the accumulated CRDT frame buffer for each (kb_id, slug)
-- room. The Build #8 wiki real-time collaboration server fans out
-- y-protocol frames between clients in the same room and debounce-
-- persists the buffer here so late joiners can replay and reach the
-- current document state without depending on another live peer.
--
-- Why a separate table rather than reusing wiki_pages.content_html:
--   * CRDT frames are binary; storing them next to the human-readable
--     markdown would couple two unrelated lifecycles (HTML projection
--     vs CRDT replay buffer).
--   * The snapshot can grow past the markdown body when many offline
--     edits queue up; we don't want write amplification on the
--     canonical content row.
--   * page_version is already used by the page revision flow; reusing
--     it here would force the hub to bump the version on every CRDT
--     frame, surfacing a new revision per keystroke.
--
-- Forward-only contract:
--   * up creates the table on all supported dialects.
--   * down drops it. Existing collab sessions on the live hub will
--     start cold (no replay buffer) — that matches the original
--     pre-Build #8 behaviour so down-migration is non-destructive to
--     user-facing wiki features.

CREATE TABLE IF NOT EXISTS wiki_collab_snapshots (
    room_key      TEXT PRIMARY KEY,
    snapshot      BLOB         NOT NULL,
    version       BIGINT       NOT NULL DEFAULT 0,
    last_write_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_wiki_collab_snapshots_last_write_at
    ON wiki_collab_snapshots (last_write_at DESC);
