-- SQLite cannot drop a column on all supported versions without rebuilding
-- the table. The column is harmless when rolling back the Lite migration set.

