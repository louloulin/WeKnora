-- v0.7.38 Build #45.a — Daily Note 默认页 (P0 gap #12).
--
-- One row per (user, kb, calendar_date) so the user always lands on
-- "today's note" without having to remember to create one. The
-- linked wiki_pages row (page_id) is created lazily on first GET;
-- keeping it nullable here means a write-only path (PATCH) doesn't
-- have to materialize the full wiki surface.
--
-- Indexes:
--   * uq_user_daily_notes_user_kb_date — uniqueness + per-user listing
--   * idx_user_daily_notes_user_date   — covers the dashboard widget
--                                         "recent notes" query
--   * idx_user_daily_notes_kb_date     — covers the KB-scoped admin
--                                         "team daily notes" view
CREATE TABLE IF NOT EXISTS user_daily_notes (
    id                  VARCHAR(36)  PRIMARY KEY,
    tenant_id           INTEGER      NOT NULL,
    user_id             VARCHAR(64)  NOT NULL,
    knowledge_base_id   VARCHAR(36)  NOT NULL,
    note_date           DATE         NOT NULL,
    slug                VARCHAR(255) NOT NULL DEFAULT '',
    page_id             VARCHAR(36)  NOT NULL DEFAULT '',
    title               VARCHAR(255) NOT NULL DEFAULT '',
    content             TEXT         NOT NULL DEFAULT '',
    summary             VARCHAR(512) NOT NULL DEFAULT '',
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, knowledge_base_id, note_date)
);
CREATE INDEX IF NOT EXISTS idx_user_daily_notes_user_date ON user_daily_notes (user_id, note_date DESC);
CREATE INDEX IF NOT EXISTS idx_user_daily_notes_kb_date   ON user_daily_notes (knowledge_base_id, note_date DESC);
