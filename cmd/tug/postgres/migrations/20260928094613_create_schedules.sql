-- The run of each schedule pushed last, so that one instance pushes
-- it: jobs.go.
CREATE TABLE schedules (
	name   text PRIMARY KEY,
	run_at bigint NOT NULL
);
