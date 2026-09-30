-- A kind whose jobs run so many at once at most keeps its limit with
-- each of them in at_once, and a claim counts the jobs held, by
-- jobs_held; a kind that a rate holds back waits in held_kinds until
-- its time: jobs.go.
ALTER TABLE jobs ADD COLUMN at_once INTEGER;
CREATE INDEX jobs_held ON jobs (held_until);
CREATE TABLE held_kinds (
	kind       TEXT PRIMARY KEY,
	held_until INTEGER NOT NULL
);
