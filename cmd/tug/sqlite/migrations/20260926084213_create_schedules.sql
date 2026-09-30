-- The run of each schedule pushed last, so that one instance pushes
-- it: jobs.go.
CREATE TABLE schedules (
	name   TEXT PRIMARY KEY,
	run_at INTEGER NOT NULL
);
