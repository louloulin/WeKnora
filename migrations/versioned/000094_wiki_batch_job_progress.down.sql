-- 000094_wiki_batch_job_progress.down.sql

DROP INDEX IF EXISTS idx_wbbf_kb_code;
DROP INDEX IF EXISTS idx_wbbf_job;
DROP TABLE IF EXISTS wiki_batch_job_failures;

ALTER TABLE wiki_batch_jobs
    DROP COLUMN IF EXISTS progress;