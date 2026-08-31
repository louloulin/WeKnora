-- v0.7.26 — collab_doc_files: the canonical byte payload per (doc, version).
--
-- Each upload of a .docx / .pptx / .xlsx writes a row here; the Yjs
-- collab_doc_snapshots row keeps the CRDT state in parallel. The frontend
-- editor pulls the latest row's bytes on open, mutates locally, and POSTs
-- a new version on save. version monotonically grows per doc_id so we can
-- do optimistic concurrency via the If-Match header.
CREATE TABLE IF NOT EXISTS collab_doc_files (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    format          VARCHAR(16)  NOT NULL,
    content         BLOB         NOT NULL,
    size_bytes      INTEGER      NOT NULL,
    sha256          VARCHAR(64)  NOT NULL DEFAULT '',
    version         INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (doc_id, version)
);
CREATE INDEX IF NOT EXISTS idx_collab_doc_files_doc ON collab_doc_files (doc_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_files_tenant ON collab_doc_files (tenant_id);
