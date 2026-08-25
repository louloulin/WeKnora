-- Migration 000095 down: drop wiki_collab_awareness
--
-- Awareness rows are a presence cache. Dropping the table resets the
-- "recent collaborators" UI to empty until live awareness repopulates
-- it. No wiki_pages row is affected.

DROP TABLE IF EXISTS wiki_collab_awareness;
