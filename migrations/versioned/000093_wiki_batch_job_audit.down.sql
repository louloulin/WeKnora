-- Build #14 — rollback
DROP INDEX IF EXISTS idx_wbb_audit_action;
DROP INDEX IF EXISTS idx_wbb_audit_actor;
DROP INDEX IF EXISTS idx_wbb_audit_job;
DROP INDEX IF EXISTS idx_wbb_audit_kb_occurred;
DROP TABLE IF EXISTS wiki_batch_job_audit;