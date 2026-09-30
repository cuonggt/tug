-- A row for each key of a kind that runs one at a time, which a claim
-- of one of its jobs locks, so that two claims of the key take turns:
-- jobs_db.go.
CREATE TABLE job_locks (
	kind      text NOT NULL,
	alone_key text NOT NULL,
	PRIMARY KEY (kind, alone_key)
);
