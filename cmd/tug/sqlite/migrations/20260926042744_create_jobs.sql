-- The jobs waiting to run, and the ones that failed for good: jobs.go.
CREATE TABLE jobs (
	id         INTEGER PRIMARY KEY,
	kind       TEXT NOT NULL,
	payload    TEXT NOT NULL,
	run_at     INTEGER NOT NULL,
	held_until INTEGER NOT NULL DEFAULT 0,
	attempts   INTEGER NOT NULL DEFAULT 0,
	error      TEXT,
	failed_at  DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX jobs_due ON jobs (run_at) WHERE failed_at IS NULL;
