-- B31 — Eval System rollback
DROP INDEX IF EXISTS idx_eval_badcases_run;
DROP INDEX IF EXISTS idx_eval_badcases_tenant;
DROP TABLE IF EXISTS eval_badcases;

DROP INDEX IF EXISTS idx_eval_run_results_failed;
DROP INDEX IF EXISTS idx_eval_run_results_run;
DROP TABLE IF EXISTS eval_run_results;

DROP INDEX IF EXISTS idx_eval_runs_correlation;
DROP INDEX IF EXISTS idx_eval_runs_dataset;
DROP INDEX IF EXISTS idx_eval_runs_tenant;
DROP TABLE IF EXISTS eval_runs;

DROP INDEX IF EXISTS idx_eval_dataset_qa_dataset;
DROP TABLE IF EXISTS eval_dataset_qa;

DROP INDEX IF EXISTS idx_eval_datasets_tenant;
DROP TABLE IF EXISTS eval_datasets;