-- Migration 000095: wiki_tags
--
-- Build #17 — wiki page tags (P1.1). Mirrors the KnowledgeTag
-- pattern (internal/types/tag.go) but the KB field is wiki's UUID.
-- Two tables: tag definitions + per-page associations.
--
-- Fields:
--   * id UUID — exposed to the client for picker / detail.
--   * knowledge_base_id — every tag is KB-scoped; cross-KB reads return 404.
--   * name — UNIQUE within (knowledge_base_id, name); user-facing label.
--   * color — one of 8 hard-coded palette entries (frontend enforces).
--   * sort_order — panel display order; lower = first.
--
-- wiki_page_tags is the many-to-many join. Cascade delete at the DB level
-- keeps the join table clean when a tag definition is removed; the service
-- layer still wipes wiki_page_tags inside DeletePage for safety even though
-- no DB FK is declared (wiki_page_id is a logical slug, not a UUID FK, so
-- the database cannot enforce it).

CREATE TABLE IF NOT EXISTS wiki_tags (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    name VARCHAR(64) NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT 'blue',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_tags_kb_name
    ON wiki_tags (knowledge_base_id, name);

CREATE INDEX IF NOT EXISTS idx_wiki_tags_kb_sort
    ON wiki_tags (knowledge_base_id, sort_order, name);

CREATE TABLE IF NOT EXISTS wiki_page_tags (
    wiki_tag_id UUID NOT NULL REFERENCES wiki_tags(id) ON DELETE CASCADE,
    wiki_page_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (wiki_tag_id, wiki_page_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_tags_page
    ON wiki_page_tags (wiki_page_id);

CREATE INDEX IF NOT EXISTS idx_wiki_page_tags_tag
    ON wiki_page_tags (wiki_tag_id);