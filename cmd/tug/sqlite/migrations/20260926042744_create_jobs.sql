-- The jobs waiting to run, and the ones that failed for good: jobs.go.
-- A unique kind's job keeps its key in unique_key while it waits, and of
-- a kind and a key, one job can; a job of a kind that runs one at a time
-- keeps its key in alone_key all its life, where a claim lets unique_key
-- go, for a claim to see whether another of the key runs. A kind whose
-- jobs run so many at once at most keeps its limit with each of them in
-- at_once, and a claim counts the jobs held, by jobs_held. carried is
-- what a job carried from the context it was pushed from, as the ID of
-- the request that pushed it, as JSON, which a claim and the failed jobs'
-- list give back.
CREATE TABLE jobs (
	id         INTEGER PRIMARY KEY,
	kind       TEXT NOT NULL,
	payload    TEXT NOT NULL,
	run_at     INTEGER NOT NULL,
	held_until INTEGER NOT NULL DEFAULT 0,
	attempts   INTEGER NOT NULL DEFAULT 0,
	error      TEXT,
	failed_at  DATETIME,
	unique_key TEXT,
	alone_key  TEXT,
	at_once    INTEGER,
	carried    TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX jobs_due ON jobs (run_at) WHERE failed_at IS NULL;
CREATE UNIQUE INDEX jobs_unique ON jobs (kind, unique_key) WHERE unique_key IS NOT NULL;
CREATE INDEX jobs_alone ON jobs (kind, alone_key) WHERE alone_key IS NOT NULL;
CREATE INDEX jobs_held ON jobs (held_until);
