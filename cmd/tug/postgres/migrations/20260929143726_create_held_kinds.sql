-- The kinds a rate holds back, each until its time: jobs.go.
CREATE TABLE held_kinds (
	kind       text PRIMARY KEY,
	held_until bigint NOT NULL
);
