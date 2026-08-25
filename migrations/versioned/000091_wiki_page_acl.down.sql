DROP INDEX IF EXISTS idx_wiki_page_acl_audit_kb_slug;
DROP TABLE IF EXISTS wiki_page_acl_audit;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS acl_revision;
ALTER TABLE wiki_pages DROP COLUMN IF EXISTS acl;