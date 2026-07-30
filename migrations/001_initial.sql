-- Approved normalized records only. Raw protocol/configuration columns are forbidden.
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
CREATE TABLE probe_executions (id TEXT PRIMARY KEY, scan_id TEXT NOT NULL, probe_id TEXT NOT NULL, outcome TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL, reason TEXT NOT NULL);
