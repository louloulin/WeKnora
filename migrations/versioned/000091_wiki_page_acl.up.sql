-- Migration 000091: wiki_pages.acl + wiki_page_acl_audit
--
-- Adds page-level access control for Build #7. The column carries a JSON
-- struct mirroring the frontend contract in frontend/src/api/wiki/acl.ts:
--
--   {
--     "mode": "inherit" | "private" | "allow_list",
--     "allow_user_ids":  [...],
--     "allow_group_ids": [...],
--     "deny_inherited":  bool,
--     "revision":        int,    -- optimistic-lock, server-stamped
--     "updated_at":      string  -- RFC3339
--   }
--
-- Default NULL means "inherit": every KB member can read the page. Decision
-- logic lives in WikiAclService.Resolve and is independent of the column's
-- presence; legacy rows automatically fall through to the inherit branch
-- without any backfill.
--
-- Forward-only contract:
--   * up adds the column as JSON NULL (no default — historical pages stay NULL).
--   * up also creates the audit table for forensics.
--   * down drops both objects. No data migration: nothing else references
--     them yet (read-path integration is wired in this same Build).
--
-- Why JSON and not JSONB: the ACL payload is a small bag of scalars and
-- string arrays. JSONB would buy us indexability we don't use (we always
-- fetch the full row by (kb, slug) PK). Plain JSON matches the rest of the
-- row's GORM mapping and keeps the Value/Scan driver pattern identical to
-- WikiConfig (internal/types/wiki_page.go:603-617).

ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS acl JSON;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS acl_revision BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS wiki_page_acl_audit (
    id BIGSERIAL PRIMARY KEY,
    wiki_page_id BIGINT,
    knowledge_base_id UUID NOT NULL,
    slug TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    before_acl JSONB,
    after_acl JSONB,
    action TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_acl_audit_kb_slug
    ON wiki_page_acl_audit (knowledge_base_id, slug, created_at DESC);