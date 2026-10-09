-- The kinds a rate holds back, each until its time: jobs.go.
CREATE TABLE held_kinds (
	kind       TEXT PRIMARY KEY,
	held_until INTEGER NOT NULL
);
