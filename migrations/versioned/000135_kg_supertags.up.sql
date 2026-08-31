-- v0.7.29 Build #35 — Knowledge Graph + KGSupertags foundation.
CREATE TABLE supertags (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    kb_id           VARCHAR(36) NOT NULL,
    name            VARCHAR(128) NOT NULL,
    color           VARCHAR(16) NOT NULL DEFAULT '',
    schema          JSONB NOT NULL DEFAULT '[]'::jsonb,
    icon            VARCHAR(64) NOT NULL DEFAULT '',
    child_supertag  BOOLEAN NOT NULL DEFAULT false,
    autofill_model  VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE kg_entities (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    kb_id           VARCHAR(36) NOT NULL,
    supertag_id     VARCHAR(36),
    name            VARCHAR(256) NOT NULL,
    properties      JSONB NOT NULL DEFAULT '{}'::jsonb,
    embeddings      BYTEA,
    first_seen_doc  VARCHAR(36),
    last_seen_doc   VARCHAR(36),
    occurrence      INTEGER NOT NULL DEFAULT 1,
    trust_score     REAL NOT NULL DEFAULT 1.0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE entity_relations (
    id              VARCHAR(36) PRIMARY KEY,
    src_entity_id   VARCHAR(36) NOT NULL,
    dst_entity_id   VARCHAR(36) NOT NULL,
    relation        VARCHAR(64) NOT NULL,
    weight          REAL NOT NULL DEFAULT 1.0,
    evidence_docs   JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence      REAL NOT NULL DEFAULT 1.0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (src_entity_id, dst_entity_id, relation)
);

CREATE TABLE supertag_commands (
    id              VARCHAR(36) PRIMARY KEY,
    supertag_id     VARCHAR(36) NOT NULL,
    event           VARCHAR(32) NOT NULL,
    automation_id   VARCHAR(36) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_supertags_tenant_kb ON supertags(tenant_id, kb_id);
CREATE INDEX idx_kg_entities_tenant_kb ON kg_entities(tenant_id, kb_id);
CREATE INDEX idx_kg_entities_supertag ON kg_entities(supertag_id);
CREATE INDEX idx_kg_entities_name ON kg_entities(name);
CREATE INDEX idx_relations_src ON entity_relations(src_entity_id);
CREATE INDEX idx_relations_dst ON entity_relations(dst_entity_id);
CREATE INDEX idx_supertag_commands ON supertag_commands(supertag_id, event);
