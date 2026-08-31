-- SQLite counterpart of 000115_wiki_kb_references. SQLite does not
-- support partial indexes on every version we target, so the WHERE
-- deleted_at IS NULL clause is omitted. Application code keeps the
-- soft-delete predicate; the index just covers the live rows.
CREATE TABLE IF NOT EXISTS wiki_kb_references (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36)  NOT NULL,
    wiki_page_id    VARCHAR(36)  NOT NULL,
    knowledge_id    VARCHAR(36)  NOT NULL,
    reference_label VARCHAR(256) NOT NULL DEFAULT '',
    created_by      VARCHAR(36)  NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME,
    CONSTRAINT uq_wiki_kb_reference UNIQUE (wiki_page_id, knowledge_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_knowledge ON wiki_kb_references (knowledge_id);
CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_wiki_page  ON wiki_kb_references (wiki_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_tenant     ON wiki_kb_references (tenant_id);
