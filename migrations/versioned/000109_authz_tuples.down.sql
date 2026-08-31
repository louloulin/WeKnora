DROP TABLE IF EXISTS authz_tuples;
DROP INDEX IF EXISTS idx_datasource_creator;
ALTER TABLE datasources DROP COLUMN IF EXISTS creator_id;
