-- v0.7.34 Build #42 — sqlite mirror of 000138.
CREATE TABLE doc_kg_relations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    INTEGER NOT NULL,
    source_type  TEXT NOT NULL,
    source_id    TEXT NOT NULL,
    target_type  TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'mentions_entity',
    confidence   REAL NOT NULL DEFAULT 1.0,
    anchor       TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_doc_kg_relations_natural ON doc_kg_relations(source_type, source_id, target_type, target_id, kind);
CREATE INDEX idx_doc_kg_relations_tenant_source ON doc_kg_relations(tenant_id, source_type, source_id);
CREATE INDEX idx_doc_kg_relations_tenant_target ON doc_kg_relations(tenant_id, target_type, target_id);
CREATE INDEX idx_doc_kg_relations_kind ON doc_kg_relations(kind);

CREATE TABLE kb_wiki_references (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     INTEGER NOT NULL,
    kb_chunk_id   TEXT NOT NULL,
    wiki_page_id  TEXT NOT NULL,
    anchor        TEXT NOT NULL DEFAULT '',
    citation_ctx  TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_kb_wiki_references_natural ON kb_wiki_references(kb_chunk_id, wiki_page_id);
CREATE INDEX idx_kb_wiki_references_chunk ON kb_wiki_references(kb_chunk_id);
CREATE INDEX idx_kb_wiki_references_page ON kb_wiki_references(wiki_page_id);
CREATE INDEX idx_kb_wiki_references_tenant ON kb_wiki_references(tenant_id);

CREATE TABLE inline_kb_refs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    INTEGER NOT NULL,
    wiki_page_id TEXT NOT NULL,
    kb_chunk_id  TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'text',
    anchor       TEXT NOT NULL DEFAULT '',
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_inline_kb_refs_natural ON inline_kb_refs(wiki_page_id, kb_chunk_id, kind);
CREATE INDEX idx_inline_kb_refs_page_position ON inline_kb_refs(wiki_page_id, position ASC);
CREATE INDEX idx_inline_kb_refs_chunk ON inline_kb_refs(kb_chunk_id);
CREATE INDEX idx_inline_kb_refs_tenant ON inline_kb_refs(tenant_id);
