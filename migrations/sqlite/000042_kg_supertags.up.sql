-- v0.7.29 Build #35 — sqlite mirror of 000135.
CREATE TABLE supertags (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    kb_id           TEXT NOT NULL,
    name            TEXT NOT NULL,
    color           TEXT NOT NULL DEFAULT '',
    schema          TEXT NOT NULL DEFAULT '[]',
    icon            TEXT NOT NULL DEFAULT '',
    child_supertag  INTEGER NOT NULL DEFAULT 0,
    autofill_model  TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE kg_entities (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    kb_id           TEXT NOT NULL,
    supertag_id     TEXT,
    name            TEXT NOT NULL,
    properties      TEXT NOT NULL DEFAULT '{}',
    embeddings      BLOB,
    first_seen_doc  TEXT,
    last_seen_doc   TEXT,
    occurrence      INTEGER NOT NULL DEFAULT 1,
    trust_score     REAL NOT NULL DEFAULT 1.0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE entity_relations (
    id              TEXT PRIMARY KEY,
    src_entity_id   TEXT NOT NULL,
    dst_entity_id   TEXT NOT NULL,
    relation        TEXT NOT NULL,
    weight          REAL NOT NULL DEFAULT 1.0,
    evidence_docs   TEXT NOT NULL DEFAULT '[]',
    confidence      REAL NOT NULL DEFAULT 1.0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (src_entity_id, dst_entity_id, relation)
);

CREATE TABLE supertag_commands (
    id              TEXT PRIMARY KEY,
    supertag_id     TEXT NOT NULL,
    event           TEXT NOT NULL,
    automation_id   TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_supertags_tenant_kb ON supertags(tenant_id, kb_id);
CREATE INDEX idx_kg_entities_tenant_kb ON kg_entities(tenant_id, kb_id);
CREATE INDEX idx_kg_entities_supertag ON kg_entities(supertag_id);
CREATE INDEX idx_kg_entities_name ON kg_entities(name);
CREATE INDEX idx_relations_src ON entity_relations(src_entity_id);
CREATE INDEX idx_relations_dst ON entity_relations(dst_entity_id);
CREATE INDEX idx_supertag_commands ON supertag_commands(supertag_id, event);
