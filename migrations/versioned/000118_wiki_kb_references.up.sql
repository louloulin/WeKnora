-- Wiki Page ↔ Knowledge Base Document bidirectional references.
-- A wiki page may embed KB document references via the [[kb:<id>]]
-- markdown syntax; the wiki_kb_references row records the binding so
-- that:
--   * the KB document viewer can show "Mentioned in Wiki Pages" backlinks,
--   * the wiki page viewer can render each reference as a live card,
--   * deleting the page or the KB document cascades the reference row.
--
-- One row per (wiki_page, knowledge) pair. The reference_label is the
-- human-readable label that the author typed into the wiki content at
-- the moment the backfill ran; it is kept for forensics even after the
-- KB document title changes, so an audit can still tell "page X used
-- to say 'Foo release notes'".
--
-- tenant_id is denormalised on the row so a tenant-scoped list query
-- does not have to join through either side. The soft-delete on
-- either side does NOT remove the row — instead the resolver returns
-- the tombstone status and the UI renders a "deleted" badge. Hard
-- deletes (admin GDPR purge) DO cascade via ON DELETE CASCADE.
CREATE TABLE IF NOT EXISTS wiki_kb_references (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36)  NOT NULL,
    wiki_page_id    VARCHAR(36)  NOT NULL,
    knowledge_id    VARCHAR(36)  NOT NULL,
    reference_label VARCHAR(256) NOT NULL DEFAULT '',
    created_by      VARCHAR(36)  NOT NULL DEFAULT '',
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP,
    CONSTRAINT uq_wiki_kb_reference UNIQUE (wiki_page_id, knowledge_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_knowledge
    ON wiki_kb_references (knowledge_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_wiki_page
    ON wiki_kb_references (wiki_page_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_wiki_kb_ref_tenant
    ON wiki_kb_references (tenant_id) WHERE deleted_at IS NULL;
