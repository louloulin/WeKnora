-- v0.7.35 — collab_doc_comments: cross-doc comment threads for DOC / PPT / SHEET.
--
-- A thread is anchored to a position in the document (paragraph index,
-- shape id, cell ref) and carries one or more replies. Threaded comments
-- match the Feishu / Tencent doc / Notion model: pick a region, drop a
-- comment, others can reply. `anchor_type` discriminates the surface
-- (`doc` = paragraph-range, `slide` = shape id, `sheet` = cell ref).
-- `anchor_ref` carries a JSON blob specific to the type.
CREATE TABLE IF NOT EXISTS collab_doc_comments (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    thread_id       VARCHAR(36)  NOT NULL,
    parent_id       INTEGER      REFERENCES collab_doc_comments(id) ON DELETE CASCADE,
    author_user_id  INTEGER      NOT NULL,
    author_name     VARCHAR(128) NOT NULL DEFAULT '',
    author_color    VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    anchor_type     VARCHAR(16)  NOT NULL,
    anchor_ref      TEXT         NOT NULL DEFAULT '',
    body            TEXT         NOT NULL,
    resolved        INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_comments_doc ON collab_doc_comments (doc_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_comments_thread ON collab_doc_comments (thread_id, created_at);
CREATE INDEX IF NOT EXISTS idx_collab_doc_comments_tenant ON collab_doc_comments (tenant_id);
