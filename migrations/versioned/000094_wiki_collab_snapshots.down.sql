-- Migration 000094 down: drop wiki_collab_snapshots
--
-- The CRDT snapshot is purely a replay cache; dropping the table
-- forces the wiki collab hub to start cold on the next connection.
-- No wiki_pages row is affected, so down-migration is safe to run
-- while a wiki is open.

DROP TABLE IF EXISTS wiki_collab_snapshots;
