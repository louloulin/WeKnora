-- Rollback for 000132_collaborative_docs
DROP TRIGGER IF EXISTS trg_collab_doc_snapshots_updated_at ON collab_doc_snapshots;
DROP FUNCTION IF EXISTS collab_doc_snapshots_set_updated_at();
DROP TABLE IF EXISTS collab_doc_sessions;
DROP TABLE IF EXISTS collab_doc_snapshots;
DROP TABLE IF EXISTS collaborative_docs;
